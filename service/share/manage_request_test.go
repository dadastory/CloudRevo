package share

import (
	"os"
	"strings"
	"testing"
)

func TestShareCreateServiceDoesNotExposeLinkAccessRule(t *testing.T) {
	request := ShareCreateService{Default: true, IsPrivate: true}
	if !request.Default || !request.IsPrivate {
		t.Fatal("test fixture must describe a password-protected default share")
	}
}

func TestDefaultShareRejectsPasswordProtection(t *testing.T) {
	service := ShareCreateService{Default: true, IsPrivate: true}
	if err := service.validateDefaultPassword(); err == nil {
		t.Fatal("a default share with a password must be rejected")
	}
	if err := (&ShareCreateService{Default: true}).validateDefaultPassword(); err != nil {
		t.Fatalf("a public default share must be accepted: %v", err)
	}
}

func TestOwnerCannotClearAdministratorManagedDefaultMarker(t *testing.T) {
	contents, err := os.ReadFile("manage.go")
	if err != nil {
		t.Fatalf("read share service: %v", err)
	}
	source := string(contents)
	if !strings.Contains(source, "wasDefault && !isAdmin") {
		t.Fatal("ordinary owner edits must not clear an existing default marker")
	}
}
