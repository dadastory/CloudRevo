package gopeed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/downloader"
	"github.com/gofrs/uuid"
)

type client struct {
	server       string
	token        string
	downloadRoot string
	tempRoot     string
	options      map[string]any
	httpClient   *http.Client
}

const composeTokenEnv = "CR_GOPEED_API_TOKEN"

var sourceHTTPStatusPattern = regexp.MustCompile(`(?i)http request fail, code:([1-5][0-9]{2})`)

type apiResult struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type apiError struct {
	Code    int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("Gopeed API error %d: %s", e.Code, e.Message)
}

const gopeedTaskNotFoundCode = 2001

type task struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Uploading bool   `json:"uploading"`
	Size      int64  `json:"size"`
	Progress  struct {
		Downloaded  int64 `json:"downloaded"`
		Speed       int64 `json:"speed"`
		Uploaded    int64 `json:"uploaded"`
		UploadSpeed int64 `json:"uploadSpeed"`
	} `json:"progress"`
	Meta struct {
		Opts struct {
			SelectFiles []int `json:"selectFiles"`
		} `json:"opts"`
		Res struct {
			Name  string `json:"name"`
			Files []struct {
				Name string `json:"name"`
				Path string `json:"path"`
				Size int64  `json:"size"`
			} `json:"files"`
		} `json:"res"`
	} `json:"meta"`
}

func newClient(server, token, downloadRoot, tempRoot string, options map[string]any) *client {
	return &client{
		server:       strings.TrimRight(server, "/"),
		token:        token,
		downloadRoot: strings.TrimRight(downloadRoot, "/"),
		tempRoot:     strings.TrimRight(tempRoot, "/"),
		options:      options,
		httpClient:   http.DefaultClient,
	}
}

func New(options *types.GopeedSetting) (downloader.Downloader, error) {
	if options == nil || options.Server == "" || options.DownloadPath == "" || options.TempPath == "" {
		return nil, fmt.Errorf("incomplete Gopeed configuration")
	}
	token := options.Token
	if composeToken := os.Getenv(composeTokenEnv); composeToken != "" {
		token = composeToken
	}
	if token == "" {
		return nil, fmt.Errorf("incomplete Gopeed configuration")
	}
	server, err := url.Parse(options.Server)
	if err != nil || (server.Scheme != "http" && server.Scheme != "https") || server.Host == "" {
		return nil, fmt.Errorf("invalid Gopeed server URL")
	}
	if !strings.HasPrefix(options.DownloadPath, "/") || !strings.HasPrefix(options.TempPath, "/") {
		return nil, fmt.Errorf("Gopeed download roots must be absolute")
	}
	return newClient(options.Server, token, options.DownloadPath, options.TempPath, options.Options), nil
}

func (c *client) CreateTask(ctx context.Context, source string, groupOptions map[string]interface{}) (*downloader.TaskHandle, error) {
	return c.CreateTaskWithRequestOptions(ctx, source, groupOptions, nil)
}

func (c *client) CreateTaskWithRequestOptions(ctx context.Context, source string, groupOptions map[string]interface{}, requestOptions *downloader.RequestOptions) (*downloader.TaskHandle, error) {
	return c.CreateTaskWithOptions(ctx, source, groupOptions, requestOptions, nil, nil)
}

func (c *client) CreateTaskWithOptions(ctx context.Context, source string, groupOptions map[string]interface{}, requestOptions *downloader.RequestOptions, taskOptions *downloader.TaskOptions, selectedFiles []int) (*downloader.TaskHandle, error) {
	guid, err := uuid.NewV4()
	if err != nil {
		return nil, fmt.Errorf("create task UUID: %w", err)
	}

	taskDir := guid.String()
	opts := map[string]any{
		"path": path.Join(c.downloadRoot, taskDir),
	}
	if len(selectedFiles) > 0 {
		opts["selectFiles"] = selectedFiles
	}
	if extra := gopeedTaskOptions(c.options, groupOptions, taskOptions); len(extra) > 0 {
		opts["extra"] = extra
	}

	var resolved struct {
		ID string `json:"id"`
	}
	request := map[string]any{"url": source}
	if requestOptions != nil && (requestOptions.Method != "" || len(requestOptions.Headers) > 0 || requestOptions.Body != "") {
		request["extra"] = map[string]any{
			"method": requestOptions.Method,
			"header": requestOptions.Headers,
			"body":   requestOptions.Body,
		}
	}
	if err := c.call(ctx, http.MethodPost, "/api/v1/resolve", map[string]any{
		"req":  request,
		"opts": opts,
	}, &resolved); err != nil {
		return nil, fmt.Errorf("resolve Gopeed task: %w", err)
	}
	if resolved.ID == "" {
		return nil, fmt.Errorf("resolve Gopeed task: empty resolve id")
	}

	var taskID string
	if err := c.call(ctx, http.MethodPost, "/api/v1/tasks", map[string]any{"rid": resolved.ID}, &taskID); err != nil {
		return nil, fmt.Errorf("create Gopeed task: %w", err)
	}
	if taskID == "" {
		return nil, fmt.Errorf("create Gopeed task: empty task id")
	}

	return &downloader.TaskHandle{ID: taskID, Hash: taskDir}, nil
}

