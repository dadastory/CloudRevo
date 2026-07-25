package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/dadastory/CloudRevo/application/dependency"
	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/cluster/routes"
	"github.com/dadastory/CloudRevo/pkg/defaultshare"
	"github.com/dadastory/CloudRevo/pkg/hashid"
	"github.com/dadastory/CloudRevo/pkg/serializer"
	shareservice "github.com/dadastory/CloudRevo/service/share"
	userservice "github.com/dadastory/CloudRevo/service/user"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

const (
	shareUserIDCondition = "share_user_id"
	shareFileIDCondition = "share_file_id"
	shareIDCondition     = "share_id"
)

func (s *AdminListService) Shares(c *gin.Context) (*ListShareResponse, error) {
	dep := dependency.FromContext(c)
	shareClient := dep.ShareClient()
	hasher := dep.HashIDEncoder()

	var (
		err      error
		userID   int
		fileID   int
		shareIDs []int
	)

	if s.Conditions[shareUserIDCondition] != "" {
		userID, err = strconv.Atoi(s.Conditions[shareUserIDCondition])
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid share user ID", err)
		}
	}

	if s.Conditions[shareFileIDCondition] != "" {
		fileID, err = strconv.Atoi(s.Conditions[shareFileIDCondition])
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid share file ID", err)
		}
	}

	if s.Conditions[shareIDCondition] != "" {
		shareIdStrs := strings.Split(s.Conditions[shareIDCondition], ",")
		for _, shareIdStr := range shareIdStrs {
			shareID, err := strconv.Atoi(shareIdStr)
			if err != nil {
				return nil, serializer.NewError(serializer.CodeParamErr, "Invalid share ID", err)
			}

			shareIDs = append(shareIDs, shareID)
		}
	}

	ctx := context.WithValue(c, inventory.LoadShareFile{}, true)
	ctx = context.WithValue(ctx, inventory.LoadShareUser{}, true)

	res, err := shareClient.List(ctx, &inventory.ListShareArgs{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     s.Page - 1,
			PageSize: s.PageSize,
			OrderBy:  s.OrderBy,
			Order:    inventory.OrderDirection(s.OrderDirection),
		},
		UserID:   userID,
		FileID:   fileID,
		ShareIDs: shareIDs,
	})

	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list shares", err)
	}

	siteUrl := dep.SettingProvider().SiteURL(c)

	return &ListShareResponse{
		Pagination: res.PaginationResults,
		Shares: lo.Map(res.Shares, func(share *ent.Share, _ int) GetShareResponse {
			var (
				uid       string
				shareLink string
			)

			if share.Edges.User != nil {
				uid = hashid.EncodeUserID(hasher, share.Edges.User.ID)
			}

			shareLink = routes.MasterShareUrl(siteUrl, hashid.EncodeShareID(hasher, share.ID), share.Password).String()

			return GetShareResponse{
				Share:      share,
				UserHashID: uid,
				ShareLink:  shareLink,
			}
		}),
	}, nil

}

type (
	SingleShareService struct {
		ShareID int `uri:"id" binding:"required"`
	}
	SingleShareParamCtx struct{}
)

func (s *SingleShareService) Get(c *gin.Context) (*GetShareResponse, error) {
	dep := dependency.FromContext(c)
	shareClient := dep.ShareClient()
	hasher := dep.HashIDEncoder()

	ctx := context.WithValue(c, inventory.LoadShareFile{}, true)
	ctx = context.WithValue(ctx, inventory.LoadShareUser{}, true)
	share, err := shareClient.GetByID(ctx, s.ShareID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get share", err)
	}

	var (
		uid       string
		shareLink string
	)

	if share.Edges.User != nil {
		uid = hashid.EncodeUserID(hasher, share.Edges.User.ID)
	}

	siteUrl := dep.SettingProvider().SiteURL(c)
	shareLink = routes.MasterShareUrl(siteUrl, hashid.EncodeShareID(hasher, share.ID), share.Password).String()

	return &GetShareResponse{
		Share:      share,
		UserHashID: uid,
		ShareLink:  shareLink,
	}, nil
}

