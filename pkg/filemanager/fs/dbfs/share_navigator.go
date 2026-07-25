package dbfs

import (
	"context"
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
	"github.com/dadastory/CloudRevo/pkg/serializer"
	"github.com/dadastory/CloudRevo/pkg/setting"
)

var (
	ErrShareNotFound = serializer.NewError(serializer.CodeNotFound, "Shared file does not exist", nil)
	ErrNotPurchased  = serializer.NewError(serializer.CodePurchaseRequired, "You need to purchased this share", nil)
)

const (
	PurchaseTicketHeader = constants.CrHeaderPrefix + "Purchase-Ticket"
)

var shareNavigatorCapability = &boolset.BooleanSet{}

// NewShareNavigator creates a navigator for user's "shared" file system.
func NewShareNavigator(u *ent.User, fileClient inventory.FileClient, shareClient inventory.ShareClient,
	l logging.Logger, config *setting.DBFS, hasher hashid.Encoder) Navigator {
	n := &shareNavigator{
		user:        u,
		l:           l,
		fileClient:  fileClient,
		shareClient: shareClient,
		config:      config,
	}
	n.baseNavigator = newBaseNavigator(fileClient, defaultFilter, u, hasher, config)
	return n
}

type (
	shareNavigator struct {
		l           logging.Logger
		user        *ent.User
		fileClient  inventory.FileClient
		shareClient inventory.ShareClient
		config      *setting.DBFS

		*baseNavigator
		shareRoot       *File
		singleFileShare bool
		ownerRoot       *File
		share           *ent.Share
		owner           *ent.User
		disableRecycle  bool
		persist         func()
	}

	shareNavigatorState struct {
		ShareRoot       *File
		OwnerRoot       *File
		SingleFileShare bool
		Share           *ent.Share
		Owner           *ent.User
	}
)

func (n *shareNavigator) PersistState(kv cache.Driver, key string) {
	// Share state includes the active access rule. It must be loaded for every
	// request so a revoked permission cannot survive in a Context Hint cache.
}

func (n *shareNavigator) RestoreState(s State) error {
	return fmt.Errorf("share navigator state cannot be restored from Context Hint")
}

func (n *shareNavigator) Recycle() {
	if n.persist != nil {
		n.persist()
		n.persist = nil
	}

	if !n.disableRecycle {
		if n.ownerRoot != nil {
			n.ownerRoot.Recycle()
		} else if n.shareRoot != nil {
			n.shareRoot.Recycle()
		}
	}
}

func (n *shareNavigator) Root(ctx context.Context, path *fs.URI) (*File, error) {
	n.singleFileShare = false
	ctx = context.WithValue(ctx, inventory.LoadShareUser{}, true)
	ctx = context.WithValue(ctx, inventory.LoadUserGroup{}, true)
	ctx = context.WithValue(ctx, inventory.LoadShareFile{}, true)
	share, err := n.shareClient.GetByHashID(ctx, path.ID(hashid.EncodeUserID(n.hasher, n.user.ID)))
	if err != nil {
		return nil, ErrShareNotFound.WithError(err)
	}

	if err := inventory.IsValidShare(share); err != nil {
		return nil, ErrShareNotFound.WithError(err)
	}

	n.owner = share.Edges.User
	n.share = share

	// Check password
	if share.Password != "" && share.Password != path.Password() {
		return nil, ErrShareIncorrectPassword
	}

	// File and folder permissions are global. The source object's rule is the
	// only rule that governs a share root; link settings never carry an ACL.
	n.shareRoot = newFile(nil, share.Edges.File)

	// Find the user side root of the file.
	ownerRoot, err := n.findRoot(ctx, n.shareRoot)
	if err != nil {
		return nil, err
	}

	if n.shareRoot.Type() == types.FileTypeFile {
		n.singleFileShare = true
		n.shareRoot = n.shareRoot.Parent
	}

	n.shareRoot.Path[pathIndexUser] = path.Root()
	n.shareRoot.OwnerModel = n.owner
	n.shareRoot.IsUserRoot = true
	n.shareRoot.disableView = (share.Props == nil || !share.Props.ShareView) && n.user.ID != n.owner.ID
	n.shareRoot.CapabilitiesBs = n.Capabilities(false).Capability

	// Check if any ancestors is deleted
	if ownerRoot.Name() != inventory.RootFolderName {
		return nil, ErrShareNotFound
	}

	if n.user.ID != n.owner.ID && !n.user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionShareDownload)) {
		if inventory.IsAnonymousUser(n.user) {
			return nil, serializer.NewError(
				serializer.CodeAnonymouseAccessDenied,
				fmt.Sprintf("You don't have permission to access share links"),
				err,
			)
		}

		return nil, serializer.NewError(
			serializer.CodeNoPermissionErr,
			fmt.Sprintf("You don't have permission to access share links"),
			err,
		)
	}

	n.ownerRoot = ownerRoot
	n.ownerRoot.Path[pathIndexRoot] = newMyIDUri(hashid.EncodeUserID(n.hasher, n.owner.ID))
	return n.shareRoot, nil
}

