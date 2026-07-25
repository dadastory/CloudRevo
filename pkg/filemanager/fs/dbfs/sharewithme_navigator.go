package dbfs

import (
	"context"
	"errors"
	"fmt"

	"github.com/dadastory/CloudRevo/application/constants"
	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/boolset"
	"github.com/dadastory/CloudRevo/pkg/cache"
	"github.com/dadastory/CloudRevo/pkg/filemanager/fs"
	"github.com/dadastory/CloudRevo/pkg/hashid"
	"github.com/dadastory/CloudRevo/pkg/logging"
	"github.com/dadastory/CloudRevo/pkg/setting"
)

var sharedWithMeNavigatorCapability = &boolset.BooleanSet{}

// NewSharedWithMeNavigator creates a navigator for user's "shared with me" file system.
func NewSharedWithMeNavigator(u *ent.User, fileClient inventory.FileClient, l logging.Logger,
	config *setting.DBFS, hasher hashid.Encoder, shareClient inventory.ShareClient) Navigator {
	n := &sharedWithMeNavigator{
		user:        u,
		l:           l,
		fileClient:  fileClient,
		config:      config,
		hasher:      hasher,
		shareClient: shareClient,
	}
	n.baseNavigator = newBaseNavigator(fileClient, defaultFilter, u, hasher, config)
	return n
}

type sharedWithMeNavigator struct {
	l           logging.Logger
	user        *ent.User
	fileClient  inventory.FileClient
	config      *setting.DBFS
	hasher      hashid.Encoder
	shareClient inventory.ShareClient

	root *File
	*baseNavigator
}

func (t *sharedWithMeNavigator) Recycle() {

}

func (n *sharedWithMeNavigator) PersistState(kv cache.Driver, key string) {
}

func (n *sharedWithMeNavigator) RestoreState(s State) error {
	return nil
}

func (t *sharedWithMeNavigator) To(ctx context.Context, path *fs.URI) (*File, error) {
	// Anonymous user does not have a trash folder.
	if inventory.IsAnonymousUser(t.user) {
		return nil, ErrLoginRequired
	}

	elements := path.Elements()
	if len(elements) > 0 {
		// Shared with me folder is a flatten tree, only root can be accessed.
		return nil, fs.ErrPathNotExist.WithError(fmt.Errorf("invalid Path %q", path))
	}

	if t.root == nil {
		rootFile, err := t.fileClient.Root(ctx, t.user)
		if err != nil {
			t.l.Info("User's root folder not found: %s, will initialize it.", err)
			return nil, ErrFsNotInitialized
		}

		t.root = newFile(nil, rootFile)
		rootPath := newSharedWithMeUri("")
		t.root.Path[pathIndexRoot], t.root.Path[pathIndexUser] = rootPath, rootPath
		t.root.OwnerModel = t.user
		t.root.IsUserRoot = true
		t.root.CapabilitiesBs = t.Capabilities(false).Capability
	}

	return t.root, nil
}

func (t *sharedWithMeNavigator) Children(ctx context.Context, parent *File, args *ListArgs) (*ListResult, error) {
	res, err := paginateVisibleChildren(args, func(pageArgs *ListArgs) (*ListResult, error) {
		pageArgs.SharedWithMe = true
		return t.baseNavigator.children(ctx, nil, pageArgs)
	}, func(file *File) bool {
		if !t.defaultShortcutVisible(ctx, file) {
			return false
		}
		file.Path[pathIndexUser] = newSharedWithMeUri(hashid.EncodeFileID(t.hasher, file.Model.ID))
		return true
	})
	if err != nil {
		return nil, err
	}

	return res, nil
}

// defaultShortcutVisible prevents a materialized default shortcut from
// revealing its source name while a background cleanup sweep is pending.
func (t *sharedWithMeNavigator) defaultShortcutVisible(ctx context.Context, file *File) bool {
	if !file.IsSymbolic() {
		return true
	}
	if file.Model.Edges.Metadata == nil {
		if err := t.fileClient.QueryMetadata(ctx, file.Model); err != nil {
			t.l.Warning("Failed to load shared shortcut metadata: %s", err)
			return false
		}
	}
	redirect, ok := file.Metadata()[MetadataSharedRedirect]
	if !ok {
		return true
	}
	uri, err := fs.NewUriFromString(redirect)
	if err != nil || uri.FileSystem() != constants.FileSystemShare {
		return false
	}
	shareCtx := context.WithValue(ctx, inventory.LoadShareFile{}, true)
	shareCtx = context.WithValue(shareCtx, inventory.LoadShareUser{}, true)
	share, err := t.shareClient.GetByHashID(shareCtx, uri.ID(""))
	if err != nil {
		return false
	}
	if inventory.IsValidShare(share) != nil {
		return false
	}
	return canListDefaultShareShortcut(t.user, share)
}

func canListDefaultShareShortcut(user *ent.User, share *ent.Share) bool {
	if user == nil || share == nil || !share.IsDefault || share.Edges.File == nil {
		return false
	}
	if share.Edges.File.Props == nil || share.Edges.File.Props.ShareAccessRule == nil {
		return true
	}
	groupID := 0
	if user.Edges.Group != nil {
		groupID = user.Edges.Group.ID
	}
	return share.Edges.File.Props.ShareAccessRule.Resolve(user.ID, groupID, inventory.IsAnonymousUser(user)).Read
}

func (t *sharedWithMeNavigator) Capabilities(isSearching bool) *fs.NavigatorProps {
	res := &fs.NavigatorProps{
		Capability:            sharedWithMeNavigatorCapability,
		OrderDirectionOptions: fullOrderDirectionOption,
		OrderByOptions:        fullOrderByOption,
		MaxPageSize:           t.config.MaxPageSize,
	}

	if isSearching {
		res.OrderByOptions = searchLimitedOrderByOption
	}

	return res
}

func (t *sharedWithMeNavigator) Walk(ctx context.Context, levelFiles []*File, limit, depth int, f WalkFunc) error {
	return errors.New("not implemented")
}

func (n *sharedWithMeNavigator) FollowTx(ctx context.Context) (func(), error) {
	if _, ok := ctx.Value(inventory.TxCtx{}).(*inventory.Tx); !ok {
		return nil, fmt.Errorf("navigator: no inherited transaction found in context")
	}
	newFileClient, _, _, err := inventory.WithTx(ctx, n.fileClient)
	if err != nil {
		return nil, err
	}

	oldFileClient := n.fileClient
	revert := func() {
		n.fileClient = oldFileClient
		n.baseNavigator.fileClient = oldFileClient
	}

	n.fileClient = newFileClient
	n.baseNavigator.fileClient = newFileClient
	return revert, nil
}

func (n *sharedWithMeNavigator) ExecuteHook(ctx context.Context, hookType fs.HookType, file *File) error {
	return nil
}

func (n *sharedWithMeNavigator) GetView(ctx context.Context, file *File) *types.ExplorerView {
	if view, ok := n.user.Settings.FsViewMap[string(constants.FileSystemSharedWithMe)]; ok {
		return &view
	}
	return getDefaultView()
}