func (c *client) PreviewTask(ctx context.Context, source string, groupOptions map[string]interface{}, requestOptions *downloader.RequestOptions, taskOptions *downloader.TaskOptions) (*downloader.TaskStatus, error) {
	guid, err := uuid.NewV4()
	if err != nil {
		return nil, fmt.Errorf("create preview UUID: %w", err)
	}
	request := map[string]any{"url": source}
	if requestOptions != nil && (requestOptions.Method != "" || len(requestOptions.Headers) > 0 || requestOptions.Body != "") {
		request["extra"] = map[string]any{"method": requestOptions.Method, "header": requestOptions.Headers, "body": requestOptions.Body}
	}
	opts := map[string]any{"path": path.Join(c.downloadRoot, "preview-"+guid.String())}
	if extra := gopeedTaskOptions(c.options, groupOptions, taskOptions); len(extra) > 0 {
		opts["extra"] = extra
	}
	var resolved struct {
		ID  string `json:"id"`
		Res struct {
			Name  string `json:"name"`
			Files []struct {
				Name string `json:"name"`
				Path string `json:"path"`
				Size int64  `json:"size"`
			} `json:"files"`
		} `json:"res"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/v1/resolve", map[string]any{"req": request, "opts": opts}, &resolved); err != nil {
		return nil, fmt.Errorf("resolve Gopeed task: %w", err)
	}
	// Some protocol extensions return a complete resource directly and do not
	// allocate a resolver ID. Only retained resolvers need explicit cleanup.
	if resolved.ID != "" {
		if err := c.call(ctx, http.MethodDelete, "/api/v1/resolve/"+url.PathEscape(resolved.ID), nil, nil); err != nil {
			return nil, fmt.Errorf("discard Gopeed preview: %w", err)
		}
	}
	files := make([]downloader.TaskFile, 0, len(resolved.Res.Files))
	displayName := resolved.Res.Name
	var total int64
	for index, file := range resolved.Res.Files {
		fileName := file.Path
		if fileName == "" {
			fileName = file.Name
		}
		if index == 0 && displayName == "" {
			displayName = file.Name
		}
		files = append(files, downloader.TaskFile{Index: index, Name: fileName, Size: file.Size, Selected: true})
		total += file.Size
	}
	if displayName == "" && len(files) > 0 {
		displayName = files[0].Name
	}
	return &downloader.TaskStatus{Name: displayName, Total: total, Files: files}, nil
}

func (c *client) Info(ctx context.Context, handle *downloader.TaskHandle) (*downloader.TaskStatus, error) {
	if !isTaskUUID(handle.Hash) {
		return nil, fmt.Errorf("invalid Gopeed task path key")
	}

	item, err := c.getTask(ctx, handle.ID)
	if err != nil {
		return nil, err
	}

	selected := selectedFileIndexes(item)
	state := mapStatus(item.Status, item.Uploading)

	files := make([]downloader.TaskFile, 0, len(item.Meta.Res.Files))
	total := item.Size
	for index, file := range item.Meta.Res.Files {
		name := file.Path
		if name == "" {
			name = file.Name
		}
		if _, ok := localTaskFilePath(c.tempRoot, handle.Hash, name); !ok {
			return nil, fmt.Errorf("unsafe Gopeed task file path %q", name)
		}
		size := file.Size
		if size == 0 && state == downloader.StatusCompleted {
			if localSize, ok := localTaskFileSize(c.tempRoot, handle.Hash, name); ok {
				size = localSize
			}
		}
		if item.Size == 0 {
			total += size
		}
		files = append(files, downloader.TaskFile{
			Index:    index,
			Name:     name,
			Size:     size,
			Selected: selected[index],
		})
	}
	progress := float64(0)
	if total > 0 {
		progress = float64(item.Progress.Downloaded) / float64(total)
		if progress > 1 {
			progress = 1
		}
	}
	for index := range files {
		files[index].Progress = progress
	}

	name := displayName(item.Name, item.Meta.Res.Name, item.Meta.Res.Files)
	return &downloader.TaskStatus{
		Name:          name,
		State:         state,
		Total:         total,
		Downloaded:    item.Progress.Downloaded,
		DownloadSpeed: item.Progress.Speed,
		Uploaded:      item.Progress.Uploaded,
		UploadSpeed:   item.Progress.UploadSpeed,
		SavePath:      path.Join(c.tempRoot, handle.Hash),
		Files:         files,
		ErrorMessage:  errorMessage(item.Status),
	}, nil
}

func (c *client) Cancel(ctx context.Context, handle *downloader.TaskHandle) error {
	if err := c.call(ctx, http.MethodDelete, "/api/v1/tasks/"+url.PathEscape(handle.ID)+"?force=true", nil, nil); err != nil {
		var apiErr *apiError
		if !errors.As(err, &apiErr) || apiErr.Code != gopeedTaskNotFoundCode {
			return fmt.Errorf("delete Gopeed task: %w", err)
		}
	}
	if isTaskUUID(handle.Hash) {
		if err := os.RemoveAll(path.Join(c.tempRoot, handle.Hash)); err != nil {
			return fmt.Errorf("remove Gopeed temporary task directory: %w", err)
		}
	}
	return nil
}

func (c *client) SetFilesToDownload(ctx context.Context, handle *downloader.TaskHandle, args ...*downloader.SetFileToDownloadArgs) error {
	item, err := c.getTask(ctx, handle.ID)
	if err != nil {
		return err
	}
	selected := selectedFileIndexes(item)
	for _, arg := range args {
		if arg.Download {
			selected[arg.Index] = true
		} else {
			delete(selected, arg.Index)
		}
	}

	indices := make([]int, 0, len(selected))
	for index := range selected {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	if err := c.call(ctx, http.MethodPatch, "/api/v1/tasks/"+url.PathEscape(handle.ID), map[string]any{
		"opts": map[string]any{"selectFiles": indices},
	}, nil); err != nil {
		return fmt.Errorf("patch Gopeed selected files: %w", err)
	}
	return nil
}

func (c *client) Test(ctx context.Context) (string, error) {
	var info struct {
		Version string `json:"version"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/v1/info", nil, &info); err != nil {
		return "", fmt.Errorf("test Gopeed connection: %w", err)
	}
	return info.Version, nil
}

