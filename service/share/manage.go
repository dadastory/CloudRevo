package share

import (
	"context"
	"fmt"
	"time"

	"github.com/dadastory/CloudRevo/application/dependency"
	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/ent/setting"
	"github.com/dadastory/CloudRevo/ent/share"
	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/defaultshare"
	"github.com/dadastory/CloudRevo/pkg/filemanager/fs"
	"github.com/dadastory/CloudRevo/pkg/filemanager/manager"
	"github.com/dadastory/CloudRevo/pkg/hashid"
	"github.com/dadastory/CloudRevo/pkg/serializer"
	"github.com/dadastory/CloudRevo/service/explorer"
	userservice "github.com/dadastory/CloudRevo/service/user"
	"github.com/gin-gonic/gin"
)

type (
	// ShareCreateService 创建新分享服务
	ShareCreateService struct {
		Uri             string `json:"uri" binding:"required"`
		IsPrivate       bool   `json:"is_private"`
		Password        string `json:"password" binding:"omitempty,max=32,alphanum"`
		RemainDownloads int    `json:"downloads"`
		Expire          int    `json:"expire"`
		ShareView       bool   `json:"share_view"`
		ShowReadMe      bool   `json:"show_readme"`
		Default         bool   `json:"default"`
	}
	ShareCreateParamCtx struct{}

	BatchDeleteShareService struct {
		ShareIDs []string `json:"ids" binding:"required"`
	}
	BatchDeleteParamCtx struct{}
)

// maxDefaultShares bounds the work performed during a new-user provisioning
// request. Default shares are an administrator-managed onboarding feature,
// not an unbounded distribution channel.
const maxDefaultShares = 20

const defaultShareLockSetting = "default_share_lock"

// reserveDefaultShareSlot uses a row lock in the shared database so separate
// server instances serialize the count-and-mark transition.
func ReserveDefaultShareSlot(ctx context.Context, dep dependency.Dep) (*ent.Tx, error) {
	if err := dep.DBClient().Setting.Create().SetName(defaultShareLockSetting).SetValue("0").OnConflict().DoNothing().Exec(ctx); err != nil {
		return nil, err
	}
	tx, err := dep.DBClient().Tx(ctx)
	if err != nil {
		return nil, err
	}
	rollback := func() error { return tx.Rollback() }
	if _, err := tx.Setting.Update().Where(setting.NameEQ(defaultShareLockSetting)).SetValue("1").Save(ctx); err != nil {
		_ = rollback()
		return nil, err
	}
	count, err := tx.Share.Query().Where(share.IsDefaultEQ(true)).Count(ctx)
	if err != nil {
		_ = rollback()
		return nil, err
	}
	if count >= maxDefaultShares {
		_ = rollback()
		return nil, serializer.NewError(serializer.CodeParamErr, "Default share limit reached", nil)
	}
	return tx, nil
}

func (service *BatchDeleteShareService) Delete(c *gin.Context) error {
	dep := dependency.FromContext(c)
	uid := inventory.UserIDFromContext(c)
	shareClient := dep.ShareClient()

	var ids []int

	for _, v := range service.ShareIDs {
		id, err := dep.HashIDEncoder().Decode(v, hashid.ShareID)
		if err != nil {
			return fmt.Errorf("failed to decode hash id %q: %w", v, err)
		}

		ids = append(ids, id)
	}

	shareCtx := context.WithValue(c, inventory.LoadShareFile{}, true)
	shareCtx = context.WithValue(shareCtx, inventory.LoadShareUser{}, true)
	shares, err := shareClient.GetByIDs(shareCtx, ids)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to load shares", err)
	}
	for _, share := range shares {
		if share.Edges.User == nil || share.Edges.User.ID != uid {
			continue
		}
		if err := userservice.CleanupDefaultShareShortcuts(c, dep, share); err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to reconcile default shares", err)
		}
	}
	if err := shareClient.DeleteBatchByUserID(c, uid, ids); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete shares", err)
	}

	return nil
}

