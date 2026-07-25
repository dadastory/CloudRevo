package downloader

import (
	"context"
	"encoding/gob"
	"fmt"
	"net/http"
)

var (
	ErrTaskNotFount = fmt.Errorf("task not found")
)

type (
	Downloader interface {
		// Create a task with the given URL and options overwriting the default settings, returns a task handle for future operations.
		CreateTask(ctx context.Context, url string, options map[string]interface{}) (*TaskHandle, error)
		// Info returns the status of the task with the given handle.
		Info(ctx context.Context, handle *TaskHandle) (*TaskStatus, error)
		// Cancel the task with the given handle.
		Cancel(ctx context.Context, handle *TaskHandle) error
		// SetFilesToDownload sets the files to download for the task with the given handle.
		SetFilesToDownload(ctx context.Context, handle *TaskHandle, args ...*SetFileToDownloadArgs) error
		// Test tests the connection to the downloader.
		Test(ctx context.Context) (string, error)
	}
	// RequestOptionsDownloader is implemented by downloaders that can apply
	// request-specific HTTP context without turning it into a global node setting.
	RequestOptionsDownloader interface {
		CreateTaskWithRequestOptions(ctx context.Context, url string, options map[string]interface{}, requestOptions *RequestOptions) (*TaskHandle, error)
	}
	// SelectedFilesDownloader is implemented by downloaders that can choose
	// resolved files before a task is started.
	SelectedFilesDownloader interface {
		CreateTaskWithOptions(ctx context.Context, url string, options map[string]interface{}, requestOptions *RequestOptions, taskOptions *TaskOptions, selectedFiles []int) (*TaskHandle, error)
	}
	// PreviewDownloader resolves a source without creating a runnable task.
	// Implementations must release all temporary resolver state before return.
	PreviewDownloader interface {
		PreviewTask(ctx context.Context, url string, options map[string]interface{}, requestOptions *RequestOptions, taskOptions *TaskOptions) (*TaskStatus, error)
	}
	// RequestOptions holds validated HTTP request context for a single remote download.
	// It is stored by the workflow in private task state only.
	RequestOptions struct {
		Method  string            `json:"method,omitempty"`
		Headers map[string]string `json:"headers,omitempty"`
		Body    string            `json:"body,omitempty"`
	}
	// TaskOptions holds the narrowly-scoped Gopeed settings that are safe to
	// configure for an individual remote download.
	TaskOptions struct {
		Connections int `json:"connections,omitempty"`
		// AutoTorrent is set only by the workflow for a CloudRevo-hosted
		// torrent file. It deliberately cannot be supplied through the API.
		AutoTorrent bool `json:"-"`
	}
	// SourceHTTPError identifies an HTTP response returned by the requested
	// download source, rather than by the downloader service itself.
	SourceHTTPError struct {
		StatusCode int
	}

	// TaskHandle represents a task handle for future operations
	TaskHandle struct {
		ID       string `json:"id"`
		Hash     string `json:"hash"`
		ParentID string `json:"parent_id,omitempty"`
	}
	Status     string
	TaskStatus struct {
		FollowedBy    *TaskHandle `json:"-"` // Indicate if the task handle is changed
		SavePath      string      `json:"save_path,omitempty"`
		Name          string      `json:"name"`
		State         Status      `json:"state"`
		Total         int64       `json:"total"`
		Downloaded    int64       `json:"downloaded"`
		DownloadSpeed int64       `json:"download_speed"`
		Uploaded      int64       `json:"uploaded"`
		UploadSpeed   int64       `json:"upload_speed"`
		Hash          string      `json:"hash,omitempty"`
		Files         []TaskFile  `json:"files,omitempty"`
		Pieces        []byte      `json:"pieces,omitempty"` // Hexadecimal representation of the download progress of the peer. The highest bit corresponds to the piece at index 0.
		NumPieces     int         `json:"num_pieces,omitempty"`
		ErrorMessage  string      `json:"error_message,omitempty"`
	}

	TaskFile struct {
		Index         int     `json:"index"`
		Name          string  `json:"name"`
		Size          int64   `json:"size"`
		Progress      float64 `json:"progress"`
		ProgressKnown bool    `json:"progress_known,omitempty"`
		Selected      bool    `json:"selected"`
	}

	SetFileToDownloadArgs struct {
		Index    int  `json:"index"`
		Download bool `json:"download"`
	}
)

func (e *SourceHTTPError) Error() string {
	return fmt.Sprintf("source server rejected the download request (HTTP %d)", e.StatusCode)
}

func (e *SourceHTTPError) IsClientError() bool {
	return e.StatusCode >= http.StatusBadRequest && e.StatusCode < http.StatusInternalServerError
}

const (
	StatusDownloading Status = "downloading"
	StatusSeeding     Status = "seeding"
	StatusCompleted   Status = "completed"
	StatusError       Status = "error"
	StatusUnknown     Status = "unknown"

	DownloaderCtxKey = "downloader"
)

func init() {
	gob.Register(TaskHandle{})
	gob.Register(TaskStatus{})
}
