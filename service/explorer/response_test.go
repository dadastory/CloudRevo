package explorer

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWriteEventSourceComment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	WriteEventSourceComment(context, "keepalive")

	if got, want := recorder.Body.String(), ": keepalive\n\n"; got != want {
		t.Fatalf("heartbeat frame = %q, want %q", got, want)
	}
}