func (n *shareNavigator) accessPermission() types.SharePermission {
	if n.share == nil {
		return types.SharePermission{}
	}

	if n.share.Edges.File != nil && n.share.Edges.File.Props != nil && n.share.Edges.File.Props.ShareAccessRule != nil {
		return n.resolveAccessRule(n.share.Edges.File.Props.ShareAccessRule)
	}
	return types.SharePermission{Read: true}
}

func (n *shareNavigator) resolveAccessRule(rule *types.ShareAccessRule) types.SharePermission {
	groupID := 0
	if n.user != nil && n.user.Edges.Group != nil {
		groupID = n.user.Edges.Group.ID
	}

	return rule.Resolve(n.user.ID, groupID, inventory.IsAnonymousUser(n.user))
}

// accessPermissionFor applies the closest configured object rule. A child
// rule replaces its ancestor's rule, so a directory owner can grant a more
// specific capability without duplicating the source rule on every child.
func (n *shareNavigator) accessPermissionFor(file *File) types.SharePermission {
	for current := file; current != nil && current != n.shareRoot; current = current.Parent {
		if current.Model != nil && n.share != nil && n.share.Edges.File != nil &&
			n.share.Edges.File.ID != 0 && current.Model.ID == n.share.Edges.File.ID {
			continue
		}
		if current.Model != nil && current.Model.Props != nil && current.Model.Props.ShareAccessRule != nil {
			return n.resolveAccessRule(current.Model.Props.ShareAccessRule)
		}
	}
	return n.accessPermission()
}

func (n *shareNavigator) capabilitiesFor(file *File) *boolset.BooleanSet {
	permission := n.accessPermissionFor(file)
	capability := boolset.BooleanSet(append([]byte(nil), (*shareNavigatorCapability)...))
	boolset.Sets(map[NavigatorCapability]bool{
		NavigatorCapabilityListChildren:   permission.Read,
		NavigatorCapabilityDownloadFile:   permission.Read,
		NavigatorCapabilityGenerateThumb:  permission.Read,
		NavigatorCapabilityInfo:           permission.Read,
		NavigatorCapabilityEnterFolder:    permission.Read,
		NavigatorCapabilityLockFile:       false,
		NavigatorCapabilityCreateFile:     permission.Create && !n.singleFileShare,
		NavigatorCapabilityUploadFile:     permission.Create && !n.singleFileShare,
		NavigatorCapabilityRenameFile:     permission.Update && !n.singleFileShare,
		NavigatorCapabilityUpdateMetadata: permission.Update,
		NavigatorCapabilityDeleteFile:     permission.Delete && !n.singleFileShare,
		NavigatorCapabilitySoftDelete:     permission.Delete && !n.singleFileShare,
		NavigatorCapabilityVersionControl: permission.Update,
		NavigatorCapabilityModifyProps:    permission.Update,
	}, &capability)

	return &capability
}

func (n *shareNavigator) allows(file *File, capability NavigatorCapability) bool {
	return n.capabilitiesFor(file).Enabled(int(capability))
}

func (n *shareNavigator) ensureAccess(ctx context.Context, path *fs.URI, capabilities ...NavigatorCapability) error {
	target, err := n.To(ctx, path)
	if err != nil && target == nil {
		return err
	}
	for _, capability := range capabilities {
		if !n.allows(target, capability) {
			return ErrPermissionDenied
		}
	}
	return nil
}

func (n *shareNavigator) To(ctx context.Context, path *fs.URI) (*File, error) {
	if n.shareRoot == nil {
		root, err := n.Root(ctx, path)
		if err != nil {
			return nil, err
		}

		n.shareRoot = root
	}

	current, lastAncestor := n.shareRoot, n.shareRoot
	elements := path.Elements()

	// If target is root of single file share, the root itself is the target.
	if len(elements) == 1 && n.singleFileShare {
		file, err := n.latestSharedSingleFile(ctx)
		if err != nil {
			return nil, err
		}

		if len(elements) == 1 && file.Name() != elements[0] {
			return nil, fs.ErrPathNotExist
		}

		file.CapabilitiesBs = n.capabilitiesFor(file)
		return file, nil
	}

	var err error
	for index, element := range elements {
		lastAncestor = current
		current, err = n.walkNext(ctx, current, element, index == len(elements)-1)
		if err != nil {
			return lastAncestor, fmt.Errorf("failed to walk into %q: %w", element, err)
		}
	}

	current.CapabilitiesBs = n.capabilitiesFor(current)
	return current, nil
}

