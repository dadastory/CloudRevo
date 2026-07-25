package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dadastory/CloudRevo/application/constants"
	"github.com/dadastory/CloudRevo/application/dependency"
	"github.com/dadastory/CloudRevo/ent"
	settingmodel "github.com/dadastory/CloudRevo/ent/setting"
	"github.com/dadastory/CloudRevo/ent/share"
	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/cache"
	"github.com/dadastory/CloudRevo/pkg/conf"
	"github.com/dadastory/CloudRevo/pkg/filemanager/fs"
	"github.com/dadastory/CloudRevo/pkg/filemanager/manager"
	"github.com/dadastory/CloudRevo/pkg/hashid"
	"github.com/dadastory/CloudRevo/pkg/logging"
	"github.com/dadastory/CloudRevo/routers/controllers"
	adminservice "github.com/dadastory/CloudRevo/service/admin"
	"github.com/dadastory/CloudRevo/service/explorer"
	shareservice "github.com/dadastory/CloudRevo/service/share"
	userservice "github.com/dadastory/CloudRevo/service/user"
	"github.com/gin-gonic/gin"
)

type sharedAccessTestConfig struct {
	database conf.Database
	system   conf.System
	cors     conf.Cors
}

func (c sharedAccessTestConfig) Database() *conf.Database        { return &c.database }
func (c sharedAccessTestConfig) System() *conf.System            { return &c.system }
func (c sharedAccessTestConfig) SSL() *conf.SSL                  { return &conf.SSL{} }
func (c sharedAccessTestConfig) Unix() *conf.Unix                { return &conf.Unix{} }
func (c sharedAccessTestConfig) Slave() *conf.Slave              { return &conf.Slave{} }
func (c sharedAccessTestConfig) Redis() *conf.Redis              { return &conf.Redis{} }
func (c sharedAccessTestConfig) Cors() *conf.Cors                { return &c.cors }
func (c sharedAccessTestConfig) OptionOverwrite() map[string]any { return map[string]any{} }

