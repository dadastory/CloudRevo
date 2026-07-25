package explorer

import (
	"testing"

	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/boolset"
)

func TestCanManageFileAccessRule(t *testing.T) {
	owner := &ent.User{ID: 1}
	if !canManageFileAccessRule(&ent.User{ID: owner.ID}, owner) {
		t.Fatal("owner must manage the file rule")
	}

	permissions := &boolset.BooleanSet{}
	boolset.Set(types.GroupPermissionIsAdmin, true, permissions)
	admin := &ent.User{ID: 2, Edges: ent.UserEdges{Group: &ent.Group{Permissions: permissions}}}
	if !canManageFileAccessRule(admin, owner) {
		t.Fatal("administrator must receive the existing rule before editing")
	}
	if canManageFileAccessRule(&ent.User{ID: 3}, owner) {
		t.Fatal("unrelated user must not receive the rule")
	}
}
