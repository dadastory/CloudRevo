package types

import (
	"testing"
)

func TestShareAccessRulePrefersDirectUserRule(t *testing.T) {
	rule := ShareAccessRule{
		Authenticated: SharePermission{Read: true, Create: true},
		Groups: map[int]SharePermission{
			2: {Read: true, Update: true, Delete: true},
		},
		Users: map[int]SharePermission{
			7: {},
		},
	}

	got := rule.Resolve(7, 2, false)
	if got.Read || got.Create || got.Update || got.Delete {
		t.Fatalf("direct empty grant must deny group and authenticated grants, got %#v", got)
	}
}

func TestShareAccessRuleUsesAnonymousGrantOnlyForAnonymousVisitor(t *testing.T) {
	rule := ShareAccessRule{
		Anonymous: SharePermission{Read: true, Create: true},
	}

	anonymous := rule.Resolve(0, 0, true)
	if !anonymous.Read || !anonymous.Create || anonymous.Update || anonymous.Delete {
		t.Fatalf("unexpected anonymous permissions: %#v", anonymous)
	}

	authenticated := rule.Resolve(9, 0, false)
	if authenticated.Read || authenticated.Create || authenticated.Update || authenticated.Delete {
		t.Fatalf("anonymous grant must not apply to authenticated visitor, got %#v", authenticated)
	}
}

func TestShareAccessRuleRejectsInvalidAudienceIDs(t *testing.T) {
	rule := ShareAccessRule{Users: map[int]SharePermission{0: {Read: true}}}
	if err := rule.Validate(); err == nil {
		t.Fatal("zero user id must be rejected")
	}

	rule = ShareAccessRule{Groups: map[int]SharePermission{-1: {Read: true}}}
	if err := rule.Validate(); err == nil {
		t.Fatal("negative group id must be rejected")
	}
}

func TestShareAccessRuleRejectsOversizedExactAudiences(t *testing.T) {
	rule := ShareAccessRule{Users: make(map[int]SharePermission, 257)}
	for id := 1; id <= 257; id++ {
		rule.Users[id] = SharePermission{Read: true}
	}

	if err := rule.Validate(); err == nil {
		t.Fatal("oversized exact audience list must be rejected")
	}
}