func TestAnonymousSharePreparesUploadOverHTTP(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	dep := newSharedAccessTestDependency(t)
	ctx := context.Background()
	owner, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{
		Email: "owner@example.test", Nick: "owner", Status: "active", GroupID: 2,
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	owner, err = dep.UserClient().GetLoginUserByID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}
	fm := manager.NewFileManager(dep, owner)
	root, err := fs.NewUriFromString(fs.NewMyUri(hashid.EncodeUserID(dep.HashIDEncoder(), owner.ID)))
	if err != nil {
		t.Fatalf("create owner root: %v", err)
	}
	folder, err := fm.Create(ctx, root.Join("shared"), types.FileTypeFolder)
	if err != nil {
		t.Fatalf("create share folder: %v", err)
	}
	if err := fm.PatchShareAccessRule(ctx, folder.Uri(true), &types.ShareAccessRule{Anonymous: types.SharePermission{Read: true, Create: true}}); err != nil {
		t.Fatalf("set share folder permissions: %v", err)
	}
	share, err := fm.CreateOrUpdateShare(ctx, folder.Uri(true), &manager.CreateShareArgs{})
	fm.Recycle()
	if err != nil {
		t.Fatalf("create share: %v", err)
	}

	router := anonymousUploadRouter(t, dep)
	uri := fs.NewShareUri(hashid.EncodeShareID(dep.HashIDEncoder(), share.ID), "") + "/created.txt"
	payload, _ := json.Marshal(map[string]any{"uri": uri, "size": 5, "mime_type": "text/plain"})
	req := httptest.NewRequest(http.MethodPut, "/api/v4/file/upload", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	var response struct {
		Code int `json:"code"`
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	if response.Code != 0 || response.Data.SessionID == "" {
		t.Fatalf("anonymous upload preparation must succeed: code=%d body=%s", response.Code, recorder.Body.String())
	}

	chunk := httptest.NewRequest(http.MethodPost, "/api/v4/file/upload/"+response.Data.SessionID+"/0", bytes.NewBufferString("hello"))
	chunk.Header.Set("Content-Type", "application/octet-stream")
	chunk.Header.Set("Content-Length", "5")
	chunkRecorder := httptest.NewRecorder()
	router.ServeHTTP(chunkRecorder, chunk)
	var chunkResponse struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(chunkRecorder.Body.Bytes(), &chunkResponse); err != nil {
		t.Fatalf("decode upload completion response: %v; body=%s", err, chunkRecorder.Body.String())
	}
	if chunkResponse.Code != 0 {
		t.Fatalf("anonymous upload completion must succeed: code=%d body=%s", chunkResponse.Code, chunkRecorder.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v4/file?uri="+url.QueryEscape(fs.NewShareUri(hashid.EncodeShareID(dep.HashIDEncoder(), share.ID), "")), nil)
	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, list)
	if !bytes.Contains(listRecorder.Body.Bytes(), []byte("created.txt")) {
		t.Fatalf("completed anonymous upload must be visible through the share: body=%s", listRecorder.Body.String())
	}
}

func TestAnonymousShareRejectsUploadWithoutCreatePermission(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	dep := newSharedAccessTestDependency(t)
	ctx := context.Background()
	owner, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{
		Email: "read-only-owner@example.test", Nick: "owner", Status: "active", GroupID: 2,
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	owner, err = dep.UserClient().GetLoginUserByID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}
	fm := manager.NewFileManager(dep, owner)
	root, err := fs.NewUriFromString(fs.NewMyUri(hashid.EncodeUserID(dep.HashIDEncoder(), owner.ID)))
	if err != nil {
		t.Fatalf("create owner root: %v", err)
	}
	folder, err := fm.Create(ctx, root.Join("read-only"), types.FileTypeFolder)
	if err != nil {
		t.Fatalf("create share folder: %v", err)
	}
	if err := fm.PatchShareAccessRule(ctx, folder.Uri(true), &types.ShareAccessRule{Anonymous: types.SharePermission{Read: true}}); err != nil {
		t.Fatalf("set share folder permissions: %v", err)
	}
	share, err := fm.CreateOrUpdateShare(ctx, folder.Uri(true), &manager.CreateShareArgs{})
	fm.Recycle()
	if err != nil {
		t.Fatalf("create share: %v", err)
	}

	router := anonymousUploadRouter(t, dep)
	uri := fs.NewShareUri(hashid.EncodeShareID(dep.HashIDEncoder(), share.ID), "") + "/denied.txt"
	payload, _ := json.Marshal(map[string]any{"uri": uri, "size": 5, "mime_type": "text/plain"})
	req := httptest.NewRequest(http.MethodPut, "/api/v4/file/upload", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	var response struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	if response.Code == 0 {
		t.Fatalf("anonymous upload preparation must be denied without create permission: body=%s", recorder.Body.String())
	}
}

func TestDefaultShareShortcutsAreRemovedAfterSourceRuleRevocation(t *testing.T) {
	dep := newSharedAccessTestDependency(t)
	ctx := context.Background()
	owner, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "default-owner@example.test", Nick: "owner", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	target, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "default-target@example.test", Nick: "target", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	owner, err = dep.UserClient().GetLoginUserByID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}

	fm := manager.NewFileManager(dep, owner)
	defer fm.Recycle()
	root, err := fs.NewUriFromString(fs.NewMyUri(hashid.EncodeUserID(dep.HashIDEncoder(), owner.ID)))
	if err != nil {
		t.Fatalf("create owner root URI: %v", err)
	}
	folder, err := fm.Create(ctx, root.Join("default-shared"), types.FileTypeFolder)
	if err != nil {
		t.Fatalf("create default share folder: %v", err)
	}
	if err := fm.PatchShareAccessRule(ctx, folder.Uri(true), &types.ShareAccessRule{Authenticated: types.SharePermission{Read: true}}); err != nil {
		t.Fatalf("grant default share access: %v", err)
	}
	share, err := fm.CreateOrUpdateShare(ctx, folder.Uri(true), &manager.CreateShareArgs{Default: true})
	if err != nil {
		t.Fatalf("create default share: %v", err)
	}
	share, err = dep.ShareClient().GetByID(context.WithValue(ctx, inventory.LoadShareFile{}, true), share.ID)
	if err != nil {
		t.Fatalf("load default share: %v", err)
	}
	if err := userservice.ReconcileDefaultShareShortcuts(ctx, dep, share); err != nil {
		t.Fatalf("create default shortcuts: %v", err)
	}

	targetRoot, err := dep.FileClient().Root(ctx, target)
	if err != nil {
		t.Fatalf("load target root: %v", err)
	}
	shortcutName := "default-shared (" + fmt.Sprint(share.ID) + ")"
	if _, err := dep.FileClient().GetChildFile(ctx, targetRoot, target.ID, shortcutName, false); err != nil {
		t.Fatalf("default shortcut must exist before revocation: %v", err)
	}
	target, err = dep.UserClient().GetLoginUserByID(ctx, target.ID)
	if err != nil {
		t.Fatalf("load default-share recipient: %v", err)
	}
	targetFM := manager.NewFileManager(dep, target)
	defer targetFM.Recycle()
	sharedWithMe, err := fs.NewUriFromString("cloudrevo://shared_with_me")
	if err != nil {
		t.Fatalf("create shared-with-me URI: %v", err)
	}
	_, listed, err := targetFM.List(ctx, sharedWithMe, &manager.ListArgs{PageSize: 100})
	if err != nil {
		t.Fatalf("list shared-with-me shortcuts: %v", err)
	}
	if !containsFileName(listed.Files, shortcutName) {
		t.Fatalf("eligible default shortcut must be visible in shared-with-me listing: %#v", listed.Files)
	}
	if err := fm.PatchShareAccessRule(ctx, folder.Uri(true), &types.ShareAccessRule{Authenticated: types.SharePermission{}}); err != nil {
		t.Fatalf("revoke default share access: %v", err)
	}
	_, listed, err = targetFM.List(ctx, sharedWithMe, &manager.ListArgs{PageSize: 100})
	if err != nil {
		t.Fatalf("list shared-with-me shortcuts after revocation: %v", err)
	}
	if containsFileName(listed.Files, shortcutName) {
		t.Fatalf("revoked default shortcut must be hidden before background cleanup: %#v", listed.Files)
	}
	if listed.Pagination.TotalItems != 0 {
		t.Fatalf("hidden default shortcuts must not leak through pagination totals: %#v", listed.Pagination)
	}
	if err := userservice.ReconcileDefaultShareShortcuts(ctx, dep, share); err != nil {
		t.Fatalf("reconcile revoked default share: %v", err)
	}
	if _, err := dep.FileClient().GetChildFile(ctx, targetRoot, target.ID, shortcutName, false); !ent.IsNotFound(err) {
		t.Fatalf("revoked default shortcut must be removed, got %v", err)
	}
}

func TestSharedWithMeSearchStreamsNormalizedShortcutURI(t *testing.T) {
	dep := newSharedAccessTestDependency(t)
	ctx := context.Background()
	if _, err := dep.DBClient().Setting.Update().Where(settingmodel.NameEQ("use_sse_for_search")).SetValue("1").Save(ctx); err != nil {
		t.Fatalf("enable SSE search: %v", err)
	}
	if !dep.SettingProvider().DBFS(ctx).UseSSEForSearch {
		t.Fatal("test setup must enable SSE search")
	}

	owner, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "sse-owner@example.test", Nick: "owner", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	target, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "sse-target@example.test", Nick: "target", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	owner, err = dep.UserClient().GetLoginUserByID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}
	fm := manager.NewFileManager(dep, owner)
	defer fm.Recycle()
	root, err := fs.NewUriFromString(fs.NewMyUri(hashid.EncodeUserID(dep.HashIDEncoder(), owner.ID)))
	if err != nil {
		t.Fatalf("create owner root URI: %v", err)
	}
	folder, err := fm.Create(ctx, root.Join("sse-default-shared"), types.FileTypeFolder)
	if err != nil {
		t.Fatalf("create shared folder: %v", err)
	}
	if err := fm.PatchShareAccessRule(ctx, folder.Uri(true), &types.ShareAccessRule{Authenticated: types.SharePermission{Read: true}}); err != nil {
		t.Fatalf("grant read access: %v", err)
	}
	share, err := fm.CreateOrUpdateShare(ctx, folder.Uri(true), &manager.CreateShareArgs{Default: true})
	if err != nil {
		t.Fatalf("create default share: %v", err)
	}
	share, err = dep.ShareClient().GetByID(context.WithValue(ctx, inventory.LoadShareFile{}, true), share.ID)
	if err != nil {
		t.Fatalf("load share source: %v", err)
	}
	if err := userservice.ReconcileDefaultShareShortcuts(ctx, dep, share); err != nil {
		t.Fatalf("reconcile default shortcut: %v", err)
	}
	target, err = dep.UserClient().GetLoginUserByID(ctx, target.ID)
	if err != nil {
		t.Fatalf("load target: %v", err)
	}
	targetFM := manager.NewFileManager(dep, target)
	defer targetFM.Recycle()
	uri, err := fs.NewUriFromString("cloudrevo://shared_with_me?name=sse-default-shared")
	if err != nil {
		t.Fatalf("create shared-with-me search URI: %v", err)
	}
	streamed := make([]fs.File, 0, 1)
	_, _, err = targetFM.List(ctx, uri, &manager.ListArgs{PageSize: 10, StreamResponseCallback: func(_ fs.File, files []fs.File) {
		streamed = append(streamed, files...)
	}})
	if err != nil {
		t.Fatalf("stream shared-with-me search: %v", err)
	}
	if len(streamed) != 1 {
		t.Fatalf("expected one streamed shortcut, got %d", len(streamed))
	}
	streamedURI := streamed[0].Uri(false)
	if streamedURI == nil || streamedURI.FileSystem() != constants.FileSystemSharedWithMe {
		t.Fatalf("streamed shortcut must already have a shared-with-me URI, got %#v", streamedURI)
	}
}

func TestDefaultShareReservationCapsConcurrentTransactions(t *testing.T) {
	dep := newSharedAccessTestDependency(t)
	ctx := context.Background()
	owner, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "cap-owner@example.test", Nick: "owner", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	root, err := dep.FileClient().CreateFolder(ctx, nil, &inventory.CreateFolderParameters{Owner: owner.ID, Name: inventory.RootFolderName})
	if err != nil {
		t.Fatalf("create owner root: %v", err)
	}
	for i := 0; i < 19; i++ {
		file, err := dep.FileClient().CreateFolder(ctx, root, &inventory.CreateFolderParameters{Owner: owner.ID, Name: fmt.Sprintf("default-%d", i)})
		if err != nil {
			t.Fatalf("create default source %d: %v", i, err)
		}
		if _, err := dep.DBClient().Share.Create().SetUserID(owner.ID).SetFileID(file.ID).SetIsDefault(true).Save(ctx); err != nil {
			t.Fatalf("create default share %d: %v", i, err)
		}
	}

	candidates := make([]int, 0, 2)
	for i := 0; i < 2; i++ {
		file, err := dep.FileClient().CreateFolder(ctx, root, &inventory.CreateFolderParameters{Owner: owner.ID, Name: fmt.Sprintf("candidate-%d", i)})
		if err != nil {
			t.Fatalf("create candidate source %d: %v", i, err)
		}
		share, err := dep.DBClient().Share.Create().SetUserID(owner.ID).SetFileID(file.ID).Save(ctx)
		if err != nil {
			t.Fatalf("create candidate share %d: %v", i, err)
		}
		candidates = append(candidates, share.ID)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	limitErrors := 0
	for _, candidate := range candidates {
		wg.Add(1)
		go func(shareID int) {
			defer wg.Done()
			<-start
			tx, err := shareservice.ReserveDefaultShareSlot(ctx, dep)
			if err != nil {
				mu.Lock()
				limitErrors++
				mu.Unlock()
				return
			}
			defer tx.Rollback()
			if err := tx.Share.UpdateOneID(shareID).SetIsDefault(true).Exec(ctx); err != nil {
				t.Errorf("set default share %d: %v", shareID, err)
				return
			}
			if err := tx.Commit(); err != nil {
				t.Errorf("commit default share %d: %v", shareID, err)
				return
			}
			mu.Lock()
			successes++
			mu.Unlock()
		}(candidate)
	}
	close(start)
	wg.Wait()

	count, err := dep.DBClient().Share.Query().Where(share.IsDefaultEQ(true)).Count(ctx)
	if err != nil {
		t.Fatalf("count default shares: %v", err)
	}
	if count != 20 || successes != 1 || limitErrors != 1 {
		t.Fatalf("concurrent reservation must admit exactly one final slot: count=%d successes=%d errors=%d", count, successes, limitErrors)
	}
}

func TestSharedFolderListingContinuesPastDeniedPhysicalPage(t *testing.T) {
	dep := newSharedAccessTestDependency(t)
	ctx := context.Background()
	owner, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "page-owner@example.test", Nick: "owner", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	visitor, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "page-visitor@example.test", Nick: "visitor", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create visitor: %v", err)
	}
	if _, err := dep.FileClient().CreateFolder(ctx, nil, &inventory.CreateFolderParameters{Owner: owner.ID, Name: inventory.RootFolderName}); err != nil {
		t.Fatalf("create owner root: %v", err)
	}
	owner, err = dep.UserClient().GetLoginUserByID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}
	ownerFM := manager.NewFileManager(dep, owner)
	defer ownerFM.Recycle()
	ownerRoot, err := fs.NewUriFromString(fs.NewMyUri(hashid.EncodeUserID(dep.HashIDEncoder(), owner.ID)))
	if err != nil {
		t.Fatalf("create owner root URI: %v", err)
	}
	folder, err := ownerFM.Create(ctx, ownerRoot.Join("shared"), types.FileTypeFolder)
	if err != nil {
		t.Fatalf("create shared folder: %v", err)
	}
	if err := ownerFM.PatchShareAccessRule(ctx, folder.Uri(true), &types.ShareAccessRule{Authenticated: types.SharePermission{Read: true}}); err != nil {
		t.Fatalf("grant source read: %v", err)
	}
	hidden, err := ownerFM.Create(ctx, folder.Uri(true).Join("hidden"), types.FileTypeFile)
	if err != nil {
		t.Fatalf("create hidden child: %v", err)
	}
	if err := ownerFM.PatchShareAccessRule(ctx, hidden.Uri(true), &types.ShareAccessRule{Users: map[int]types.SharePermission{visitor.ID: {}}}); err != nil {
		t.Fatalf("deny visitor on child: %v", err)
	}
	if _, err := ownerFM.Create(ctx, folder.Uri(true).Join("visible"), types.FileTypeFile); err != nil {
		t.Fatalf("create visible child: %v", err)
	}
	share, err := ownerFM.CreateOrUpdateShare(ctx, folder.Uri(true), &manager.CreateShareArgs{})
	if err != nil {
		t.Fatalf("create folder share: %v", err)
	}
	visitor, err = dep.UserClient().GetLoginUserByID(ctx, visitor.ID)
	if err != nil {
		t.Fatalf("load visitor: %v", err)
	}
	visitorFM := manager.NewFileManager(dep, visitor)
	defer visitorFM.Recycle()
	shareURI, err := fs.NewUriFromString(fs.NewShareUri(hashid.EncodeShareID(dep.HashIDEncoder(), share.ID), ""))
	if err != nil {
		t.Fatalf("create share URI: %v", err)
	}
	_, listed, err := visitorFM.List(ctx, shareURI, &manager.ListArgs{PageSize: 1})
	if err != nil {
		t.Fatalf("list shared folder: %v", err)
	}
	if len(listed.Files) != 1 || listed.Files[0].Name() != "visible" || !listed.Pagination.IsCursor || listed.Pagination.TotalItems != 0 {
		t.Fatalf("first visible page must skip denied child without losing later child: %#v %#v", listed.Files, listed.Pagination)
	}
}

func TestSourceRuleChangeRestartsAffectedDefaultShareReconciliation(t *testing.T) {
	dep := newSharedAccessTestDependency(t)
	ctx := context.Background()
	owner, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "restart-owner@example.test", Nick: "owner", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	owner, err = dep.UserClient().GetLoginUserByID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}

	fm := manager.NewFileManager(dep, owner)
	defer fm.Recycle()
	root, err := fs.NewUriFromString(fs.NewMyUri(hashid.EncodeUserID(dep.HashIDEncoder(), owner.ID)))
	if err != nil {
		t.Fatalf("create owner root URI: %v", err)
	}
	folder, err := fm.Create(ctx, root.Join("restart-default"), types.FileTypeFolder)
	if err != nil {
		t.Fatalf("create default folder: %v", err)
	}
	share, err := fm.CreateOrUpdateShare(ctx, folder.Uri(true), &manager.CreateShareArgs{Default: true})
	if err != nil {
		t.Fatalf("create default share: %v", err)
	}
	key := "default_share_reconcile_page_" + fmt.Sprint(share.ID)
	if err := dep.KV().Set(key, 4, 0); err != nil {
		t.Fatalf("advance default-share cursor: %v", err)
	}

	if err := fm.PatchShareAccessRule(ctx, folder.Uri(true), &types.ShareAccessRule{Authenticated: types.SharePermission{Read: true}}); err != nil {
		t.Fatalf("save source rule: %v", err)
	}
	if value, ok := dep.KV().Get(key); !ok || value != 0 {
		t.Fatalf("source-rule save must restart default-share reconciliation from page zero, got %#v (present=%t)", value, ok)
	}
}

