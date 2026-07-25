package dbfs

import (
	"testing"

	"github.com/dadastory/CloudRevo/ent"
	usermodel "github.com/dadastory/CloudRevo/ent/user"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/filemanager/fs"
	"github.com/dadastory/CloudRevo/pkg/filemanager/lock"
	"github.com/dadastory/CloudRevo/pkg/setting"
)

func TestShareNavigatorWithoutGlobalAccessRuleIsReadOnly(t *testing.T) {
	navigator := &shareNavigator{
		user:   &ent.User{ID: 1},
		share:  &ent.Share{Props: &types.ShareProps{}},
		config: &setting.DBFS{},
	}

	fileSystem := &DBFS{user: &ent.User{ID: 1}}
	foreignFile := &File{Model: &ent.File{OwnerID: 2}}
	if fileSystem.canAccessWithNavigator(navigator, foreignFile, NavigatorCapabilityCreateFile) {
		t.Fatal("a share without an access rule must not authorize a collaborator")
	}

	for _, capabilityID := range []NavigatorCapability{
		NavigatorCapabilityCreateFile,
		NavigatorCapabilityUploadFile,
		NavigatorCapabilityRenameFile,
		NavigatorCapabilitySoftDelete,
		NavigatorCapabilityVersionControl,
		NavigatorCapabilityModifyProps,
	} {
		if navigator.Capabilities(false).Capability.Enabled(int(capabilityID)) {
			t.Fatalf("a share without an access rule must not expose capability %v", capabilityID)
		}
	}
}

func TestShareNavigatorAllowsExplicitAnonymousCreatePermission(t *testing.T) {
	navigator := &shareNavigator{
		user: &ent.User{},
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Anonymous: types.SharePermission{Read: true, Create: true},
		}}}}},
	}

	capability := navigator.Capabilities(false).Capability
	if !capability.Enabled(int(NavigatorCapabilityCreateFile)) {
		t.Fatal("an explicit anonymous create grant must expose create capability")
	}
	if !capability.Enabled(int(NavigatorCapabilityUploadFile)) {
		t.Fatal("an explicit anonymous create grant must expose upload capability")
	}
	if capability.Enabled(int(NavigatorCapabilityRenameFile)) || capability.Enabled(int(NavigatorCapabilitySoftDelete)) {
		t.Fatal("anonymous create grant must not imply update or delete capability")
	}
}

func TestShareNavigatorAppliesGroupGrantWithoutAddingOtherMutations(t *testing.T) {
	navigator := &shareNavigator{
		user: &ent.User{ID: 9, Edges: ent.UserEdges{Group: &ent.Group{ID: 2}}},
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Groups: map[int]types.SharePermission{2: {Read: true, Update: true}},
		}}}}},
	}

	capability := navigator.Capabilities(false).Capability
	if !capability.Enabled(int(NavigatorCapabilityListChildren)) || !capability.Enabled(int(NavigatorCapabilityRenameFile)) {
		t.Fatal("group read/update grant must expose list and rename capabilities")
	}
	if capability.Enabled(int(NavigatorCapabilityCreateFile)) || capability.Enabled(int(NavigatorCapabilityDeleteFile)) {
		t.Fatal("group update grant must not imply create or delete capability")
	}
}

func TestShareNavigatorUsesSavedSourceRootRuleForEveryAudience(t *testing.T) {
	rule := &types.ShareAccessRule{
		Anonymous:     types.SharePermission{Read: true, Update: true},
		Authenticated: types.SharePermission{Read: true},
		Users: map[int]types.SharePermission{
			7: {Read: true, Update: true},
		},
		Groups: map[int]types.SharePermission{
			2: {Read: true, Create: true},
		},
	}

	cases := []struct {
		name string
		user *ent.User
		want types.SharePermission
	}{
		{
			name: "anonymous update",
			user: &ent.User{},
			want: types.SharePermission{Read: true, Update: true},
		},
		{
			name: "direct user update",
			user: &ent.User{ID: 7, Edges: ent.UserEdges{Group: &ent.Group{ID: 2}}},
			want: types.SharePermission{Read: true, Update: true},
		},
		{
			name: "group create",
			user: &ent.User{ID: 8, Edges: ent.UserEdges{Group: &ent.Group{ID: 2}}},
			want: types.SharePermission{Read: true, Create: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			navigator := &shareNavigator{
				user: tc.user,
				share: &ent.Share{
					Props: &types.ShareProps{},
					Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: rule}}},
				},
			}

			if got := navigator.accessPermission(); got != tc.want {
				t.Fatalf("source-root rule must resolve %s, got %#v", tc.name, got)
			}
		})
	}
}

