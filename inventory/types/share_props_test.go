package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSharePropsDoesNotPersistLegacyAllowWrite(t *testing.T) {
	field, ok := reflect.TypeOf(ShareProps{}).FieldByName("AllowWrite")
	if ok {
		t.Fatalf("ShareProps must not expose legacy AllowWrite, got %#v", field)
	}

	var props ShareProps
	encoded, err := json.Marshal(props)
	if err != nil {
		t.Fatalf("marshal share properties: %v", err)
	}

	var decoded map[string]bool
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal share properties: %v", err)
	}
	if _, ok := decoded["allow_write"]; ok {
		t.Fatalf("legacy allow_write must not be persisted, got %#v", decoded)
	}
}

func TestSharePropsDoesNotPersistLinkAccessRules(t *testing.T) {
	if field, ok := reflect.TypeOf(ShareProps{}).FieldByName("AccessRule"); ok {
		t.Fatalf("share links must not carry a second ACL, got %#v", field)
	}
	encoded, err := json.Marshal(ShareProps{})
	if err != nil {
		t.Fatalf("marshal share properties: %v", err)
	}
	if string(encoded) != "{}" {
		t.Fatalf("unexpected empty share props encoding: %s", encoded)
	}
}

func TestSharePropsDefaultPersistsOnlyWhenEnabled(t *testing.T) {
	field, ok := reflect.TypeOf(ShareProps{}).FieldByName("Default")
	if !ok {
		t.Fatal("ShareProps must expose Default")
	}
	if field.Tag.Get("json") != "default,omitempty" {
		t.Fatalf("Default must persist as optional default marker, got %q", field.Tag.Get("json"))
	}

	encoded, err := json.Marshal(ShareProps{Default: true})
	if err != nil {
		t.Fatalf("marshal default share properties: %v", err)
	}
	if string(encoded) == "{}" {
		t.Fatal("enabled default marker must be persisted")
	}
}
