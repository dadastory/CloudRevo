package workflows

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/pkg/downloader"
)

func TestValidateRequestOptionsNormalizesAuthorizedHeaders(t *testing.T) {
	options, err := ValidateRequestOptions("https://downloads.example.test/file", &downloader.RequestOptions{
		Method: "post",
		Headers: map[string]string{
			"referer": "https://portal.example.test/",
			"cookie":  "session=authorized",
		},
		Body: "token=approved",
	})
	if err != nil {
		t.Fatalf("ValidateRequestOptions() error = %v", err)
	}
	if options.Method != http.MethodPost || options.Headers["Referer"] != "https://portal.example.test/" || options.Headers["Cookie"] != "session=authorized" || options.Body != "token=approved" {
		t.Fatalf("unexpected validated request options: %#v", options)
	}
}

func TestRemoteDownloadSummaryDoesNotExposeRequestOptions(t *testing.T) {
	ctx := context.WithValue(context.Background(), inventory.UserCtx{}, &ent.User{ID: 1})
	task, err := NewRemoteDownloadTask(ctx, "https://downloads.example.test/file", "", "/My", &downloader.RequestOptions{
		Headers: map[string]string{"Cookie": "session=private"},
	})
	if err != nil {
		t.Fatalf("NewRemoteDownloadTask() error = %v", err)
	}
	summary := task.(*RemoteDownloadTask).Summarize(nil)
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if strings.Contains(string(encoded), "session=private") {
		t.Fatalf("summary unexpectedly exposed private request context: %s", encoded)
	}
}

func TestRemoteDownloadTaskPreservesPreflightFileSelectionPrivately(t *testing.T) {
	ctx := context.WithValue(context.Background(), inventory.UserCtx{}, &ent.User{ID: 1})
	task, err := NewRemoteDownloadTaskWithFiles(ctx, "https://downloads.example.test/archive", "", "/My", nil, []int{1, 3})
	if err != nil {
		t.Fatalf("NewRemoteDownloadTaskWithFiles() error = %v", err)
	}
	state := &RemoteDownloadTaskState{}
	if err := json.Unmarshal([]byte(task.(*RemoteDownloadTask).State()), state); err != nil {
		t.Fatalf("unmarshal private state: %v", err)
	}
	if len(state.SelectedFiles) != 2 || state.SelectedFiles[0] != 1 || state.SelectedFiles[1] != 3 {
		t.Fatalf("selected files = %#v, want [1 3]", state.SelectedFiles)
	}
	encoded, err := json.Marshal(task.(*RemoteDownloadTask).Summarize(nil))
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if strings.Contains(string(encoded), "selected_files") {
		t.Fatalf("summary unexpectedly exposed selected files: %s", encoded)
	}
}

func TestValidateRequestOptionsRejectsUnsafeHeader(t *testing.T) {
	for name, value := range map[string]string{
		"Host":       "internal.example.test",
		"X-Injected": "safe\r\nX-Other: injected",
	} {
		if _, err := ValidateRequestOptions("https://downloads.example.test/file", &downloader.RequestOptions{Headers: map[string]string{name: value}}); err == nil {
			t.Fatalf("ValidateRequestOptions() accepted unsafe header %q", name)
		}
	}
}

func TestValidateRequestOptionsRejectsNonHTTPSource(t *testing.T) {
	if _, err := ValidateRequestOptions("magnet:?xt=urn:btih:test", &downloader.RequestOptions{Headers: map[string]string{"Referer": "https://portal.example.test/"}}); err == nil {
		t.Fatal("ValidateRequestOptions() accepted options for magnet source")
	}
}

func TestValidateTaskOptionsRestrictsConnectionsToHTTPSources(t *testing.T) {
	valid, err := ValidateTaskOptions("https://downloads.example.test/release.iso", &downloader.TaskOptions{Connections: 8})
	if err != nil {
		t.Fatalf("ValidateTaskOptions() error = %v", err)
	}
	if valid == nil || valid.Connections != 8 {
		t.Fatalf("ValidateTaskOptions() = %#v, want connections 8", valid)
	}
	for _, testCase := range []struct {
		source      string
		connections int
	}{
		{source: "https://downloads.example.test/release.iso", connections: 0},
		{source: "https://downloads.example.test/release.iso", connections: 257},
		{source: "magnet:?xt=urn:btih:test", connections: 8},
	} {
		if _, err := ValidateTaskOptions(testCase.source, &downloader.TaskOptions{Connections: testCase.connections}); err == nil {
			t.Fatalf("ValidateTaskOptions(%q, %d) error = nil", testCase.source, testCase.connections)
		}
	}
}

func TestRemoteDownloadSummaryUsesPreflightNameBeforeStatus(t *testing.T) {
	ctx := context.WithValue(context.Background(), inventory.UserCtx{}, &ent.User{})
	task, err := NewRemoteDownloadTaskWithConfig(ctx, "https://downloads.example.test/release.iso", "", "/My", nil, &downloader.TaskOptions{Connections: 8}, nil, "release.iso")
	if err != nil {
		t.Fatalf("NewRemoteDownloadTaskWithConfig() error = %v", err)
	}
	summary := task.(*RemoteDownloadTask).Summarize(nil)
	if summary.Props[SummaryKeyDownloadStatus].(*downloader.TaskStatus).Name != "release.iso" {
		t.Fatalf("queued summary did not retain preflight name: %#v", summary.Props[SummaryKeyDownloadStatus])
	}
}