func TestShareNavigatorUsesSavedSourceRootRuleWithoutShareProps(t *testing.T) {
	navigator := &shareNavigator{
		user: &ent.User{},
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Anonymous: types.SharePermission{Read: true, Update: true},
		}}}}},
	}

	if got := navigator.accessPermission(); got != (types.SharePermission{Read: true, Update: true}) {
		t.Fatalf("source-root rule must apply even when legacy share props are absent, got %#v", got)
	}
}

func TestDefaultShareShortcutRequiresCurrentReadPermission(t *testing.T) {
	visitor := &ent.User{ID: 7, Edges: ent.UserEdges{Group: &ent.Group{ID: 2}}}
	owner := &ent.User{ID: 1, Status: usermodel.StatusActive}
	share := &ent.Share{
		IsDefault: true,
		Props:     &types.ShareProps{Default: true},
		Edges: ent.ShareEdges{User: owner, File: &ent.File{OwnerID: owner.ID, FileChildren: 1, Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Users: map[int]types.SharePermission{visitor.ID: {}},
		}}}},
	}
	if canListDefaultShareShortcut(visitor, share) {
		t.Fatal("a stale default shortcut must be hidden after read access is revoked")
	}
	share.Edges.File.Props.ShareAccessRule.Users[visitor.ID] = types.SharePermission{Read: true}
	if !canListDefaultShareShortcut(visitor, share) {
		t.Fatal("an eligible default shortcut must remain listable")
	}
}

func TestShareNavigatorUsesNearestDescendantAccessRule(t *testing.T) {
	navigator := &shareNavigator{
		user: &ent.User{ID: 7},
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Authenticated: types.SharePermission{Read: true, Create: true, Update: true, Delete: true},
		}}}}},
	}

	parent := &File{Model: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
		Authenticated: types.SharePermission{Read: true},
	}}}}
	target := &File{Model: &ent.File{}, Parent: parent}

	got := navigator.accessPermissionFor(target)
	if !got.Read || got.Create || got.Update || got.Delete {
		t.Fatalf("nearest descendant rule must replace root permission, got %#v", got)
	}
}

func TestShareNavigatorDescendantRuleReplacesSourceRule(t *testing.T) {
	navigator := &shareNavigator{
		user: &ent.User{ID: 7},
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Authenticated: types.SharePermission{Read: true},
		}}}}},
	}
	target := &File{Model: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
		Authenticated: types.SharePermission{Read: true, Update: true},
	}}}}

	got := navigator.accessPermissionFor(target)
	if !got.Read || !got.Update || got.Create || got.Delete {
		t.Fatalf("a descendant rule must replace the inherited source rule, got %#v", got)
	}
}

func TestShareNavigatorBuildsPartialCapabilitiesForDescendantRule(t *testing.T) {
	navigator := &shareNavigator{
		user: &ent.User{ID: 7},
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Authenticated: types.SharePermission{Read: true, Create: true, Update: true, Delete: true},
		}}}}},
	}
	target := &File{Model: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
		Authenticated: types.SharePermission{Read: true},
	}}}}

	capability := navigator.capabilitiesFor(target)
	if !capability.Enabled(int(NavigatorCapabilityListChildren)) {
		t.Fatal("read grant must retain list capability")
	}
	for _, capabilityID := range []NavigatorCapability{
		NavigatorCapabilityCreateFile,
		NavigatorCapabilityUploadFile,
		NavigatorCapabilityRenameFile,
		NavigatorCapabilityDeleteFile,
		NavigatorCapabilitySoftDelete,
	} {
		if capability.Enabled(int(capabilityID)) {
			t.Fatalf("read-only descendant must not expose capability %v", capabilityID)
		}
	}
}

