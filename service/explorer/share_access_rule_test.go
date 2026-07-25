package explorer

import (
	"strings"
	"testing"

	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/hashid"
)

func TestShareAccessRuleUsesHashIDsAtAPIBoundary(t *testing.T) {
	encoder, err := hashid.New("share-access-rule-test")
	if err != nil {
		t.Fatal(err)
	}

	public := &ShareAccessRule{
		Anonymous:     types.SharePermission{Read: true},
		Authenticated: types.SharePermission{Read: true, Create: true},
		Users: map[string]types.SharePermission{
			hashid.EncodeUserID(encoder, 12): {Update: true},
		},
		Groups: map[string]types.SharePermission{
			hashid.EncodeGroupID(encoder, 8): {Delete: true},
		},
	}

	internal, err := public.ToInternal(encoder)
	if err != nil {
		t.Fatal(err)
	}
	if !internal.Users[12].Update || !internal.Groups[8].Delete {
		t.Fatalf("unexpected internal rule: %#v", internal)
	}

	roundTrip := BuildShareAccessRule(internal, encoder)
	if !roundTrip.Users[hashid.EncodeUserID(encoder, 12)].Update ||
		!roundTrip.Groups[hashid.EncodeGroupID(encoder, 8)].Delete {
		t.Fatalf("unexpected public rule: %#v", roundTrip)
	}
}

func TestShareAccessRuleRejectsOversizedPublicAudiencesBeforeDecodingIDs(t *testing.T) {
	encoder, err := hashid.New("share-access-rule-test")
	if err != nil {
		t.Fatal(err)
	}

	rule := &ShareAccessRule{Users: make(map[string]types.SharePermission, types.MaxShareAccessRuleAudiences+1)}
	for i := 0; i <= types.MaxShareAccessRuleAudiences; i++ {
		rule.Users["not-a-valid-hash-"+string(rune('a'+i%26))+string(rune('A'+i/26))] = types.SharePermission{Read: true}
	}

	_, err = rule.ToInternal(encoder)
	if err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("oversized audiences must be rejected before hash decoding, got %v", err)
	}
}

func TestShareAccessRuleRejectsAudienceWithWrongHashType(t *testing.T) {
	encoder, err := hashid.New("share-access-rule-test")
	if err != nil {
		t.Fatal(err)
	}

	rule := &ShareAccessRule{Users: map[string]types.SharePermission{
		hashid.EncodeGroupID(encoder, 8): {Read: true},
	}}
	if _, err := rule.ToInternal(encoder); err == nil {
		t.Fatal("a group hash must not be accepted as a user audience")
	}
}
