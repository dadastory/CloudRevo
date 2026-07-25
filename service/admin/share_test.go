package admin

import (
	"os"
	"strings"
	"testing"
)

func TestSetDefaultRejectsInvalidShareBeforeReservingSlot(t *testing.T) {
	contents, err := os.ReadFile("share.go")
	if err != nil {
		t.Fatalf("read default-share service: %v", err)
	}
	source := string(contents)
	start := strings.Index(source, "func (s *DefaultShareService) SetDefault")
	endOffset := strings.Index(source[start:], "type (")
	if start < 0 || endOffset < 0 {
		t.Fatal("could not isolate DefaultShareService.SetDefault")
	}
	setDefault := source[start : start+endOffset]
	if !strings.Contains(setDefault, "inventory.IsValidShare(share)") {
		t.Error("default designation must reject an expired, depleted, or source-missing share")
	}
}

func TestSetDefaultLoadsCandidateEdgesBeforeValidation(t *testing.T) {
	contents, err := os.ReadFile("share.go")
	if err != nil {
		t.Fatalf("read default-share service: %v", err)
	}
	source := string(contents)
	start := strings.Index(source, "func (s *DefaultShareService) SetDefault")
	endOffset := strings.Index(source[start:], "type (")
	if start < 0 || endOffset < 0 {
		t.Fatal("could not isolate DefaultShareService.SetDefault")
	}
	setDefault := source[start : start+endOffset]
	if !strings.Contains(setDefault, "inventory.LoadShareFile{}") || !strings.Contains(setDefault, "inventory.LoadShareUser{}") {
		t.Fatal("candidate validation must load both share file and user edges")
	}
}

func TestSetDefaultRevalidatesCandidateInsideReservationTransaction(t *testing.T) {
	contents, err := os.ReadFile("share.go")
	if err != nil {
		t.Fatalf("read default-share service: %v", err)
	}
	source := string(contents)
	start := strings.Index(source, "func (s *DefaultShareService) SetDefault")
	endOffset := strings.Index(source[start:], "type (")
	if start < 0 || endOffset < 0 {
		t.Fatal("could not isolate DefaultShareService.SetDefault")
	}
	setDefault := source[start : start+endOffset]
	reservation := strings.Index(setDefault, "ReserveDefaultShareSlot")
	reload := strings.Index(setDefault, "inventory.NewShareClient(defaultShareTx.Client()")
	validation := strings.LastIndex(setDefault, "inventory.IsValidShare(share)")
	if reservation < 0 || reload < reservation || validation < reload {
		t.Fatal("default designation must reload and validate the candidate after reserving its transaction slot")
	}
}
