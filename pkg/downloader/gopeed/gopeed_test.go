package gopeed

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/downloader"
)

func TestNewRejectsIncompletePrivateServiceConfiguration(t *testing.T) {
	t.Parallel()

	_, err := New(&types.GopeedSetting{Server: "http://gopeed:9999"})
	if err == nil {
		t.Fatal("New() accepted missing token and shared roots")
	}
}

func TestNewPrefersExplicitComposeToken(t *testing.T) {
	t.Setenv("CR_GOPEED_API_TOKEN", "compose-token")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Api-Token"); got != "compose-token" {
			t.Fatalf("Gopeed token = %q, want Compose token", got)
		}
		writeResult(t, w, http.StatusOK, map[string]any{"version": "test"})
	}))
	defer server.Close()

	client, err := New(&types.GopeedSetting{
		Server:       server.URL,
		Token:        "persisted-token",
		DownloadPath: "/app/Downloads",
		TempPath:     "/cloudrevo/data/temp/gopeed",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.Test(context.Background()); err != nil {
		t.Fatalf("Test() error = %v", err)
	}
}

func TestNewUsesNodeTokenWithoutComposeOverride(t *testing.T) {
	t.Setenv("CR_GOPEED_API_TOKEN", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Api-Token"); got != "persisted-token" {
			t.Fatalf("Gopeed token = %q, want persisted token", got)
		}
		writeResult(t, w, http.StatusOK, map[string]any{"version": "test"})
	}))
	defer server.Close()

	client, err := New(&types.GopeedSetting{
		Server:       server.URL,
		Token:        "persisted-token",
		DownloadPath: "/app/Downloads",
		TempPath:     "/cloudrevo/data/temp/gopeed",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.Test(context.Background()); err != nil {
		t.Fatalf("Test() error = %v", err)
	}
}

func TestCreateTaskForwardsAuthorizedHTTPRequestOptions(t *testing.T) {

	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/resolve" {
			var payload struct {
				Req struct {
					URL   string `json:"url"`
					Extra struct {
						Method string            `json:"method"`
						Header map[string]string `json:"header"`
						Body   string            `json:"body"`
					} `json:"extra"`
				} `json:"req"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode resolve payload: %v", err)
			}
			if payload.Req.URL != "https://downloads.example.test/file" {
				t.Fatalf("source URL = %q", payload.Req.URL)
			}
			if payload.Req.Extra.Method != http.MethodPost || payload.Req.Extra.Header["Referer"] != "https://portal.example.test/" || payload.Req.Extra.Header["Cookie"] != "session=authorized" || payload.Req.Extra.Body != "token=approved" {
				t.Fatalf("unexpected request context: %#v", payload.Req.Extra)
			}
			writeResult(t, w, http.StatusOK, map[string]any{"id": "resolve-1"})
			return
		}
		if r.URL.Path == "/api/v1/tasks" {
			writeResult(t, w, http.StatusOK, "task-1")
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	_, err := client.CreateTaskWithRequestOptions(context.Background(), "https://downloads.example.test/file", nil, &downloader.RequestOptions{
		Method: http.MethodPost,
		Headers: map[string]string{
			"Referer": "https://portal.example.test/",
			"Cookie":  "session=authorized",
		},
		Body: "token=approved",
	})
	if err != nil {
		t.Fatalf("CreateTaskWithRequestOptions() error = %v", err)
	}
}

func TestCreateTaskClassifiesSourceHTTP403AsTerminal(t *testing.T) {

	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeResultWithCode(t, w, http.StatusOK, 1000, "http request fail, code:403", nil)
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	_, err := client.CreateTask(context.Background(), "https://downloads.example.test/forbidden", nil)
	if err == nil {
		t.Fatal("CreateTask() error = nil")
	}
	var sourceErr *downloader.SourceHTTPError
	if !errors.As(err, &sourceErr) {
		t.Fatalf("error %T = %v, want SourceHTTPError", err, err)
	}
	if sourceErr.StatusCode != http.StatusForbidden {
		t.Fatalf("source status = %d, want %d", sourceErr.StatusCode, http.StatusForbidden)
	}
}

func TestPreviewResolvesThenDiscardsWithoutCreatingTask(t *testing.T) {
	t.Parallel()
	var discarded bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/resolve":
			var payload struct {
				Opts struct {
					Extra struct {
						Connections int `json:"connections"`
					} `json:"extra"`
				} `json:"opts"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode preview payload: %v", err)
			}
			if payload.Opts.Extra.Connections != 8 {
				t.Fatalf("preview connections = %d, want 8", payload.Opts.Extra.Connections)
			}
			writeResult(t, w, http.StatusOK, map[string]any{
				"id":  "preview-1",
				"res": map[string]any{"name": "archive", "files": []map[string]any{{"name": "one.txt", "size": 12}}},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/resolve/preview-1":
			discarded = true
			writeResult(t, w, http.StatusOK, nil)
		case r.URL.Path == "/api/v1/tasks":
			t.Fatal("preview must not create a Gopeed task")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	status, err := client.PreviewTask(context.Background(), "https://downloads.example.test/archive", nil, nil, &downloader.TaskOptions{Connections: 8})
	if err != nil {
		t.Fatalf("PreviewTask() error = %v", err)
	}
	if !discarded || status.Name != "archive" || len(status.Files) != 1 || status.Files[0].Name != "one.txt" || status.Total != 12 {
		t.Fatalf("unexpected preview: discarded=%v status=%#v", discarded, status)
	}
}

func TestPreviewCleanupContextSurvivesCallerCancellation(t *testing.T) {
	caller, cancel := context.WithCancel(context.Background())
	cancel()
	cleanup, done := previewCleanupContext(caller)
	defer done()
	if err := cleanup.Err(); err != nil {
		t.Fatalf("cleanup context unexpectedly canceled: %v", err)
	}
	if _, ok := cleanup.Deadline(); !ok {
		t.Fatal("cleanup context must have a bounded deadline")
	}
}

func TestPreviewUsesFirstFileNameWhenResourceNameIsEmpty(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			writeResult(t, w, http.StatusOK, map[string]any{
				"id":  "preview-file",
				"res": map[string]any{"files": []map[string]any{{"name": "release.iso", "size": 42}}},
			})
		case http.MethodDelete:
			writeResult(t, w, http.StatusOK, nil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	preview, err := client.PreviewTask(context.Background(), "https://downloads.example.test/release.iso", nil, nil, nil)
	if err != nil {
		t.Fatalf("PreviewTask() error = %v", err)
	}
	if preview.Name != "release.iso" {
		t.Fatalf("preview name = %q, want first resolved file name", preview.Name)
	}
}

func TestInfoUsesFirstFileNameWhenTaskAndResourceNamesAreEmpty(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tasks/task-1" {
			http.NotFound(w, r)
			return
		}
		writeResult(t, w, http.StatusOK, map[string]any{
			"id":     "task-1",
			"status": "running",
			"size":   42,
			"meta": map[string]any{
				"opts": map[string]any{},
				"res":  map[string]any{"files": []map[string]any{{"name": "release.iso", "size": 42}}},
			},
		})
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	status, err := client.Info(context.Background(), &downloader.TaskHandle{ID: "task-1", Hash: "a68b7e87-61df-41de-82fb-42956318a711"})
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if status.Name != "release.iso" {
		t.Fatalf("Info() name = %q, want release.iso", status.Name)
	}
}

func TestCreateTaskForwardsValidatedTaskConnections(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/resolve":
			var payload struct {
				Opts struct {
					Extra struct {
						Connections int `json:"connections"`
					} `json:"extra"`
				} `json:"opts"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode resolve payload: %v", err)
			}
			if payload.Opts.Extra.Connections != 8 {
				t.Fatalf("connections = %d, want 8", payload.Opts.Extra.Connections)
			}
			writeResult(t, w, http.StatusOK, map[string]any{"id": "resolve-1"})
		case "/api/v1/tasks":
			writeResult(t, w, http.StatusOK, "task-1")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	_, err := client.CreateTaskWithOptions(context.Background(), "https://downloads.example.test/release.iso", nil, nil, &downloader.TaskOptions{Connections: 8}, nil)
	if err != nil {
		t.Fatalf("CreateTaskWithOptions() error = %v", err)
	}
}

func TestCreateTaskIgnoresLegacyGroupOptions(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/resolve":
			var payload struct {
				Opts struct {
					Extra map[string]any `json:"extra"`
				} `json:"opts"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode resolve payload: %v", err)
			}
			if _, forwarded := payload.Opts.Extra["unsafeLegacyOption"]; forwarded {
				t.Fatalf("legacy group option was forwarded: %#v", payload.Opts.Extra)
			}
			writeResult(t, w, http.StatusOK, map[string]any{"id": "resolve-1"})
		case "/api/v1/tasks":
			writeResult(t, w, http.StatusOK, "task-1")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	if _, err := client.CreateTask(context.Background(), "https://downloads.example.test/release.iso", map[string]any{"unsafeLegacyOption": true}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
}

func TestCreateTaskWithOptionsForwardsSelectedFiles(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/resolve" {
			var payload struct {
				Opts struct {
					SelectFiles []int `json:"selectFiles"`
				} `json:"opts"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(payload.Opts.SelectFiles, []int{1, 3}) {
				t.Fatalf("selectFiles = %#v", payload.Opts.SelectFiles)
			}
			writeResult(t, w, http.StatusOK, map[string]any{"id": "resolve-1"})
			return
		}
		if r.URL.Path == "/api/v1/tasks" {
			writeResult(t, w, http.StatusOK, "task-1")
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	if _, err := client.CreateTaskWithOptions(context.Background(), "https://downloads.example.test/archive", nil, nil, nil, []int{1, 3}); err != nil {
		t.Fatalf("CreateTaskWithOptions() error = %v", err)
	}
}

func TestCancelTreatsMissingGopeedTaskAsAlreadyCleanedUp(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s", r.Method)
		}
		writeResultWithCode(t, w, http.StatusOK, 2001, "task not found", nil)
	}))
	defer server.Close()

	tempRoot := t.TempDir()
	client := newClient(server.URL, "test-token", "/app/Downloads", tempRoot, nil)
	if err := client.Cancel(context.Background(), &downloader.TaskHandle{ID: "missing", Hash: "c53314ac-3795-4ef4-a677-c546dfe4bf93"}); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
}

func TestIntegrationHTTPTaskLifecycle(t *testing.T) {
	server := os.Getenv("GOPEED_INTEGRATION_URL")
	token := os.Getenv("GOPEED_INTEGRATION_TOKEN")
	if server == "" || token == "" {
		t.Skip("set GOPEED_INTEGRATION_URL and GOPEED_INTEGRATION_TOKEN to run against a Compose sidecar")
	}

	client := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	handle, err := client.CreateTask(ctx, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("create HTTP task: %v", err)
	}
	defer func() {
		if err := client.Cancel(context.Background(), handle); err != nil {
			t.Errorf("clean up HTTP task: %v", err)
		}
	}()

	for {
		status, err := client.Info(ctx, handle)
		if err != nil {
			t.Fatalf("read HTTP task: %v", err)
		}
		switch status.State {
		case downloader.StatusCompleted:
			if status.Downloaded == 0 || len(status.Files) == 0 {
				t.Fatalf("completed task has no downloaded data: %#v", status)
			}
			return
		case downloader.StatusError:
			t.Fatalf("HTTP task failed: %s", status.ErrorMessage)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("HTTP task did not finish: %v", ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func TestIntegrationHTTPTaskWithAuthorizedRequestHeaders(t *testing.T) {
	server := os.Getenv("GOPEED_INTEGRATION_URL")
	token := os.Getenv("GOPEED_INTEGRATION_TOKEN")
	if server == "" || token == "" {
		t.Skip("set GOPEED_INTEGRATION_URL and GOPEED_INTEGRATION_TOKEN to run against a Compose sidecar")
	}

	client := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	handle, err := client.CreateTaskWithRequestOptions(ctx, "https://example.com/", nil, &downloader.RequestOptions{
		Headers: map[string]string{
			"Referer":    "https://example.com/",
			"User-Agent": "CloudRevo-Gopeed-Integration-Test",
		},
	})
	if err != nil {
		t.Fatalf("create HTTP task with authorized headers: %v", err)
	}
	defer func() {
		if err := client.Cancel(context.Background(), handle); err != nil {
			t.Errorf("clean up HTTP task: %v", err)
		}
	}()

	for {
		status, err := client.Info(ctx, handle)
		if err != nil {
			t.Fatalf("read HTTP task: %v", err)
		}
		if status.State == downloader.StatusCompleted {
			return
		}
		if status.State == downloader.StatusError {
			t.Fatalf("HTTP task with authorized headers failed: %s", status.ErrorMessage)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("HTTP task with authorized headers did not finish: %v", ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func TestIntegrationPreviewDoesNotCreateTask(t *testing.T) {
	server := os.Getenv("GOPEED_INTEGRATION_URL")
	token := os.Getenv("GOPEED_INTEGRATION_TOKEN")
	if server == "" || token == "" {
		t.Skip("set GOPEED_INTEGRATION_URL and GOPEED_INTEGRATION_TOKEN to run against a Compose sidecar")
	}

	client := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	preview, err := client.PreviewTask(ctx, "https://example.com/", nil, nil, nil)
	if err != nil {
		t.Fatalf("preview HTTP task: %v", err)
	}
	if len(preview.Files) == 0 {
		t.Fatalf("preview has no resolved file metadata: %#v", preview)
	}
}

func TestIntegrationComposeSourceRequestContextAndForbiddenClassification(t *testing.T) {
	server := os.Getenv("GOPEED_INTEGRATION_URL")
	token := os.Getenv("GOPEED_INTEGRATION_TOKEN")
	if server == "" || token == "" {
		t.Skip("set GOPEED_INTEGRATION_URL and GOPEED_INTEGRATION_TOKEN to run against a Compose sidecar")
	}

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split test listener address: %v", err)
	}

	source := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authorized":
			if r.Header.Get("Referer") != "https://portal.example.test/" || r.Header.Get("Cookie") != "session=authorized" {
				http.Error(w, "missing authorized request context", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Length", "2")
			_, _ = w.Write([]byte("ok"))
		case "/forbidden":
			http.Error(w, "forbidden", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	source.Listener = listener
	source.Start()
	defer source.Close()

	client := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	sourceHost := os.Getenv("GOPEED_INTEGRATION_SOURCE_HOST")
	if sourceHost == "" {
		sourceHost = "test"
	}
	baseURL := "http://" + sourceHost + ":" + strconv.Itoa(mustAtoi(t, port))
	handle, err := client.CreateTaskWithRequestOptions(context.Background(), baseURL+"/authorized", nil, &downloader.RequestOptions{
		Headers: map[string]string{
			"Referer": "https://portal.example.test/",
			"Cookie":  "session=authorized",
		},
	})
	if err != nil {
		t.Fatalf("create task against Compose test source: %v", err)
	}
	defer func() { _ = client.Cancel(context.Background(), handle) }()

	_, err = client.CreateTask(context.Background(), baseURL+"/forbidden", nil)
	var sourceErr *downloader.SourceHTTPError
	if !errors.As(err, &sourceErr) || sourceErr.StatusCode != http.StatusForbidden {
		t.Fatalf("forbidden source error = %v, want HTTP 403 SourceHTTPError", err)
	}
}

func mustAtoi(t *testing.T, value string) int {
	t.Helper()
	result, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("parse port %q: %v", value, err)
	}
	return result
}

func TestInfoMapsUploadingTaskToSeedingAndUsesLocalTaskPath(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Token") != "test-token" {
			t.Fatalf("unexpected API token")
		}
		if r.URL.Path != "/api/v1/tasks/task-1" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		writeResult(t, w, http.StatusOK, map[string]any{
			"id":           "task-1",
			"name":         "archive",
			"status":       "done",
			"uploading":    true,
			"size":         30,
			"fileProgress": map[string]any{"0": 5, "2": 10},
			"progress": map[string]any{
				"downloaded":  30,
				"speed":       4,
				"uploaded":    8,
				"uploadSpeed": 2,
			},
			"meta": map[string]any{
				"opts": map[string]any{"path": "/app/Downloads/task-1", "selectFiles": []int{0, 2}},
				"res": map[string]any{"hash": "torrent-info-hash", "files": []map[string]any{
					{"name": "one.txt", "path": "one.txt", "size": 10},
					{"name": "two.txt", "path": "two.txt", "size": 10},
					{"name": "three.txt", "path": "three.txt", "size": 10},
				}},
			},
		})
	}))
	defer server.Close()

	const taskPathKey = "c53314ac-3795-4ef4-a677-c546dfe4bf93"
	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	status, err := client.Info(context.Background(), &downloader.TaskHandle{ID: "task-1", Hash: taskPathKey})
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}

	if status.State != downloader.StatusSeeding {
		t.Fatalf("state = %q, want %q", status.State, downloader.StatusSeeding)
	}
	if status.SavePath != "/cloudrevo/data/temp/gopeed/"+taskPathKey {
		t.Fatalf("save path = %q", status.SavePath)
	}
	if status.Total != 30 || status.Downloaded != 30 || status.Uploaded != 8 {
		t.Fatalf("unexpected transfer totals: %#v", status)
	}
	if len(status.Files) != 3 || !status.Files[0].Selected || status.Files[1].Selected || !status.Files[2].Selected {
		t.Fatalf("unexpected selected files: %#v", status.Files)
	}
	if status.Hash != "torrent-info-hash" {
		t.Fatalf("hash = %q, want torrent info hash", status.Hash)
	}
	if !status.Files[0].ProgressKnown || status.Files[0].Progress != 0.5 || status.Files[1].ProgressKnown || !status.Files[2].ProgressKnown || status.Files[2].Progress != 1 {
		t.Fatalf("unexpected per-file progress telemetry: %#v", status.Files)
	}
}

func TestInfoTreatsEmptySelectionAsAllResolvedFiles(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeResult(t, w, http.StatusOK, map[string]any{
			"id":     "task-1",
			"status": "done",
			"size":   30,
			"meta": map[string]any{
				"opts": map[string]any{"selectFiles": []int{}},
				"res": map[string]any{"files": []map[string]any{
					{"name": "one.txt", "path": "one.txt", "size": 10},
					{"name": "two.txt", "path": "two.txt", "size": 20},
				}},
			},
		})
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	status, err := client.Info(context.Background(), &downloader.TaskHandle{ID: "task-1", Hash: "c53314ac-3795-4ef4-a677-c546dfe4bf93"})
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if len(status.Files) != 2 || !status.Files[0].Selected || !status.Files[1].Selected {
		t.Fatalf("default selection = %#v, want all resolved files selected", status.Files)
	}
}

func TestInfoUsesContainedLocalFileSizeWhenGopeedOmitsHTTPSize(t *testing.T) {
	t.Parallel()

	const taskPathKey = "c53314ac-3795-4ef4-a677-c546dfe4bf93"
	tempRoot := t.TempDir()
	taskRoot := filepath.Join(tempRoot, taskPathKey)
	if err := os.MkdirAll(taskRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskRoot, "example.com"), []byte("downloaded response"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeResult(t, w, http.StatusOK, map[string]any{
			"id":       "task-1",
			"status":   "done",
			"progress": map[string]any{"downloaded": 19},
			"meta": map[string]any{
				"opts": map[string]any{"selectFiles": []int{}},
				"res": map[string]any{"files": []map[string]any{
					{"name": "example.com", "path": "example.com", "size": 0},
				}},
			},
		})
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", tempRoot, nil)
	status, err := client.Info(context.Background(), &downloader.TaskHandle{ID: "task-1", Hash: taskPathKey})
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if status.Total != 19 || len(status.Files) != 1 || status.Files[0].Size != 19 {
		t.Fatalf("completed HTTP size = total %d, files %#v; want 19", status.Total, status.Files)
	}
}

func TestInfoDoesNotReadOutsideTaskRoot(t *testing.T) {
	t.Parallel()

	const taskPathKey = "c53314ac-3795-4ef4-a677-c546dfe4bf93"
	tempRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempRoot, "outside"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeResult(t, w, http.StatusOK, map[string]any{
			"id":     "task-1",
			"status": "done",
			"meta": map[string]any{
				"opts": map[string]any{"selectFiles": []int{}},
				"res": map[string]any{"files": []map[string]any{
					{"name": "../outside", "path": "../outside", "size": 0},
				}},
			},
		})
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", tempRoot, nil)
	if _, err := client.Info(context.Background(), &downloader.TaskHandle{ID: "task-1", Hash: taskPathKey}); err == nil {
		t.Fatal("unsafe remote path must be rejected before transfer")
	}
}

func TestSetFilesToDownloadPatchesZeroBasedSelection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Token") != "test-token" {
			t.Fatalf("unexpected API token")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/task-1":
			writeResult(t, w, http.StatusOK, map[string]any{
				"id": "task-1",
				"meta": map[string]any{
					"opts": map[string]any{"selectFiles": []int{0, 1}},
					"res": map[string]any{"files": []map[string]any{
						{"name": "one.txt", "path": "one.txt", "size": 10},
						{"name": "two.txt", "path": "two.txt", "size": 10},
					}},
				},
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/tasks/task-1":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			var request struct {
				Opts struct {
					SelectFiles []int `json:"selectFiles"`
				} `json:"opts"`
			}
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(request.Opts.SelectFiles, []int{1}) {
				t.Fatalf("selectFiles = %#v, want [1]", request.Opts.SelectFiles)
			}
			writeResult(t, w, http.StatusOK, nil)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	err := client.SetFilesToDownload(context.Background(), &downloader.TaskHandle{ID: "task-1"},
		&downloader.SetFileToDownloadArgs{Index: 0, Download: false},
	)
	if err != nil {
		t.Fatalf("SetFilesToDownload() error = %v", err)
	}
}

func TestSetFilesToDownloadMaterializesGopeedDefaultSelection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/task-1":
			writeResult(t, w, http.StatusOK, map[string]any{
				"id": "task-1",
				"meta": map[string]any{
					"opts": map[string]any{"selectFiles": []int{}},
					"res": map[string]any{"files": []map[string]any{
						{"name": "one.txt", "path": "one.txt", "size": 10},
						{"name": "two.txt", "path": "two.txt", "size": 10},
					}},
				},
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/tasks/task-1":
			var request struct {
				Opts struct {
					SelectFiles []int `json:"selectFiles"`
				} `json:"opts"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(request.Opts.SelectFiles, []int{1}) {
				t.Fatalf("selectFiles = %#v, want [1]", request.Opts.SelectFiles)
			}
			writeResult(t, w, http.StatusOK, nil)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	if err := client.SetFilesToDownload(context.Background(), &downloader.TaskHandle{ID: "task-1"},
		&downloader.SetFileToDownloadArgs{Index: 0, Download: false},
	); err != nil {
		t.Fatalf("SetFilesToDownload() error = %v", err)
	}
}

func TestCreateAndCancelKeepCleanupInsideConfiguredTaskRoot(t *testing.T) {
	t.Parallel()

	var taskID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Token") != "test-token" {
			t.Fatalf("unexpected API token")
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/resolve":
			var request struct {
				Opts struct {
					Path string `json:"path"`
				} `json:"opts"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			taskID = filepath.Base(request.Opts.Path)
			if filepath.Dir(request.Opts.Path) != "/app/Downloads" || taskID == "." || taskID == "/" {
				t.Fatalf("unsafe Gopeed task path: %q", request.Opts.Path)
			}
			writeResult(t, w, http.StatusOK, map[string]any{"id": "resolve-1", "res": map[string]any{"files": []map[string]any{{"name": "file", "path": "file", "size": 1}}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
			var request struct {
				ResolveID string `json:"rid"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.ResolveID != "resolve-1" {
				t.Fatalf("resolve id = %q", request.ResolveID)
			}
			writeResult(t, w, http.StatusOK, "gopeed-task")
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/tasks/gopeed-task":
			if r.URL.Query().Get("force") != "true" {
				t.Fatalf("force query = %q", r.URL.Query().Get("force"))
			}
			writeResult(t, w, http.StatusOK, nil)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newClient(server.URL, "test-token", "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	handle, err := client.CreateTask(context.Background(), "https://example.com/file", nil)
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if handle.ID != "gopeed-task" || taskID == "" {
		t.Fatalf("unexpected task handle/path: %#v, %q", handle, taskID)
	}
	if err := client.Cancel(context.Background(), handle); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
}

func writeResult(t *testing.T, w http.ResponseWriter, status int, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "", "data": data}); err != nil {
		t.Fatal(err)
	}
}

func writeResultWithCode(t *testing.T, w http.ResponseWriter, status, code int, message string, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": message, "data": data}); err != nil {
		t.Fatal(err)
	}
}