func TestShareNavigatorRejectsMutationDeniedByDescendantRule(t *testing.T) {
	navigator := &shareNavigator{
		user: &ent.User{ID: 7},
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Authenticated: types.SharePermission{Read: true, Create: true, Update: true, Delete: true},
		}}}}},
	}
	target := &File{Model: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
		Authenticated: types.SharePermission{Read: true},
	}}}}

	if navigator.allows(target, NavigatorCapabilityRenameFile) {
		t.Fatal("read-only descendant must reject rename even if the share root allows updates")
	}
	if !navigator.allows(target, NavigatorCapabilityListChildren) {
		t.Fatal("read-only descendant must retain list access")
	}
}

func TestShareNavigatorOmitsReadDeniedDescendantFromListing(t *testing.T) {
	navigator := &shareNavigator{
		user: &ent.User{ID: 7},
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Authenticated: types.SharePermission{Read: true},
		}}}}},
	}
	allowed := &File{Model: &ent.File{ID: 1, Name: "visible"}}
	denied := &File{Model: &ent.File{ID: 2, Name: "hidden", Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
		Authenticated: types.SharePermission{},
	}}}}

	files := filterReadableShareChildren(navigator, []*File{allowed, denied})
	if len(files) != 1 || files[0].Model.ID != allowed.Model.ID {
		t.Fatalf("read-denied descendant must not be returned from a share listing, got %#v", files)
	}
}

func TestShareNavigatorDoesNotExposeGenericLockToShareVisitors(t *testing.T) {
	navigator := &shareNavigator{
		user: &ent.User{},
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Anonymous: types.SharePermission{Read: true, Update: true},
		}}}}},
	}

	if navigator.allows(&File{Model: &ent.File{}}, NavigatorCapabilityLockFile) {
		t.Fatal("share visitors must not receive a generic lock capability")
	}
}

func TestSharedWopiWriteLockRequiresUpdatePermission(t *testing.T) {
	target := &File{Model: &ent.File{}}
	viewer := lock.Application{Type: string(fs.ApplicationViewer)}
	other := lock.Application{Type: string(fs.ApplicationDAV)}

	updateNavigator := &shareNavigator{
		user: &ent.User{},
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Anonymous: types.SharePermission{Read: true, Update: true},
		}}}}},
	}
	if !allowsSharedWopiWriteLock(updateNavigator, target, viewer) {
		t.Fatal("an update-authorized share visitor must be able to acquire a WOPI write lock")
	}
	if allowsSharedWopiWriteLock(updateNavigator, target, other) {
		t.Fatal("a WOPI write lock exception must not apply to other applications")
	}

	readNavigator := &shareNavigator{
		user: &ent.User{},
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Anonymous: types.SharePermission{Read: true},
		}}}}},
	}
	if allowsSharedWopiWriteLock(readNavigator, target, viewer) {
		t.Fatal("a read-only share visitor must not acquire a WOPI write lock")
	}
}

func TestShareUploadCapabilityDistinguishesCreateAndUpdate(t *testing.T) {
	if got := shareUploadCapability(false); got != NavigatorCapabilityUploadFile {
		t.Fatalf("new upload must require create capability, got %v", got)
	}
	if got := shareUploadCapability(true); got != NavigatorCapabilityVersionControl {
		t.Fatalf("content update must require update capability, got %v", got)
	}
}

