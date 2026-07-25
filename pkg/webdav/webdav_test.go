package webdav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/boolset"
	"github.com/dadastory/CloudRevo/pkg/filemanager/fs"
	"github.com/dadastory/CloudRevo/pkg/filemanager/fs/dbfs"
	"github.com/dadastory/CloudRevo/pkg/filemanager/manager"
	"github.com/gin-gonic/gin"
)

func TestOptionsAdvertisesCollaboratorUpdateCapabilities(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	capabilities := &boolset.BooleanSet{}
	boolset.Sets(map[dbfs.NavigatorCapability]bool{
		dbfs.NavigatorCapabilityDownloadFile:   true,
		dbfs.NavigatorCapabilityVersionControl: true,
		dbfs.NavigatorCapabilityUpdateMetadata: true,
	}, capabilities)
	target := &dbfs.File{Model: &ent.File{OwnerID: 1, Type: int(types.FileTypeFile)}, CapabilitiesBs: capabilities}
	fm := &optionsTestFileManager{target: target}
	user := &ent.User{ID: 2, Edges: ent.UserEdges{DavAccounts: []*ent.DavAccount{{URI: fs.NewMyUri("")}}}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodOptions, "/dav/document.txt", nil)

	if _, err := handleOptions(c, user, fm); err != nil {
		t.Fatalf("handle OPTIONS: %v", err)
	}
	allow := c.Writer.Header().Get("Allow")
	if !strings.Contains(allow, "PUT") || !strings.Contains(allow, "PROPPATCH") {
		t.Fatalf("non-owner update grant must advertise PUT and PROPPATCH, got %q", allow)
	}
	if strings.Contains(allow, "DELETE") || strings.Contains(allow, "MOVE") || strings.Contains(allow, "MKCOL") {
		t.Fatalf("update-only collaborator must not receive delete/create methods, got %q", allow)
	}
	if len(fm.options) != 1 {
		t.Fatalf("OPTIONS must resolve target exactly once, got %d", len(fm.options))
	}
}

type optionsTestFileManager struct {
	manager.FileManager
	target  fs.File
	options []fs.Option
}

func (m *optionsTestFileManager) SharedAddressTranslation(_ context.Context, uri *fs.URI, opts ...fs.Option) (fs.File, *fs.URI, error) {
	m.options = append(m.options, opts...)
	return m.target, uri, nil
}