func selectedFileIndexes(item *task) map[int]bool {
	selected := make(map[int]bool, len(item.Meta.Opts.SelectFiles))
	if len(item.Meta.Opts.SelectFiles) == 0 {
		for index := range item.Meta.Res.Files {
			selected[index] = true
		}
		return selected
	}
	for _, index := range item.Meta.Opts.SelectFiles {
		selected[index] = true
	}
	return selected
}

func localTaskFileSize(tempRoot, taskKey, fileName string) (int64, bool) {
	candidate, ok := localTaskFilePath(tempRoot, taskKey, fileName)
	if !ok {
		return 0, false
	}
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() {
		return 0, false
	}
	return info.Size(), true
}

func localTaskFilePath(tempRoot, taskKey, fileName string) (string, bool) {
	if fileName == "" || filepath.IsAbs(filepath.FromSlash(fileName)) {
		return "", false
	}
	taskRoot := filepath.Clean(filepath.Join(tempRoot, taskKey))
	candidate := filepath.Clean(filepath.Join(taskRoot, filepath.FromSlash(fileName)))
	rel, err := filepath.Rel(taskRoot, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return candidate, true
}

func (c *client) getTask(ctx context.Context, id string) (*task, error) {
	var item task
	if err := c.call(ctx, http.MethodGet, "/api/v1/tasks/"+url.PathEscape(id), nil, &item); err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.Code == gopeedTaskNotFoundCode {
			return nil, downloader.ErrTaskNotFount
		}
		return nil, fmt.Errorf("get Gopeed task: %w", err)
	}
	return &item, nil
}

func (c *client) call(ctx context.Context, method, endpoint string, payload any, output any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.server+endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Token", c.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected HTTP status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var result apiResult
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	if result.Code != 0 {
		if matches := sourceHTTPStatusPattern.FindStringSubmatch(result.Msg); len(matches) == 2 {
			var statusCode int
			if _, err := fmt.Sscanf(matches[1], "%d", &statusCode); err == nil {
				return &downloader.SourceHTTPError{StatusCode: statusCode}
			}
		}
		return &apiError{Code: result.Code, Message: result.Msg}
	}
	if output != nil && len(result.Data) > 0 && string(result.Data) != "null" {
		return json.Unmarshal(result.Data, output)
	}
	return nil
}

func mergeOptions(defaults, overrides map[string]any) map[string]any {
	merged := make(map[string]any, len(defaults)+len(overrides))
	for key, value := range defaults {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

func gopeedTaskOptions(defaults, overrides map[string]any, taskOptions *downloader.TaskOptions) map[string]any {
	merged := mergeOptions(defaults, overrides)
	if taskOptions != nil && taskOptions.Connections != 0 {
		merged["connections"] = taskOptions.Connections
	}
	return merged
}

func displayName(taskName, resourceName string, files []struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}) string {
	if taskName != "" {
		return taskName
	}
	if resourceName != "" {
		return resourceName
	}
	if len(files) > 0 {
		return files[0].Name
	}
	return ""
}

func mapStatus(status string, uploading bool) downloader.Status {
	if uploading {
		return downloader.StatusSeeding
	}
	switch status {
	case "ready", "running", "pause", "wait":
		return downloader.StatusDownloading
	case "done":
		return downloader.StatusCompleted
	case "error":
		return downloader.StatusError
	default:
		return downloader.StatusUnknown
	}
}

func errorMessage(status string) string {
	if status == "error" {
		return "Gopeed task failed"
	}
	return ""
}

func isTaskUUID(value string) bool {
	_, err := uuid.FromString(value)
	return err == nil
}