func TestSharedTransferPathsAllowInSubtreeOperations(t *testing.T) {

	source, err := fs.NewUriFromString(fs.NewShareUri("share-id", ""))
	if err != nil {
		t.Fatal(err)
	}
	source = source.Join("source.txt")
	destination, err := fs.NewUriFromString(fs.NewShareUri("share-id", ""))
	if err != nil {
		t.Fatal(err)
	}
	destination = destination.Join("destination")

	if !canMoveOrCopyTo(source, destination, true) {
		t.Fatal("copying between paths in the active shared filesystem must be considered for ACL authorization")
	}
	if !canMoveOrCopyTo(source, destination, false) {
		t.Fatal("moving between paths in the active shared filesystem must be considered for ACL authorization")
	}
}

func TestSharedOperationsDoNotRequireInternalLockAccess(t *testing.T) {
	cases := map[sharedOperation]NavigatorCapability{
		sharedOperationCreate:        NavigatorCapabilityCreateFile,
		sharedOperationUploadNew:     NavigatorCapabilityUploadFile,
		sharedOperationUploadReplace: NavigatorCapabilityVersionControl,
		sharedOperationRename:        NavigatorCapabilityRenameFile,
		sharedOperationMetadata:      NavigatorCapabilityUpdateMetadata,
		sharedOperationProps:         NavigatorCapabilityModifyProps,
		sharedOperationDelete:        NavigatorCapabilityDeleteFile,
		sharedOperationSoftDelete:    NavigatorCapabilitySoftDelete,
		sharedOperationCopySource:    NavigatorCapabilityDownloadFile,
	}

	for operation, expected := range cases {
		capabilities := sharedOperationCapabilities(operation)
		if len(capabilities) != 1 || capabilities[0] != expected {
			t.Fatalf("operation %d must require only %v, got %#v", operation, expected, capabilities)
		}
	}
}

func TestVersionControlRequiresDeleteOnlyForHistoryDeletion(t *testing.T) {
	selectCapabilities := versionControlCapabilities(false)
	if len(selectCapabilities) != 1 || selectCapabilities[0] != NavigatorCapabilityVersionControl {
		t.Fatalf("version selection must require only update capability, got %#v", selectCapabilities)
	}

	deleteCapabilities := versionControlCapabilities(true)
	if len(deleteCapabilities) != 2 ||
		deleteCapabilities[0] != NavigatorCapabilityVersionControl ||
		deleteCapabilities[1] != NavigatorCapabilityDeleteFile {
		t.Fatalf("version deletion must require update and delete capabilities, got %#v", deleteCapabilities)
	}
}

func TestSingleFileUpdateAllowsContentReplacementOnly(t *testing.T) {
	navigator := &shareNavigator{
		user:            &ent.User{},
		singleFileShare: true,
		share: &ent.Share{Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
			Anonymous: types.SharePermission{Read: true, Update: true},
		}}}}},
	}
	fileSystem := &DBFS{user: &ent.User{}}
	foreignFile := &File{Model: &ent.File{OwnerID: 2}}

	capability := navigator.capabilitiesFor(foreignFile)
	if !capability.Enabled(int(NavigatorCapabilityVersionControl)) {
		t.Fatal("single-file update grant must expose content replacement capability")
	}
	for _, capabilityID := range []NavigatorCapability{
		NavigatorCapabilityCreateFile,
		NavigatorCapabilityUploadFile,
		NavigatorCapabilityRenameFile,
		NavigatorCapabilityDeleteFile,
		NavigatorCapabilitySoftDelete,
	} {
		if capability.Enabled(int(capabilityID)) {
			t.Fatalf("single-file update must not expose capability %v", capabilityID)
		}
	}
	if !fileSystem.canAccessWithNavigator(navigator, foreignFile, NavigatorCapabilityVersionControl) {
		t.Fatal("single-file update grant must pass the content-update owner check")
	}
}

func TestShareNavigatorDoesNotRestoreAuthorizationState(t *testing.T) {
	navigator := &shareNavigator{}
	stale := shareNavigatorState{
		Share: &ent.Share{},
	}

	if err := navigator.RestoreState(stale); err == nil {
		t.Fatal("share authorization state must not be restored from a context hint")
	}
}