type DefaultShareService struct {
	Default bool `json:"default"`
}

type DefaultShareParamCtx struct{}

// SetDefault changes only the administrator-managed onboarding marker. It
// deliberately does not reuse the owner-scoped share editor.
func (s *DefaultShareService) SetDefault(c *gin.Context, shareID int) error {
	dep := dependency.FromContext(c)
	shareCtx := context.WithValue(c, inventory.LoadShareFile{}, true)
	shareCtx = context.WithValue(shareCtx, inventory.LoadShareUser{}, true)
	share, err := dep.ShareClient().GetByID(shareCtx, shareID)
	if err != nil {
		return serializer.NewError(serializer.CodeNotFound, "share not found", err)
	}
	if s.Default {
		if share.Password != "" {
			return serializer.NewError(serializer.CodeParamErr, "Default shares cannot require a password", nil)
		}
		if err := inventory.IsValidShare(share); err != nil {
			return serializer.NewError(serializer.CodeParamErr, "Default shares must be valid", err)
		}
	}
	var defaultShareTx *ent.Tx
	if s.Default && !share.IsDefault {
		defaultShareTx, err = shareservice.ReserveDefaultShareSlot(c, dep)
		if err != nil {
			return err
		}
		defer func() { _ = defaultShareTx.Rollback() }()

		// Lock the candidate row before reloading its edges. This keeps a
		// concurrent owner update from changing validity between the check and
		// the default-marker write.
		if _, err := defaultShareTx.Share.UpdateOneID(share.ID).SetUpdatedAt(time.Now()).Save(c); err != nil {
			return serializer.NewError(serializer.CodeDBError, "failed to lock default share candidate", err)
		}
		transactionalShares := inventory.NewShareClient(defaultShareTx.Client(), dep.ConfigProvider().Database().Type, dep.HashIDEncoder())
		share, err = transactionalShares.GetByID(shareCtx, shareID)
		if err != nil {
			return serializer.NewError(serializer.CodeNotFound, "share not found", err)
		}
		if share.Password != "" {
			return serializer.NewError(serializer.CodeParamErr, "Default shares cannot require a password", nil)
		}
		if err := inventory.IsValidShare(share); err != nil {
			return serializer.NewError(serializer.CodeParamErr, "Default shares must be valid", err)
		}
	}
	props := share.Props
	if props == nil {
		props = &types.ShareProps{}
	}
	props.Default = s.Default
	shareWriter := dep.ShareClient().GetClient().Share
	if defaultShareTx != nil {
		shareWriter = defaultShareTx.Share
	}
	updated, err := shareWriter.UpdateOneID(share.ID).SetIsDefault(s.Default).SetProps(props).Save(c)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "failed to update default share", err)
	}
	if defaultShareTx != nil {
		if err := defaultShareTx.Commit(); err != nil {
			return serializer.NewError(serializer.CodeDBError, "failed to reserve default share slot", err)
		}
	}
	if s.Default {
		if err := defaultshare.Restart(dep.KV(), updated.ID); err != nil {
			return serializer.NewError(serializer.CodeCacheOperation, "failed to restart default-share reconciliation", err)
		}
		return nil
	}
	return userservice.CleanupDefaultShareShortcuts(c, dep, updated)
}

type (
	BatchShareService struct {
		ShareIDs []int `json:"ids" binding:"required"`
	}
	BatchShareParamCtx struct{}
)

func (s *BatchShareService) Delete(c *gin.Context) error {
	dep := dependency.FromContext(c)
	shareClient := dep.ShareClient()
	shares, err := shareClient.GetByIDs(c, s.ShareIDs)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to load shares", err)
	}

	for _, share := range shares {
		if err := userservice.CleanupDefaultShareShortcuts(c, dep, share); err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to clean up default share shortcuts", err)
		}
	}
	if err := shareClient.DeleteBatch(c, s.ShareIDs); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete shares", err)
	}

	return nil
}