func TestExpiredDefaultShareShortcutIsRemovedByReconciliation(t *testing.T) {
	dep := newSharedAccessTestDependency(t)
	ctx := context.Background()
	owner, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "expired-owner@example.test", Nick: "owner", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	target, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "expired-target@example.test", Nick: "target", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	owner, err = dep.UserClient().GetLoginUserByID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}
	fm := manager.NewFileManager(dep, owner)
	defer fm.Recycle()
	root, err := fs.NewUriFromString(fs.NewMyUri(hashid.EncodeUserID(dep.HashIDEncoder(), owner.ID)))
	if err != nil {
		t.Fatalf("create owner root URI: %v", err)
	}
	folder, err := fm.Create(ctx, root.Join("expired-default"), types.FileTypeFolder)
	if err != nil {
		t.Fatalf("create default folder: %v", err)
	}
	if err := fm.PatchShareAccessRule(ctx, folder.Uri(true), &types.ShareAccessRule{Authenticated: types.SharePermission{Read: true}}); err != nil {
		t.Fatalf("grant default access: %v", err)
	}
	share, err := fm.CreateOrUpdateShare(ctx, folder.Uri(true), &manager.CreateShareArgs{Default: true})
	if err != nil {
		t.Fatalf("create default share: %v", err)
	}
	share, err = dep.ShareClient().GetByID(context.WithValue(ctx, inventory.LoadShareFile{}, true), share.ID)
	if err != nil {
		t.Fatalf("load default share: %v", err)
	}
	if err := userservice.ReconcileDefaultShareShortcuts(ctx, dep, share); err != nil {
		t.Fatalf("provision shortcut: %v", err)
	}
	targetRoot, err := dep.FileClient().Root(ctx, target)
	if err != nil {
		t.Fatalf("load target root: %v", err)
	}
	shortcutName := "expired-default (" + fmt.Sprint(share.ID) + ")"
	if _, err := dep.FileClient().GetChildFile(ctx, targetRoot, target.ID, shortcutName, false); err != nil {
		t.Fatalf("default shortcut must exist before expiry: %v", err)
	}
	expires := time.Now().Add(-time.Second)
	share, err = dep.ShareClient().Upsert(ctx, &inventory.CreateShareParams{Existed: share, Expires: &expires, Props: share.Props})
	if err != nil {
		t.Fatalf("expire default share: %v", err)
	}
	if err := userservice.ReconcileDefaultShareShortcuts(ctx, dep, share); err != nil {
		t.Fatalf("reconcile expired shortcut: %v", err)
	}
	if _, err := dep.FileClient().GetChildFile(ctx, targetRoot, target.ID, shortcutName, false); !ent.IsNotFound(err) {
		t.Fatalf("expired default shortcut must be removed, got %v", err)
	}
}

