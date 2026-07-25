package explorer

import (
	"context"
	"testing"
	"time"
)

func TestRemoteDownloadPreviewContextBoundsOnlyMagnetSources(t *testing.T) {
	parent := context.Background()
	magnetCtx, magnetCancel := remoteDownloadPreviewContext(parent, "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567")
	defer magnetCancel()
	deadline, ok := magnetCtx.Deadline()
	if !ok {
		t.Fatal("magnet preview must have a deadline")
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > magnetPreviewTimeout {
		t.Fatalf("magnet preview deadline remaining = %s, want (0, %s]", remaining, magnetPreviewTimeout)
	}

	httpCtx, httpCancel := remoteDownloadPreviewContext(parent, "https://downloads.example.test/release.iso")
	defer httpCancel()
	if _, ok := httpCtx.Deadline(); ok {
		t.Fatal("HTTP preview must retain its caller context without a magnet deadline")
	}
}
