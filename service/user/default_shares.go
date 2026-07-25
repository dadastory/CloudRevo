package user

import (
	"context"
	"fmt"

	"github.com/dadastory/CloudRevo/application/constants"
	"github.com/dadastory/CloudRevo/application/dependency"
	"github.com/dadastory/CloudRevo/ent"
	usermodel "github.com/dadastory/CloudRevo/ent/user"
	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/pkg/crontab"
	"github.com/dadastory/CloudRevo/pkg/defaultshare"
	"github.com/dadastory/CloudRevo/pkg/hashid"
	"github.com/dadastory/CloudRevo/pkg/setting"
)

const (
	maxDefaultShareShortcuts       = 20
	defaultShareRedirectKey        = "sys:shared_redirect"
	defaultShareRecipientBatchSize = 100
)

func init() {
	crontab.Register(setting.CronTypeDefaultShareReconcile, func(ctx context.Context) {
		dep := dependency.FromContext(ctx)
		if err := ReconcileAllDefaultShareShortcuts(ctx, dep); err != nil {
			dep.Logger().Error("Failed to reconcile default share shortcuts: %s", err)
		}
	})
}

// provisionDefaultShareShortcuts creates symbolic folders in a new user's
// root. The shared-with-me navigator exposes these shortcuts without copying
// the source files or granting permissions beyond the share's own rule.
func provisionDefaultShareShortcuts(ctx context.Context, dep dependency.Dep, target *ent.User) error {
	targetWithGroup, err := dep.UserClient().GetByID(context.WithValue(ctx, inventory.LoadUserGroup{}, true), target.ID)
	if err != nil {
		return fmt.Errorf("load default-share recipient: %w", err)
	}
	target = targetWithGroup
	fileClient := dep.FileClient()
	root, err := fileClient.Root(ctx, target)
	if err != nil {
		root, err = fileClient.CreateFolder(ctx, nil, &inventory.CreateFolderParameters{
			Owner: target.ID,
			Name:  inventory.RootFolderName,
		})
		if err != nil {
			return fmt.Errorf("initialize user root: %w", err)
		}
	}

	pageToken := ""
	created := 0
	for {
		shareCtx := context.WithValue(ctx, inventory.LoadShareUser{}, true)
		shareCtx = context.WithValue(shareCtx, inventory.LoadShareFile{}, true)
		res, err := dep.ShareClient().List(shareCtx, &inventory.ListShareArgs{
			DefaultOnly: true,
			PaginationArgs: &inventory.PaginationArgs{
				UseCursorPagination: true,
				PageToken:           pageToken,
				PageSize:            100,
			},
		})
		if err != nil {
			return fmt.Errorf("list default shares: %w", err)
		}

		for _, share := range res.Shares {
			if created >= maxDefaultShareShortcuts {
				return nil
			}
			added, err := provisionDefaultShareShortcut(ctx, dep, target, root, share)
			if err != nil {
				return err
			}
			if added {
				created++
			}
		}

		if res.NextPageToken == "" {
			return nil
		}
		pageToken = res.NextPageToken
	}
}

func defaultShareRedirect(dep dependency.Dep, share *ent.Share) string {
	return fmt.Sprintf("%s://%s@%s", constants.CloudRevoScheme,
		hashid.EncodeShareID(dep.HashIDEncoder(), share.ID), constants.FileSystemShare)
}

func provisionDefaultShareShortcut(ctx context.Context, dep dependency.Dep, target *ent.User, root *ent.File, share *ent.Share) (bool, error) {
	if !share.IsDefault || inventory.IsValidShare(share) != nil ||
		!canReceiveDefaultShare(share, target) || share.Edges.File == nil {
		return false, nil
	}
	fileRoot, err := dep.FileClient().Root(ctx, target)
	if err != nil {
		fileRoot, err = dep.FileClient().CreateFolder(ctx, nil, &inventory.CreateFolderParameters{Owner: target.ID, Name: inventory.RootFolderName})
		if err != nil {
			return false, fmt.Errorf("initialize user root: %w", err)
		}
	}
	if root != nil {
		fileRoot = root
	}
	if _, err := dep.FileClient().CreateFolder(ctx, fileRoot, &inventory.CreateFolderParameters{
		Owner:      target.ID,
		Name:       fmt.Sprintf("%s (%d)", share.Edges.File.Name, share.ID),
		IsSymbolic: true,
		Metadata:   map[string]string{defaultShareRedirectKey: defaultShareRedirect(dep, share)},
	}); err != nil && !ent.IsConstraintError(err) {
		return false, fmt.Errorf("create default share shortcut %d: %w", share.ID, err)
	}
	return true, nil
}

