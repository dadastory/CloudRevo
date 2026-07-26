package gopeed

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dadastory/CloudRevo/pkg/downloader"
)

func bencodeString(value string) []byte {
	return []byte(strconv.Itoa(len(value)) + ":" + value)
}

func torrentWithWebSeed(name, webSeed string, payload []byte) []byte {
	torrent, _ := torrentWithWebSeedInfoHash(name, webSeed, payload)
	return torrent
}

func torrentWithWebSeedInfoHash(name, webSeed string, payload []byte) ([]byte, string) {
	const pieceLength = 16 * 1024
	pieces := make([]byte, 0, ((len(payload)+pieceLength-1)/pieceLength)*sha1.Size)
	for offset := 0; offset < len(payload); offset += pieceLength {
		end := min(offset+pieceLength, len(payload))
		sum := sha1.Sum(payload[offset:end])
		pieces = append(pieces, sum[:]...)
	}

	info := bytes.NewBufferString("d")
	info.Write(bencodeString("length"))
	info.WriteString("i" + strconv.Itoa(len(payload)) + "e")
	info.Write(bencodeString("name"))
	info.Write(bencodeString(name))
	info.Write(bencodeString("piece length"))
	info.WriteString("i" + strconv.Itoa(pieceLength) + "e")
	info.Write(bencodeString("pieces"))
	info.Write(bencodeString(string(pieces)))
	info.WriteByte('e')

	torrent := bytes.NewBufferString("d")
	torrent.Write(bencodeString("info"))
	torrent.Write(info.Bytes())
	torrent.Write(bencodeString("url-list"))
	torrent.WriteByte('l')
	torrent.Write(bencodeString(webSeed))
	torrent.WriteByte('e')
	torrent.WriteByte('e')
	infoHash := sha1.Sum(info.Bytes())
	return torrent.Bytes(), hex.EncodeToString(infoHash[:])
}

func TestComposeGopeedDownloadContract(t *testing.T) {
	server := os.Getenv("GOPEED_CONTRACT_URL")
	token := os.Getenv("GOPEED_CONTRACT_TOKEN")
	if server == "" || token == "" {
		t.Skip("Gopeed Compose contract is enabled only by the Compose test profile")
	}

	client := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	handle, err := client.CreateTask(ctx, "http://download-fixture:8080/fixture.txt", nil)
	if err != nil {
		t.Fatalf("create task against Compose Gopeed: %v", err)
	}

	// Recreate the client before observing completion. CloudRevo persists only
	// the handle in its workflow state, so this verifies that a resumed worker
	// can continue with the same Gopeed task instead of relying on in-memory
	// downloader state.
	restartedClient := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	var status *downloader.TaskStatus
	for ctx.Err() == nil {
		status, err = restartedClient.Info(ctx, handle)
		if err == nil && status.State == downloader.StatusCompleted {
			break
		}
		if err != nil && !errors.Is(err, downloader.ErrTaskNotFount) {
			t.Fatalf("read task status from Compose Gopeed: %v", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal("timed out waiting for Compose Gopeed download")
	}
	if status == nil || status.Total == 0 || status.SavePath == "" {
		t.Fatalf("unexpected completed status: %#v", status)
	}
	contents, err := os.ReadFile(filepath.Join(status.SavePath, "fixture.txt"))
	if err != nil {
		t.Fatalf("read completed output from CloudRevo shared mount: %v", err)
	}
	if string(contents) != "CloudRevo Gopeed contract fixture.\n" {
		t.Fatalf("unexpected completed output: %q", contents)
	}

	if err := restartedClient.Cancel(ctx, handle); err != nil {
		t.Fatalf("cancel completed Gopeed task: %v", err)
	}
	if _, err := os.Stat(status.SavePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary Gopeed task directory still exists after cleanup: %v", err)
	}
}

func TestComposeGopeedRequestLifecycleContract(t *testing.T) {
	server := os.Getenv("GOPEED_CONTRACT_URL")
	token := os.Getenv("GOPEED_CONTRACT_TOKEN")
	if server == "" || token == "" {
		t.Skip("Gopeed Compose contract is enabled only by the Compose test profile")
	}
	sourceHost := os.Getenv("GOPEED_CONTRACT_SOURCE_HOST")
	if sourceHost == "" {
		t.Fatal("GOPEED_CONTRACT_SOURCE_HOST must name the private Compose test service")
	}

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen for private fixture: %v", err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split private fixture address: %v", err)
	}

	var authorizedRequests atomic.Int32
	fixture := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authorized.txt":
			if r.Header.Get("Referer") != "https://portal.example.test/" || r.Header.Get("Cookie") != "session=authorized" {
				http.Error(w, "missing authorized request context", http.StatusForbidden)
				return
			}
			authorizedRequests.Add(1)
			w.Header().Set("Content-Disposition", `attachment; filename="authorized.txt"`)
			w.Header().Set("Content-Length", strconv.Itoa(len("authorized fixture\n")))
			_, _ = w.Write([]byte("authorized fixture\n"))
		case "/forbidden":
			http.Error(w, "forbidden", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	})}
	go func() { _ = fixture.Serve(listener) }()
	defer fixture.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	baseURL := "http://" + sourceHost + ":" + port
	requestOptions := &downloader.RequestOptions{Headers: map[string]string{
		"Referer": "https://portal.example.test/",
		"Cookie":  "session=authorized",
	}}

	preview, err := client.PreviewTask(ctx, baseURL+"/authorized.txt", nil, requestOptions, nil)
	if err != nil {
		t.Fatalf("preflight authorized Compose source: %v", err)
	}
	if len(preview.Files) == 0 {
		t.Fatalf("preflight returned no file metadata: %#v", preview)
	}

	handle, err := client.CreateTaskWithRequestOptions(ctx, baseURL+"/authorized.txt", nil, requestOptions)
	if err != nil {
		t.Fatalf("create authorized Compose task: %v", err)
	}
	defer func() {
		if err := client.Cancel(context.Background(), handle); err != nil {
			t.Errorf("clean up authorized Compose task: %v", err)
		}
	}()

	var status *downloader.TaskStatus
	for ctx.Err() == nil {
		status, err = client.Info(ctx, handle)
		if err != nil {
			t.Fatalf("read authorized Compose task: %v", err)
		}
		if status.State == downloader.StatusCompleted {
			break
		}
		if status.State == downloader.StatusError {
			t.Fatalf("authorized Compose task failed: %s", status.ErrorMessage)
		}
		time.Sleep(250 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal("timed out waiting for authorized Compose task")
	}
	if status == nil || status.SavePath == "" {
		t.Fatalf("unexpected authorized Compose task status: %#v", status)
	}
	if authorizedRequests.Load() == 0 {
		t.Fatal("Gopeed did not forward authorized request headers to the private fixture")
	}

	_, err = client.CreateTask(ctx, baseURL+"/forbidden", nil)
	var sourceErr *downloader.SourceHTTPError
	if !errors.As(err, &sourceErr) || sourceErr.StatusCode != http.StatusForbidden {
		t.Fatalf("forbidden Compose source error = %v, want HTTP 403 SourceHTTPError", err)
	}
}

