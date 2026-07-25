package user

import (
	"os"
	"strings"
	"testing"

	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/inventory/types"
)

func TestReconciliationUsesPersistedDefaultMarker(t *testing.T) {
	contents, err := os.ReadFile("default_shares.go")
	if err != nil {
		t.Fatalf("read default shortcut reconciliation: %v", err)
	}
	source := string(contents)
	start := strings.Index(source, "func ReconcileDefaultShareShortcuts")
	end := strings.Index(source, "func defaultShareReconcilePage")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("could not isolate default shortcut reconciliation")
	}
	if !strings.Contains(source[start:end], "!share.IsDefault") {
		t.Error("reconciliation must use the persisted is_default marker")
	}
}

func TestCanReceiveDefaultShareRequiresEffectiveRead(t *testing.T) {
	target := &ent.User{ID: 7, Edges: ent.UserEdges{Group: &ent.Group{ID: 2}}}
	share := &ent.Share{Props: &types.ShareProps{Default: true}, Edges: ent.ShareEdges{File: &ent.File{Props: &types.FileProps{ShareAccessRule: &types.ShareAccessRule{
		Groups: map[int]types.SharePermission{2: {}},
	}}}}}

	if canReceiveDefaultShare(share, target) {
		t.Fatal("a default shortcut must not be created when a source rule denies read")
	}

	share.Edges.File.Props.ShareAccessRule.Groups[2] = types.SharePermission{Read: true}
	if !canReceiveDefaultShare(share, target) {
		t.Fatal("a recipient with effective read access must receive the shortcut")
	}
}