func (n *shareNavigator) walkNext(ctx context.Context, root *File, next string, isLeaf bool) (*File, error) {
	nextFile, err := n.baseNavigator.walkNext(ctx, root, next, isLeaf)
	if err != nil {
		return nil, err
	}

	nextFile.CapabilitiesBs = n.capabilitiesFor(nextFile)
	return nextFile, nil
}

func (n *shareNavigator) Children(ctx context.Context, parent *File, args *ListArgs) (*ListResult, error) {
	if n.singleFileShare {
		file, err := n.latestSharedSingleFile(ctx)
		if err != nil {
			return nil, err
		}

		return &ListResult{
			Files:          []*File{file},
			Pagination:     &inventory.PaginationResults{},
			SingleFileView: true,
		}, nil
	}

	return paginateVisibleChildren(args, func(pageArgs *ListArgs) (*ListResult, error) {
		return n.baseNavigator.children(ctx, parent, pageArgs)
	}, func(file *File) bool {
		return len(filterReadableShareChildren(n, []*File{file})) == 1
	})
}

// filterReadableShareChildren applies the disclosure boundary before a file is
// returned to an API response. Empty capabilities are not sufficient because a
// directory listing itself exposes a child name and metadata.
func filterReadableShareChildren(n *shareNavigator, files []*File) []*File {
	visible := files[:0]
	for _, file := range files {
		if !n.accessPermissionFor(file).Read {
			continue
		}
		file.CapabilitiesBs = n.capabilitiesFor(file)
		visible = append(visible, file)
	}
	return visible
}

func (n *shareNavigator) latestSharedSingleFile(ctx context.Context) (*File, error) {
	if n.singleFileShare {
		file, err := n.fileClient.GetByID(ctx, n.share.Edges.File.ID)
		if err != nil {
			return nil, err
		}

		f := newFile(n.shareRoot, file)
		f.OwnerModel = n.shareRoot.OwnerModel
		f.CapabilitiesBs = n.capabilitiesFor(f)

		return f, nil
	}

	return nil, fs.ErrPathNotExist
}

func (n *shareNavigator) Capabilities(isSearching bool) *fs.NavigatorProps {
	maxPageSize := 0
	if n.config != nil {
		maxPageSize = n.config.MaxPageSize
	}
	res := &fs.NavigatorProps{
		Capability:            n.capabilitiesFor(n.shareRoot),
		OrderDirectionOptions: fullOrderDirectionOption,
		OrderByOptions:        fullOrderByOption,
		MaxPageSize:           maxPageSize,
	}

	if isSearching {
		res.OrderByOptions = nil
		res.OrderDirectionOptions = nil
	}

	return res
}

func (n *shareNavigator) FollowTx(ctx context.Context) (func(), error) {
	if _, ok := ctx.Value(inventory.TxCtx{}).(*inventory.Tx); !ok {
		return nil, fmt.Errorf("navigator: no inherited transaction found in context")
	}
	newFileClient, _, _, err := inventory.WithTx(ctx, n.fileClient)
	if err != nil {
		return nil, err
	}

	newSharClient, _, _, err := inventory.WithTx(ctx, n.shareClient)
	if err != nil {
		return nil, err
	}

	oldFileClient, oldShareClient := n.fileClient, n.shareClient
	revert := func() {
		n.fileClient = oldFileClient
		n.shareClient = oldShareClient
		n.baseNavigator.fileClient = oldFileClient
	}

	n.fileClient = newFileClient
	n.shareClient = newSharClient
	n.baseNavigator.fileClient = newFileClient
	return revert, nil
}

func (n *shareNavigator) ExecuteHook(ctx context.Context, hookType fs.HookType, file *File) error {
	switch hookType {
	case fs.HookTypeBeforeDownload:
		return n.shareClient.Downloaded(ctx, n.share)
	}
	return nil
}

func (n *shareNavigator) Walk(ctx context.Context, levelFiles []*File, limit, depth int, f WalkFunc) error {
	return n.baseNavigator.walk(ctx, levelFiles, limit, depth, f)
}

func (n *shareNavigator) GetView(ctx context.Context, file *File) *types.ExplorerView {
	return file.View()
}