// Upsert 创建或更新分享
func (service *ShareCreateService) Upsert(c *gin.Context, existed int) (string, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()
	// Check group permission for creating share link
	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionShare)) {
		return "", serializer.NewError(serializer.CodeGroupNotAllowed, "Group permission denied", nil)
	}
	isAdmin := user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin))
	if service.Default && !isAdmin {
		return "", serializer.NewError(serializer.CodeNoPermissionErr, "Only administrators can create default shares", nil)
	}
	if err := service.validateDefaultPassword(); err != nil {
		return "", err
	}
	wasDefault, err := isExistingDefaultShare(c, dep, user, existed)
	if err != nil {
		return "", err
	}
	if wasDefault && !isAdmin {
		return "", serializer.NewError(serializer.CodeNoPermissionErr, "Only administrators can modify default shares", nil)
	}
	var defaultShareTx *ent.Tx
	if service.Default && !wasDefault {
		defaultShareTx, err = ReserveDefaultShareSlot(c, dep)
		if err != nil {
			return "", err
		}
		defer func() { _ = defaultShareTx.Rollback() }()
	}

	uri, err := fs.NewUriFromString(service.Uri)
	if err != nil {
		return "", serializer.NewError(serializer.CodeParamErr, "unknown uri", err)
	}

	var expires *time.Time
	if service.Expire > 0 {
		expires = new(time.Time)
		*expires = time.Now().Add(time.Duration(service.Expire) * time.Second)
	}

	createArgs := &manager.CreateShareArgs{
		IsPrivate:       service.IsPrivate,
		Password:        service.Password,
		RemainDownloads: service.RemainDownloads,
		Expire:          expires,
		ExistedShareID:  existed,
		ShareView:       service.ShareView,
		ShowReadMe:      service.ShowReadMe,
		Default:         service.Default,
	}
	var share *ent.Share
	if defaultShareTx != nil {
		shareClient := inventory.NewShareClient(defaultShareTx.Client(), dep.ConfigProvider().Database().Type, dep.HashIDEncoder())
		share, err = m.CreateOrUpdateShareWithClient(c, uri, createArgs, shareClient)
	} else {
		share, err = m.CreateOrUpdateShare(c, uri, createArgs)
	}
	if err != nil {
		return "", err
	}
	if defaultShareTx != nil {
		if err := defaultShareTx.Commit(); err != nil {
			return "", serializer.NewError(serializer.CodeDBError, "Failed to reserve default share slot", err)
		}
	}
	if !service.Default && wasDefault {
		shareCtx := context.WithValue(c, inventory.LoadShareFile{}, true)
		shareWithFile, err := dep.ShareClient().GetByID(shareCtx, share.ID)
		if err != nil {
			return "", serializer.NewError(serializer.CodeDBError, "Failed to load default share", err)
		}
		if err := userservice.CleanupDefaultShareShortcuts(c, dep, shareWithFile); err != nil {
			return "", serializer.NewError(serializer.CodeDBError, "Failed to reconcile default shares", err)
		}
	}
	if service.Default {
		if err := defaultshare.Restart(dep.KV(), share.ID); err != nil {
			return "", serializer.NewError(serializer.CodeCacheOperation, "Failed to restart default-share reconciliation", err)
		}
	}

	base := dep.SettingProvider().SiteURL(c)
	return explorer.BuildShareLink(share, dep.HashIDEncoder(), base, true), nil
}

func (service *ShareCreateService) validateDefaultPassword() error {
	if service.Default && service.IsPrivate {
		return serializer.NewError(serializer.CodeParamErr, "Default shares cannot require a password", nil)
	}
	return nil
}

func isExistingDefaultShare(ctx context.Context, dep dependency.Dep, user *ent.User, id int) (bool, error) {
	if id == 0 {
		return false, nil
	}
	var (
		share *ent.Share
		err   error
	)
	if user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		share, err = dep.ShareClient().GetByID(ctx, id)
	} else {
		share, err = dep.ShareClient().GetByIDUser(ctx, id, user.ID)
	}
	if err != nil {
		return false, serializer.NewError(serializer.CodeNotFound, "share not found", err)
	}
	return share.IsDefault, nil
}

func countDefaultShares(ctx context.Context, dep dependency.Dep) (int, error) {
	res, err := dep.ShareClient().List(ctx, &inventory.ListShareArgs{DefaultOnly: true, PaginationArgs: &inventory.PaginationArgs{PageSize: maxDefaultShares}})
	if err != nil {
		return 0, err
	}
	return res.PaginationResults.TotalItems, nil
}

func DeleteShare(c *gin.Context, shareId int) error {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	shareClient := dep.ShareClient()

	ctx := context.WithValue(c, inventory.LoadShareFile{}, true)
	var (
		share *ent.Share
		err   error
	)
	if user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		share, err = shareClient.GetByID(ctx, shareId)
	} else {
		share, err = shareClient.GetByIDUser(ctx, shareId, user.ID)
	}
	if err != nil {
		return serializer.NewError(serializer.CodeNotFound, "share not found", err)
	}

	if err := userservice.CleanupDefaultShareShortcuts(c, dep, share); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to reconcile default shares", err)
	}
	if err := shareClient.Delete(c, share.ID); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete share", err)
	}

	return nil
}