func TestComposeGopeedPauseAndContinueContract(t *testing.T) {
	server := os.Getenv("GOPEED_CONTRACT_URL")
	token := os.Getenv("GOPEED_CONTRACT_TOKEN")
	sourceHost := os.Getenv("GOPEED_CONTRACT_SOURCE_HOST")
	if server == "" || token == "" {
		t.Skip("Gopeed Compose contract is enabled only by the Compose test profile")
	}
	if sourceHost == "" {
		t.Fatal("GOPEED_CONTRACT_SOURCE_HOST must name the private Compose test service")
	}

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen for pause fixture: %v", err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split pause fixture address: %v", err)
	}

	chunk := bytes.Repeat([]byte("p"), 32*1024)
	const chunks = 80
	fixture := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/slow.bin" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(chunk)*chunks))
		flusher, _ := w.(http.Flusher)
		for index := 0; index < chunks; index++ {
			_, _ = w.Write(chunk)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(35 * time.Millisecond)
		}
	})}
	go func() { _ = fixture.Serve(listener) }()
	defer fixture.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	client := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	handle, err := client.CreateTask(ctx, "http://"+sourceHost+":"+port+"/slow.bin", nil)
	if err != nil {
		t.Fatalf("create pausable task: %v", err)
	}
	defer func() { _ = client.Cancel(context.Background(), handle) }()

	for ctx.Err() == nil {
		status, err := client.Info(ctx, handle)
		if err != nil {
			t.Fatalf("read task before pause: %v", err)
		}
		if status.Downloaded > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal("task never started before pause")
	}
	if err := client.Pause(ctx, handle); err != nil {
		t.Fatalf("pause Gopeed task: %v", err)
	}

	paused := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		status, err := client.Info(ctx, handle)
		if err != nil {
			t.Fatalf("read paused task: %v", err)
		}
		if status.State == downloader.StatusPaused {
			paused = true
			break
		}
	}
	if !paused {
		t.Fatal("Gopeed task did not enter paused state")
	}
	if err := client.Continue(ctx, handle); err != nil {
		t.Fatalf("continue Gopeed task: %v", err)
	}
	for ctx.Err() == nil {
		status, err := client.Info(ctx, handle)
		if err != nil {
			t.Fatalf("read continued task: %v", err)
		}
		if status.State == downloader.StatusCompleted {
			return
		}
		if status.State == downloader.StatusError {
			t.Fatalf("continued task failed: %s", status.ErrorMessage)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("continued task did not finish: %v", ctx.Err())
}

func TestComposeGopeedBitTorrentTelemetryContract(t *testing.T) {
	server := os.Getenv("GOPEED_CONTRACT_URL")
	token := os.Getenv("GOPEED_CONTRACT_TOKEN")
	sourceHost := os.Getenv("GOPEED_CONTRACT_SOURCE_HOST")
	if server == "" || token == "" {
		t.Skip("Gopeed Compose contract is enabled only by the Compose test profile")
	}
	if sourceHost == "" {
		t.Fatal("GOPEED_CONTRACT_SOURCE_HOST must name the private Compose test service")
	}

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen for private BitTorrent fixture: %v", err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split private BitTorrent fixture address: %v", err)
	}

	payload := bytes.Repeat([]byte("CloudRevo local BitTorrent fixture.\n"), 64*1024)
	baseURL := "http://" + sourceHost + ":" + port
	torrentData := torrentWithWebSeed("fixture.bin", baseURL+"/payload/", payload)
	fixture := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fixture.torrent":
			w.Header().Set("Content-Type", "application/x-bittorrent")
			_, _ = w.Write(torrentData)
		case "/payload/fixture.bin":
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			flusher, _ := w.(http.Flusher)
			for offset := 0; offset < len(payload); offset += 16 * 1024 {
				end := min(offset+16*1024, len(payload))
				_, _ = w.Write(payload[offset:end])
				if flusher != nil {
					flusher.Flush()
				}
				time.Sleep(10 * time.Millisecond)
			}
		default:
			http.NotFound(w, r)
		}
	})}
	go func() { _ = fixture.Serve(listener) }()
	defer fixture.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	handle, err := client.CreateTask(ctx, baseURL+"/fixture.torrent", map[string]any{
		"autoTorrent":                true,
		"deleteTorrentAfterDownload": true,
	})
	if err != nil {
		t.Fatalf("create local BitTorrent task: %v", err)
	}

	btHandle := &downloader.TaskHandle{Hash: handle.Hash, ParentID: handle.ID}
	var status *downloader.TaskStatus
	for ctx.Err() == nil {
		var tasks []task
		if err := client.call(ctx, http.MethodGet, "/api/v1/tasks", nil, &tasks); err != nil {
			t.Fatalf("list local BitTorrent tasks: %v", err)
		}
		for _, candidate := range tasks {
			if candidate.ID == handle.ID && candidate.FollowedBy != "" {
				btHandle.ID = candidate.FollowedBy
				break
			}
		}
		if btHandle.ID == "" {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		status, err = client.Info(ctx, btHandle)
		if err != nil {
			t.Fatalf("read local BitTorrent task status: %v", err)
		}
		if status.Hash != "" && len(status.Files) == 1 && status.Files[0].ProgressKnown {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal("timed out waiting for local BitTorrent telemetry")
	}
	if status.Hash == "" || !status.Files[0].ProgressKnown || status.Files[0].Progress < 0 || status.Files[0].Progress > 1 {
		t.Fatalf("unexpected BitTorrent telemetry: %#v", status)
	}

	if err := client.Cancel(ctx, btHandle); err != nil {
		t.Fatalf("cancel active local BitTorrent task: %v", err)
	}
	if _, err := os.Stat(status.SavePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary BitTorrent task directory still exists after cleanup: %v", err)
	}
}

func TestComposeGopeedDescriptorEndpointFollowsNativeBitTorrentTask(t *testing.T) {
	server := os.Getenv("GOPEED_CONTRACT_URL")
	token := os.Getenv("GOPEED_CONTRACT_TOKEN")
	sourceHost := os.Getenv("GOPEED_CONTRACT_SOURCE_HOST")
	if server == "" || token == "" {
		t.Skip("Gopeed Compose contract is enabled only by the Compose test profile")
	}
	if sourceHost == "" {
		t.Fatal("GOPEED_CONTRACT_SOURCE_HOST must name the private Compose test service")
	}

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen for direct torrent fixture: %v", err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split direct torrent fixture address: %v", err)
	}

	payload := bytes.Repeat([]byte("CloudRevo direct torrent fixture.\n"), 64*1024)
	baseURL := "http://" + sourceHost + ":" + port
	torrentData := torrentWithWebSeed("direct-fixture.bin", baseURL+"/payload/", payload)
	fixture := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download":
			w.Header().Set("Content-Type", "application/x-bittorrent")
			w.Header().Set("Content-Disposition", `attachment; filename="direct-fixture.torrent"`)
			_, _ = w.Write(torrentData)
		case "/payload/direct-fixture.bin":
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	})}
	go func() { _ = fixture.Serve(listener) }()
	defer fixture.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	handle, err := client.CreateTaskWithOptions(ctx, baseURL+"/download?id=fixture", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("create direct torrent URL task: %v", err)
	}
	defer func() { _ = client.Cancel(context.Background(), handle) }()

	for ctx.Err() == nil {
		status, err := client.Info(ctx, handle)
		if err != nil {
			t.Fatalf("read descriptor endpoint task: %v", err)
		}
		if status.FollowedBy != nil {
			handle = status.FollowedBy
			status, err = client.Info(ctx, handle)
			if err != nil {
				t.Fatalf("read followed BitTorrent task: %v", err)
			}
			if status.Hash != "" && len(status.Files) == 1 && status.Files[0].Name == "direct-fixture.bin" {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("descriptor endpoint never followed a native BitTorrent task")
}

func TestComposeGopeedMagnetTaskLifecycleContract(t *testing.T) {
	server := os.Getenv("GOPEED_CONTRACT_URL")
	token := os.Getenv("GOPEED_CONTRACT_TOKEN")
	sourceHost := os.Getenv("GOPEED_CONTRACT_SOURCE_HOST")
	if server == "" || token == "" {
		t.Skip("Gopeed Compose contract is enabled only by the Compose test profile")
	}
	if sourceHost == "" {
		t.Fatal("GOPEED_CONTRACT_SOURCE_HOST must name the private Compose test service")
	}

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen for local Magnet fixture: %v", err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split local Magnet fixture address: %v", err)
	}

	payload := bytes.Repeat([]byte("CloudRevo local Magnet fixture.\n"), 512)
	baseURL := "http://" + sourceHost + ":" + port
	torrentData, infoHash := torrentWithWebSeedInfoHash("magnet-fixture.bin", baseURL+"/payload/", payload)
	fixture := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fixture.torrent":
			w.Header().Set("Content-Type", "application/x-bittorrent")
			_, _ = w.Write(torrentData)
		case "/payload/magnet-fixture.bin":
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	})}
	go func() { _ = fixture.Serve(listener) }()
	defer fixture.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	magnet := "magnet:?xt=urn:btih:" + infoHash + "&dn=magnet-fixture.bin&xl=" + strconv.Itoa(len(payload)) + "&ws=" + url.QueryEscape(baseURL+"/payload/magnet-fixture.bin") + "&xs=" + url.QueryEscape(baseURL+"/fixture.torrent")
	client := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	handle, err := client.CreateTask(ctx, magnet, nil)
	if err != nil {
		t.Fatalf("create local Magnet task: %v", err)
	}
	defer func() { _ = client.Cancel(context.Background(), handle) }()

	var status *downloader.TaskStatus
	for ctx.Err() == nil {
		status, err = client.Info(ctx, handle)
		if err != nil {
			t.Fatalf("read local Magnet task: %v", err)
		}
		if status.State == downloader.StatusError {
			t.Fatalf("local Magnet task failed: %#v", status)
		}
		if status.Name == "magnet-fixture.bin" && len(status.Files) == 1 && status.Hash != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal("timed out waiting for local Magnet metadata")
	}
	if err := client.Cancel(ctx, handle); err != nil {
		t.Fatalf("cancel local Magnet task: %v", err)
	}
	if _, err := os.Stat(status.SavePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary Magnet task directory still exists after cleanup: %v", err)
	}
}

func TestComposeGopeedED2KDirectTaskContract(t *testing.T) {
	server := os.Getenv("GOPEED_CONTRACT_URL")
	token := os.Getenv("GOPEED_CONTRACT_TOKEN")
	if server == "" || token == "" {
		t.Skip("Gopeed Compose contract is enabled only by the Compose test profile")
	}

	// eD2K is queued directly, just as the product flow does. The contract
	// proves task construction and immediately cleans it up; it does not depend
	// on a public peer becoming available.
	client := newClient(server, token, "/app/Downloads", "/cloudrevo/data/temp/gopeed", nil)
	handle, err := client.CreateTaskWithOptions(context.Background(), "ed2k://|file|cloudrevo-fixture.bin|1|0123456789ABCDEF0123456789ABCDEF|/", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("create direct ED2K source: %v", err)
	}
	if handle == nil || handle.ID == "" {
		t.Fatalf("unexpected ED2K task handle: %#v", handle)
	}
	if err := client.Cancel(context.Background(), handle); err != nil {
		t.Fatalf("clean up direct ED2K task: %v", err)
	}
}