func TestAdministratorBatchDeleteRemovesDefaultShareShortcuts(t *testing.T) {
	dep := newSharedAccessTestDependency(t)
	ctx := context.Background()
	owner, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "batch-owner@example.test", Nick: "owner", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	target, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{Email: "batch-target@example.test", Nick: "target", Status: "active", GroupID: 2})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	owner, err = dep.UserClient().GetLoginUserByID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}
	fm := manager.NewFileManager(dep, owner)
	defer fm.Recycle()
	root, err := fs.NewUriFromString(fs.NewMyUri(hashid.EncodeUserID(dep.HashIDEncoder(), owner.ID)))
	if err != nil {
		t.Fatalf("create owner root URI: %v", err)
	}
	folder, err := fm.Create(ctx, root.Join("batch-default"), types.FileTypeFolder)
	if err != nil {
		t.Fatalf("create default folder: %v", err)
	}
	if err := fm.PatchShareAccessRule(ctx, folder.Uri(true), &types.ShareAccessRule{Authenticated: types.SharePermission{Read: true}}); err != nil {
		t.Fatalf("grant default access: %v", err)
	}
	share, err := fm.CreateOrUpdateShare(ctx, folder.Uri(true), &manager.CreateShareArgs{Default: true})
	if err != nil {
		t.Fatalf("create default share: %v", err)
	}
	share, err = dep.ShareClient().GetByID(context.WithValue(ctx, inventory.LoadShareFile{}, true), share.ID)
	if err != nil {
		t.Fatalf("load default share: %v", err)
	}
	if err := userservice.ReconcileDefaultShareShortcuts(ctx, dep, share); err != nil {
		t.Fatalf("provision shortcut: %v", err)
	}
	targetRoot, err := dep.FileClient().Root(ctx, target)
	if err != nil {
		t.Fatalf("load target root: %v", err)
	}
	shortcutName := "batch-default (" + fmt.Sprint(share.ID) + ")"
	if _, err := dep.FileClient().GetChildFile(ctx, targetRoot, target.ID, shortcutName, false); err != nil {
		t.Fatalf("default shortcut must exist before deletion: %v", err)
	}
	var deleteErr error
	router := gin.New()
	router.ContextWithFallback = true
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), dependency.DepCtx{}, dep))
		c.Next()
	})
	router.DELETE("/admin/shares", func(c *gin.Context) {
		deleteErr = (&adminservice.BatchShareService{ShareIDs: []int{share.ID}}).Delete(c)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/admin/shares", nil))
	if deleteErr != nil {
		t.Fatalf("administrator batch delete: %v", deleteErr)
	}
	if _, err := dep.FileClient().GetChildFile(ctx, targetRoot, target.ID, shortcutName, false); !ent.IsNotFound(err) {
		t.Fatalf("batch-deleted default shortcut must be removed, got %v", err)
	}
}

func TestAdministratorCanDesignateDefaultButOwnerCannotClearIt(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	dep := newSharedAccessTestDependency(t)
	ctx := context.Background()
	adminUser, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{
		Email: "default-admin@example.test", Nick: "admin", Status: "active", GroupID: 1,
	})
	if err != nil {
		t.Fatalf("create administrator: %v", err)
	}
	adminUser, err = dep.UserClient().GetLoginUserByID(ctx, adminUser.ID)
	if err != nil {
		t.Fatalf("load administrator: %v", err)
	}
	owner, err := dep.UserClient().Create(ctx, &inventory.NewUserArgs{
		Email: "default-owner@example.test", Nick: "owner", Status: "active", GroupID: 2,
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	owner, err = dep.UserClient().GetLoginUserByID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}
	if owner.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		t.Fatal("test owner must be a non-administrator")
	}

	fm := manager.NewFileManager(dep, owner)
	defer fm.Recycle()
	root, err := fs.NewUriFromString(fs.NewMyUri(hashid.EncodeUserID(dep.HashIDEncoder(), owner.ID)))
	if err != nil {
		t.Fatalf("create owner root URI: %v", err)
	}
	folder, err := fm.Create(ctx, root.Join("admin-default"), types.FileTypeFolder)
	if err != nil {
		t.Fatalf("create default source: %v", err)
	}
	created, err := fm.CreateOrUpdateShare(ctx, folder.Uri(true), &manager.CreateShareArgs{})
	if err != nil {
		t.Fatalf("create share: %v", err)
	}

	if err := invokeDefaultShareService(dep, adminUser, &adminservice.DefaultShareService{Default: true}, created.ID); err != nil {
		t.Fatalf("administrator must be able to designate a valid share: %v", err)
	}
	updated, err := dep.ShareClient().GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("load designated share: %v", err)
	}
	if !updated.IsDefault || updated.Props == nil || !updated.Props.Default {
		t.Fatalf("administrator designation must synchronize both markers: %#v", updated)
	}

	if _, err := invokeShareUpsertService(dep, owner, &shareservice.ShareCreateService{Uri: folder.Uri(true).String()}, created.ID); err == nil {
		t.Fatal("ordinary owner edit must not clear administrator-managed default share")
	}
	unchanged, err := dep.ShareClient().GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("reload designated share: %v", err)
	}
	if !unchanged.IsDefault || unchanged.Props == nil || !unchanged.Props.Default {
		t.Fatalf("owner rejection must retain both default markers: %#v", unchanged)
	}
}