// ReconcileDefaultShareShortcuts is a background-only operation that removes
// stale shortcuts and recreates eligible ones. Request paths must use
// CleanupDefaultShareShortcuts instead of enumerating every active user.
func ReconcileDefaultShareShortcuts(ctx context.Context, dep dependency.Dep, share *ent.Share) error {
	if share == nil {
		return nil
	}
	if share.Edges.User == nil || share.Edges.File == nil {
		shareCtx := context.WithValue(ctx, inventory.LoadShareUser{}, true)
		shareCtx = context.WithValue(shareCtx, inventory.LoadShareFile{}, true)
		loaded, err := dep.ShareClient().GetByID(shareCtx, share.ID)
		if err != nil {
			return fmt.Errorf("load default share %d: %w", share.ID, err)
		}
		share = loaded
	}
	if !share.IsDefault || inventory.IsValidShare(share) != nil || share.Edges.File == nil {
		return CleanupDefaultShareShortcuts(ctx, dep, share)
	}
	page := defaultShareReconcilePage(dep, share.ID)
	users, err := dep.UserClient().ListUsers(context.WithValue(ctx, inventory.LoadUserGroup{}, true), &inventory.ListUserParameters{
		PaginationArgs: &inventory.PaginationArgs{Page: page, PageSize: defaultShareRecipientBatchSize},
		Status:         usermodel.StatusActive,
	})
	if err != nil {
		return fmt.Errorf("list default-share recipients: %w", err)
	}
	for _, target := range users.Users {
		if canReceiveDefaultShare(share, target) {
			if _, err := provisionDefaultShareShortcut(ctx, dep, target, nil, share); err != nil {
				return err
			}
			continue
		}
		if err := dep.FileClient().DeleteSymbolicFoldersByMetadataForOwner(ctx, target.ID, defaultShareRedirectKey, defaultShareRedirect(dep, share)); err != nil {
			return err
		}
	}
	nextPage := page + 1
	if len(users.Users) < users.PageSize {
		nextPage = 0
	}
	if err := dep.KV().Set(defaultshare.ReconciliationPageKey(share.ID), nextPage, 0); err != nil {
		return fmt.Errorf("store default-share reconciliation cursor: %w", err)
	}
	return nil
}

func defaultShareReconcilePage(dep dependency.Dep, shareID int) int {
	value, ok := dep.KV().Get(defaultshare.ReconciliationPageKey(shareID))
	if !ok {
		return 0
	}
	page, ok := value.(int)
	if !ok || page < 0 {
		return 0
	}
	return page
}

// CleanupDefaultShareShortcuts removes all materialized shortcuts for one
// opaque share redirect. It is bounded by the number of matching folders and
// is safe to call before or after the share record is deleted.
func CleanupDefaultShareShortcuts(ctx context.Context, dep dependency.Dep, share *ent.Share) error {
	if share == nil {
		return nil
	}
	return dep.FileClient().DeleteSymbolicFoldersByMetadata(ctx, defaultShareRedirectKey, defaultShareRedirect(dep, share))
}

// ReconcileAllDefaultShareShortcuts runs from cron so share and file-rule
// requests never enumerate tenant recipients. It also cleans up expired or
// disabled defaults in the same pass.
func ReconcileAllDefaultShareShortcuts(ctx context.Context, dep dependency.Dep) error {
	pageToken := ""
	for {
		shareCtx := context.WithValue(ctx, inventory.LoadShareFile{}, true)
		shareCtx = context.WithValue(shareCtx, inventory.LoadShareUser{}, true)
		res, err := dep.ShareClient().List(shareCtx, &inventory.ListShareArgs{PaginationArgs: &inventory.PaginationArgs{
			UseCursorPagination: true,
			PageToken:           pageToken,
			PageSize:            100,
		}, DefaultOnly: true})
		if err != nil {
			return fmt.Errorf("list default shares for reconciliation: %w", err)
		}
		for _, share := range res.Shares {
			if err := ReconcileDefaultShareShortcuts(ctx, dep, share); err != nil {
				return err
			}
		}
		if res.NextPageToken == "" {
			return nil
		}
		pageToken = res.NextPageToken
	}
}

// ReconcileDefaultShareShortcutsForFile refreshes every default shortcut whose
// source object was just assigned a new global access rule.
func ReconcileDefaultShareShortcutsForFile(ctx context.Context, dep dependency.Dep, fileID int) error {
	return defaultshare.RestartForFile(ctx, dep.ShareClient(), dep.KV(), fileID)
}

func canReceiveDefaultShare(share *ent.Share, target *ent.User) bool {
	if target == nil || share == nil {
		return false
	}
	if share.Edges.File == nil || share.Edges.File.Props == nil || share.Edges.File.Props.ShareAccessRule == nil {
		return true
	}
	groupID := 0
	if target.Edges.Group != nil {
		groupID = target.Edges.Group.ID
	}
	return share.Edges.File.Props.ShareAccessRule.Resolve(target.ID, groupID, inventory.IsAnonymousUser(target)).Read
}
