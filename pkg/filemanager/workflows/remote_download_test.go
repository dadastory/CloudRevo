package workflows

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/pkg/downloader"
	"github.com/dadastory/CloudRevo/pkg/request"
)

type controllableTestDownloader struct {
	paused    bool
	continued bool
	status    *downloader.TaskStatus
}

func (d *controllableTestDownloader) CreateTask(context.Context, string, map[string]interface{}) (*downloader.TaskHandle, error) {
	return nil, nil
}
func (d *controllableTestDownloader) Info(context.Context, *downloader.TaskHandle) (*downloader.TaskStatus, error) {
	return d.status, nil
}
func (d *controllableTestDownloader) Cancel(context.Context, *downloader.TaskHandle) error {
	return nil
}
func (d *controllableTestDownloader) SetFilesToDownload(context.Context, *downloader.TaskHandle, ...*downloader.SetFileToDownloadArgs) error {
	return nil
}
func (d *controllableTestDownloader) Test(context.Context) (string, error) { return "", nil }
func (d *controllableTestDownloader) Pause(context.Context, *downloader.TaskHandle) error {
	d.paused = true
	return nil
}
func (d *controllableTestDownloader) Continue(context.Context, *downloader.TaskHandle) error {
	d.continued = true
	return nil
}

func TestRemoteDownloadTransferReadinessRequiresMaterializedSelectedFiles(t *testing.T) {
	dir := t.TempDir()
	status := &downloader.TaskStatus{
		State:      downloader.StatusSeeding,
		Total:      4,
		Downloaded: 0,
		SavePath:   dir,
		Files: []downloader.TaskFile{
			{Index: 0, Name: "release.iso", Size: 4, Selected: true},
		},
	}
	if remoteDownloadReadyForTransfer(status) {
		t.Fatal("zero-byte seeding task must not be transferable")
	}

	status.Downloaded = status.Total
	if remoteDownloadReadyForTransfer(status) {
		t.Fatal("missing selected file must not be transferable")
	}
	if err := os.WriteFile(filepath.Join(dir, "release.iso"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write selected file: %v", err)
	}
	if !remoteDownloadReadyForTransfer(status) {
		t.Fatal("materialized fully downloaded seeding task must be transferable")
	}
}

func TestRemoteDownloadPausedStatusIsNotTransferReady(t *testing.T) {
	status := &downloader.TaskStatus{State: downloader.StatusPaused, SavePath: t.TempDir()}
	if remoteDownloadReadyForTransfer(status) {
		t.Fatal("paused remote download must remain in the monitor phase")
	}
}

func TestControlDownloadPreservesExistingHandleAndPersistsLiveStatus(t *testing.T) {
	ctx := context.WithValue(context.Background(), inventory.UserCtx{}, &ent.User{ID: 1})
	queued, err := NewRemoteDownloadTask(ctx, "https://downloads.example.test/file", "", "/My", nil)
	if err != nil {
		t.Fatalf("NewRemoteDownloadTask() error = %v", err)
	}
	remote := queued.(*RemoteDownloadTask)
	state := &RemoteDownloadTaskState{}
	if err := json.Unmarshal([]byte(remote.State()), state); err != nil {
		t.Fatalf("unmarshal task state: %v", err)
	}
	state.Handle = &downloader.TaskHandle{ID: "gopeed-task", Hash: "workspace"}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal task state: %v", err)
	}
	remote.Task.PrivateState = string(encoded)
	controlled := &controllableTestDownloader{status: &downloader.TaskStatus{State: downloader.StatusPaused}}
	remote.d = controlled

	if err := remote.ControlDownload(context.Background(), false); err != nil {
		t.Fatalf("pause remote task: %v", err)
	}
	if !controlled.paused || controlled.continued {
		t.Fatalf("unexpected pause calls: %#v", controlled)
	}
	persisted := &RemoteDownloadTaskState{}
	if err := json.Unmarshal([]byte(remote.State()), persisted); err != nil {
		t.Fatalf("unmarshal persisted state: %v", err)
	}
	if persisted.Handle == nil || persisted.Handle.ID != "gopeed-task" || persisted.Status == nil || persisted.Status.State != downloader.StatusPaused {
		t.Fatalf("pause did not preserve task state: %#v", persisted)
	}

	controlled.status = &downloader.TaskStatus{State: downloader.StatusDownloading}
	if err := remote.ControlDownload(context.Background(), true); err != nil {
		t.Fatalf("continue remote task: %v", err)
	}
	if !controlled.continued {
		t.Fatal("continue was not delegated to the existing downloader")
	}
}

func TestRemoteDownloadTransferReadinessRejectsSelectedFileOutsideSavePath(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), "outside-release.iso")
	if err := os.WriteFile(outside, []byte("data"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })

	status := &downloader.TaskStatus{
		State:      downloader.StatusCompleted,
		Total:      4,
		Downloaded: 4,
		SavePath:   dir,
		Files: []downloader.TaskFile{
			{Index: 0, Name: "../outside-release.iso", Size: 4, Selected: true},
		},
	}
	if remoteDownloadReadyForTransfer(status) {
		t.Fatal("selected file outside the download directory must not be transferable")
	}
}

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

func TestTorrentTaskOptionsAreInternalAndPreserveConnections(t *testing.T) {
	options := torrentTaskOptions(&downloader.TaskOptions{Connections: 32})
	if !options.AutoTorrent || options.Connections != 32 {
		t.Fatalf("torrent task options = %#v, want internal auto torrent with connections", options)
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("marshal task options: %v", err)
	}
	if strings.Contains(string(encoded), "AutoTorrent") || strings.Contains(string(encoded), "auto_torrent") {
		t.Fatalf("internal auto-torrent flag leaked into task state: %s", encoded)
	}
}

func TestWithNetworkPolicyKeepsNodePolicyOutOfSerializedTaskOptions(t *testing.T) {
	options := WithNetworkPolicy(&downloader.TaskOptions{Connections: 8}, request.SSRFOptions{
		AllowedHosts: []string{"files.internal.example"},
		AllowedCIDRs: []string{"192.168.10.0/24"},
	})
	if options == nil || options.NetworkPolicy == nil || options.NetworkPolicy.AllowedHosts[0] != "files.internal.example" {
		t.Fatalf("unexpected transient policy: %#v", options)
	}
	data, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	if strings.Contains(string(data), "NetworkPolicy") || strings.Contains(string(data), "allowedHosts") || strings.Contains(string(data), "192.168.10.0") {
		t.Fatalf("private policy leaked into task JSON: %s", data)
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

func TestApplyDownloadSelectionRejectsInvalidOrEmptyResult(t *testing.T) {
	files := []downloader.TaskFile{
		{Index: 0, Selected: true},
		{Index: 1, Selected: false},
	}
	for _, args := range [][]*downloader.SetFileToDownloadArgs{
		{{Index: -1, Download: true}},
		{{Index: 2, Download: true}},
		{{Index: 1, Download: true}, {Index: 1, Download: false}},
		{{Index: 0, Download: false}},
	} {
		if _, err := applyDownloadSelection(files, args...); err == nil {
			t.Fatalf("applyDownloadSelection(%#v) error = nil", args)
		}
	}

	selected, err := applyDownloadSelection(files, &downloader.SetFileToDownloadArgs{Index: 1, Download: true})
	if err != nil {
		t.Fatalf("applyDownloadSelection() error = %v", err)
	}
	if len(selected) != 2 || selected[0] != 0 || selected[1] != 1 {
		t.Fatalf("selected = %#v, want [0 1]", selected)
	}
}
