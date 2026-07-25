package inventory

import (
	"os"
	"strings"
	"testing"
)

func TestWopiPutContentRequiresUpdateCapability(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../service/explorer/viewer.go")
	if err != nil {
		t.Fatalf("read WOPI service: %v", err)
	}

	source := string(contents)
	start := strings.Index(source, "func (service *WopiService) PutContent")
	end := strings.Index(source, "func (service *WopiService) GetFile")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("could not isolate WopiService.PutContent")
	}

	putContent := source[start:end]
	if strings.Contains(putContent, "NavigatorCapabilityUploadFile") {
		t.Error("WOPI content updates must not require the create/upload capability")
	}
	if !strings.Contains(putContent, "NavigatorCapabilityVersionControl") {
		t.Error("WOPI content updates must require the existing-file update capability")
	}
}

func TestWopiEditabilityUsesDBFSUpdateCapability(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../service/explorer/viewer.go")
	if err != nil {
		t.Fatalf("read WOPI service: %v", err)
	}

	source := string(contents)
	start := strings.Index(source, "func (service *WopiService) FileInfo")
	end := strings.Index(source, "type (")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("could not isolate WopiService.FileInfo")
	}

	fileInfo := source[start:end]
	if !strings.Contains(fileInfo, "NavigatorCapabilityVersionControl") {
		t.Error("WOPI editability must be derived from DBFS update capability")
	}
	if strings.Contains(fileInfo, "file.OwnerID() == user.ID && uri.FileSystem() == constants.FileSystemMy") {
		t.Error("WOPI editability must not be restricted to owner files")
	}
}

func TestWopiMutationRoutesRequireEditSession(t *testing.T) {
	t.Parallel()

	router, err := os.ReadFile("../routers/router.go")
	if err != nil {
		t.Fatalf("read router: %v", err)
	}
	if !strings.Contains(string(router), "wopi.POST(\":id/contents\", middleware.WopiWriteAccess()") {
		t.Error("WOPI content route must require an edit-mode viewer session")
	}
	if !strings.Contains(string(router), "wopi.POST(\":id\", middleware.WopiWriteAccess()") {
		t.Error("WOPI lock, unlock, refresh, and PutRelative route must require an edit-mode viewer session")
	}

	middlewareSource, err := os.ReadFile("../middleware/wopi.go")
	if err != nil {
		t.Fatalf("read WOPI middleware: %v", err)
	}
	if !strings.Contains(string(middlewareSource), "viewerSession.Action != types.ViewerActionEdit") {
		t.Error("WOPI write middleware must enforce the action stored in the viewer session")
	}
	viewer, err := os.ReadFile("../service/explorer/viewer.go")
	if err != nil {
		t.Fatalf("read WOPI service: %v", err)
	}
	if !strings.Contains(string(viewer), "viewerSession.Action != types.ViewerActionEdit") {
		t.Error("WOPI PutRelative must reject write attempts from a view-mode session")
	}
	unlockStart := strings.Index(string(viewer), "func (service *WopiService) Unlock")
	unlockEnd := strings.Index(string(viewer), "func (service *WopiService) RefreshLock")
	if unlockStart < 0 || unlockEnd < 0 || unlockEnd <= unlockStart {
		t.Fatal("could not isolate WopiService.Unlock")
	}
	if !strings.Contains(string(viewer)[unlockStart:unlockEnd], "viewerSession.Action != types.ViewerActionEdit") {
		t.Error("WOPI unlock must enforce the edit-mode viewer session in the service layer")
	}
}

func TestWebDAVOptionsRequiresReadCapabilityBeforeAdvertisingTargetMethods(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../pkg/webdav/webdav.go")
	if err != nil {
		t.Fatalf("read WebDAV handler: %v", err)
	}

	source := string(contents)
	start := strings.Index(source, "func handleOptions")
	end := strings.Index(source, "func handleGetHeadPost")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("could not isolate WebDAV OPTIONS handler")
	}

	options := source[start:end]
	if !strings.Contains(options, "dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityDownloadFile)") {
		t.Error("WebDAV OPTIONS must require read access before advertising target-specific methods")
	}
}