func invokeDefaultShareService(dep dependency.Dep, user *ent.User, service *adminservice.DefaultShareService, shareID int) error {
	var serviceErr error
	router := gin.New()
	router.ContextWithFallback = true
	router.Use(shareServiceContextMiddleware(dep, user))
	router.POST("/", func(c *gin.Context) { serviceErr = service.SetDefault(c, shareID) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
	return serviceErr
}

func invokeShareUpsertService(dep dependency.Dep, user *ent.User, service *shareservice.ShareCreateService, shareID int) (string, error) {
	var (
		link       string
		serviceErr error
	)
	router := gin.New()
	router.ContextWithFallback = true
	router.Use(shareServiceContextMiddleware(dep, user))
	router.POST("/", func(c *gin.Context) { link, serviceErr = service.Upsert(c, shareID) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
	return link, serviceErr
}

func shareServiceContextMiddleware(dep dependency.Dep, user *ent.User) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), dependency.DepCtx{}, dep)
		ctx = context.WithValue(ctx, inventory.UserCtx{}, user)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func containsFileName(files []fs.File, name string) bool {
	for _, file := range files {
		if file.Name() == name {
			return true
		}
	}
	return false
}

func newSharedAccessTestDependency(t *testing.T) dependency.Dep {
	t.Helper()
	logger := logging.NewConsoleLogger(logging.LevelError)
	kv := cache.NewMemoStore("", logger)
	config := sharedAccessTestConfig{
		database: conf.Database{Type: conf.SQLiteDB, DBFile: filepath.Join(t.TempDir(), "shared-access.db")},
		system:   conf.System{Mode: conf.MasterMode, SessionSecret: "test-session", LogLevel: "error"},
		cors:     conf.Cors{AllowOrigins: []string{"UNSET"}},
	}
	raw, err := inventory.NewRawEntClient(logger, config)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	client, err := inventory.InitializeDBClient(logger, raw, kv, constants.BackendVersion)
	if err != nil {
		t.Fatalf("initialize database: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return dependency.NewDependency(
		dependency.WithConfigProvider(config),
		dependency.WithDbClient(client),
		dependency.WithKV(kv),
	)
}

func anonymousUploadRouter(t *testing.T, dep dependency.Dep) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.ContextWithFallback = true
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), dependency.DepCtx{}, dep)
		anonymous, err := dep.UserClient().GetLoginUserByID(ctx, 0)
		if err != nil {
			t.Fatalf("load anonymous user: %v", err)
		}
		c.Request = c.Request.WithContext(context.WithValue(ctx, inventory.UserCtx{}, anonymous))
		c.Next()
	})
	router.PUT("/api/v4/file/upload",
		controllers.FromJSON[explorer.CreateUploadSessionService](explorer.CreateUploadSessionParameterCtx{}),
		controllers.CreateUploadSession,
	)
	router.POST("/api/v4/file/upload/:sessionId/:index",
		controllers.FromUri[explorer.UploadService](explorer.UploadParameterCtx{}),
		controllers.FileUpload,
	)
	router.GET("/api/v4/file",
		controllers.FromQuery[explorer.ListFileService](explorer.ListFileParameterCtx{}),
		controllers.ListDirectory,
	)
	return router
}
