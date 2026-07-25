package explorer

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dadastory/CloudRevo/application/dependency"
	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/ent/task"
	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/downloader"
	"github.com/dadastory/CloudRevo/pkg/filemanager/fs"
	"github.com/dadastory/CloudRevo/pkg/filemanager/fs/dbfs"
	"github.com/dadastory/CloudRevo/pkg/filemanager/manager"
	"github.com/dadastory/CloudRevo/pkg/filemanager/workflows"
	"github.com/dadastory/CloudRevo/pkg/hashid"
	"github.com/dadastory/CloudRevo/pkg/queue"
	"github.com/dadastory/CloudRevo/pkg/request"
	"github.com/dadastory/CloudRevo/pkg/serializer"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid"
	"github.com/samber/lo"
	"golang.org/x/tools/container/intsets"
)

const (
	workflowEventHeartbeatInterval = 25 * time.Second
	magnetPreviewTimeout           = 15 * time.Second
)

// ItemMoveService 处理多文件/目录移动
type ItemMoveService struct {
	SrcDir string        `json:"src_dir" binding:"required,min=1,max=65535"`
	Src    ItemIDService `json:"src"`
	Dst    string        `json:"dst" binding:"required,min=1,max=65535"`
}

// ItemRenameService 处理多文件/目录重命名
type ItemRenameService struct {
	Src     ItemIDService `json:"src"`
	NewName string        `json:"new_name" binding:"required,min=1,max=255"`
}

// ItemService 处理多文件/目录相关服务
type ItemService struct {
	Items []uint `json:"items"`
	Dirs  []uint `json:"dirs"`
}

// ItemIDService 处理多文件/目录相关服务，字段值为HashID，可通过Raw()方法获取原始ID
type ItemIDService struct {
	Items      []string `json:"items"`
	Dirs       []string `json:"dirs"`
	Source     *ItemService
	Force      bool `json:"force"`
	UnlinkOnly bool `json:"unlink"`
}

// ItemDecompressService 文件解压缩任务服务
type ItemDecompressService struct {
	Src      string `json:"src"`
	Dst      string `json:"dst" binding:"required,min=1,max=65535"`
	Encoding string `json:"encoding"`
}

// ItemPropertyService 获取对象属性服务
type ItemPropertyService struct {
	ID        string `binding:"required"`
	TraceRoot bool   `form:"trace_root"`
	IsFolder  bool   `form:"is_folder"`
}

func init() {
	gob.Register(ItemIDService{})
}

type (
	DownloadWorkflowService struct {
		Src            []string                   `json:"src"`
		SrcFile        string                     `json:"src_file"`
		Dst            string                     `json:"dst" binding:"required"`
		RequestOptions *downloader.RequestOptions `json:"request,omitempty"`
		TaskOptions    *downloader.TaskOptions    `json:"gopeed,omitempty"`
		DisplayName    string                     `json:"display_name,omitempty"`
		SelectedFiles  []int                      `json:"selected_files,omitempty"`
	}
	CreateDownloadParamCtx  struct{}
	PreviewDownloadParamCtx struct{}
)

func (service *DownloadWorkflowService) CreateDownloadTask(c *gin.Context) ([]*TaskResponse, error) {
	dep := dependency.FromContext(c)
	hasher := dep.HashIDEncoder()
	if err := service.Validate(c); err != nil {
		return nil, err
	}

	// batch creating tasks
	ae := serializer.NewAggregateError()
	tasks := make([]queue.Task, 0, len(service.Src))
	for _, src := range service.Src {
		if src == "" {
			continue
		}

		t, err := workflows.NewRemoteDownloadTaskWithConfig(c, src, service.SrcFile, service.Dst, service.RequestOptions, service.TaskOptions, service.SelectedFiles, service.DisplayName)
		if err != nil {
			ae.Add(src, err)
			continue
		}

		if err := dep.RemoteDownloadQueue(c).QueueTask(c, t); err != nil {
			ae.Add(src, err)
		}

		tasks = append(tasks, t)
	}

	if service.SrcFile != "" {
		t, err := workflows.NewRemoteDownloadTask(c, "", service.SrcFile, service.Dst, nil)
		if err != nil {
			ae.Add(service.SrcFile, err)
		}

		if err := dep.RemoteDownloadQueue(c).QueueTask(c, t); err != nil {
			ae.Add(service.SrcFile, err)
		}

		tasks = append(tasks, t)
	}

	return lo.Map(tasks, func(item queue.Task, index int) *TaskResponse {
		return BuildTaskResponse(item, nil, hasher)
	}), ae.Aggregate()
}

// Validate rechecks every permission and input constraint required to create a
// remote-download task. Retry calls this too, so a stale task never bypasses
// current source, destination, or group authorization.
func (service *DownloadWorkflowService) Validate(c *gin.Context) error {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionRemoteDownload)) {
		return serializer.NewError(serializer.CodeGroupNotAllowed, "Group not allowed to download files", nil)
	}
	if service.SrcFile == "" && len(service.Src) == 0 {
		return serializer.NewError(serializer.CodeParamErr, "No source files", nil)
	}
	if service.SrcFile != "" && len(service.Src) > 0 {
		return serializer.NewError(serializer.CodeParamErr, "Invalid source files", nil)
	}
	dst, err := fs.NewUriFromString(service.Dst)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}
	if _, err = m.Get(c, dst, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityCreateFile)); err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}
	if limit := user.Edges.Group.Settings.RemoteDownloadBatchSize; limit > 0 && len(service.Src) > limit {
		return serializer.NewError(serializer.CodeBatchRemoteDownloadSize, "", nil)
	}
	if service.SrcFile != "" {
		if len(service.SelectedFiles) > 0 {
			return serializer.NewError(serializer.CodeParamErr, "Selected files require a URL source", nil)
		}
		if service.RequestOptions != nil {
			return serializer.NewError(serializer.CodeParamErr, "Request options require a URL source", nil)
		}
		if service.TaskOptions != nil || service.DisplayName != "" {
			return serializer.NewError(serializer.CodeParamErr, "Task options require a URL source", nil)
		}
		src, err := fs.NewUriFromString(service.SrcFile)
		if err != nil {
			return serializer.NewError(serializer.CodeParamErr, "Invalid source file uri", err)
		}
		if _, err = m.Get(c, src, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityDownloadFile)); err != nil {
			return serializer.NewError(serializer.CodeParamErr, "Invalid source file", err)
		}
		return nil
	}
	if len(service.SelectedFiles) > 0 && len(service.Src) != 1 {
		return serializer.NewError(serializer.CodeParamErr, "Selected files require one URL source", nil)
	}
	if service.TaskOptions != nil && len(service.Src) != 1 {
		return serializer.NewError(serializer.CodeParamErr, "Task options require one URL source", nil)
	}
	if service.DisplayName != "" && (service.SrcFile != "" || len(service.Src) != 1) {
		return serializer.NewError(serializer.CodeParamErr, "Display name requires one URL source", nil)
	}
	for _, index := range service.SelectedFiles {
		if index < 0 {
			return serializer.NewError(serializer.CodeParamErr, "Invalid selected file", nil)
		}
	}
	for _, source := range service.Src {
		if source == "" {
			continue
		}
		validated, err := workflows.ValidateRequestOptions(source, service.RequestOptions)
		if err != nil {
			return serializer.NewError(serializer.CodeParamErr, "Invalid request options", err)
		}
		service.RequestOptions = validated
		taskOptions, err := workflows.ValidateTaskOptions(source, service.TaskOptions)
		if err != nil {
			return serializer.NewError(serializer.CodeParamErr, "Invalid task options", err)
		}
		service.TaskOptions = taskOptions
	}
	if service.DisplayName != "" {
		service.DisplayName = strings.TrimSpace(service.DisplayName)
		if len(service.DisplayName) > 255 || strings.ContainsAny(service.DisplayName, "\r\n\x00") {
			return serializer.NewError(serializer.CodeParamErr, "Invalid display name", nil)
		}
	}
	return nil
}

// PreviewDownload resolves one URL through the assigned downloader without
// creating a queue task. The adapter is responsible for discarding any
// resolver state before returning.
func (service *DownloadWorkflowService) PreviewDownload(c *gin.Context) (*downloader.TaskStatus, error) {
	if err := service.Validate(c); err != nil {
		return nil, err
	}
	if service.SrcFile != "" || len(service.Src) != 1 || service.Src[0] == "" || len(service.SelectedFiles) > 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "Preview requires one URL without selected files", nil)
	}
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	nodeState := workflows.NodeState{}
	node, err := workflows.AllocateNode(c, dep, &nodeState, types.NodeCapabilityRemoteDownload)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to allocate downloader", err)
	}
	if err := request.ValidateExternalURL(c, service.Src[0], workflows.BuildSSRFOptions(c, dep, node.Settings(c))); err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid source URL", err)
	}
	d, err := node.CreateDownloader(c, dep.RequestClient(), dep.SettingProvider())
	if err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to create downloader", err)
	}
	preview, ok := d.(downloader.PreviewDownloader)
	if !ok {
		return nil, serializer.NewError(serializer.CodeParamErr, "Configured downloader does not support preview", nil)
	}
	previewCtx, cancel := remoteDownloadPreviewContext(c, service.Src[0])
	defer cancel()
	status, err := preview.PreviewTask(previewCtx, service.Src[0], user.Edges.Group.Settings.RemoteDownloadOptions, service.RequestOptions, service.TaskOptions)
	if err != nil {
		if errors.Is(previewCtx.Err(), context.DeadlineExceeded) {
			return nil, serializer.NewError(serializer.CodeCreateTaskError, "Magnet metadata resolution timed out. Please retry.", nil)
		}
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to preview download", err)
	}
	return status, nil
}

// remoteDownloadPreviewContext constrains only magnet metadata resolution.
// Other protocols retain their existing request context and limits.
func remoteDownloadPreviewContext(parent context.Context, source string) (context.Context, context.CancelFunc) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), "magnet:?") {
		return context.WithTimeout(parent, magnetPreviewTimeout)
	}
	return parent, func() {}
}

type (
	ArchiveWorkflowService struct {
		Src      []string `json:"src" binding:"required"`
		Dst      string   `json:"dst" binding:"required"`
		Encoding string   `json:"encoding"`
		Password string   `json:"password"`
		FileMask []string `json:"file_mask"`
	}
	CreateArchiveParamCtx struct{}
)

func (service *ArchiveWorkflowService) CreateExtractTask(c *gin.Context) (*TaskResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionArchiveTask)) {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Group not allowed to compress files", nil)
	}

	dst, err := fs.NewUriFromString(service.Dst)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}

	if len(service.Src) == 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "No source files", nil)
	}

	// Validate destination
	if _, err := m.Get(c, dst, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityCreateFile)); err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}

	// Create task
	t, err := workflows.NewExtractArchiveTask(c, service.Src[0], service.Dst, service.Encoding, service.Password, service.FileMask)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to create task", err)
	}

	if err := dep.IoIntenseQueue(c).QueueTask(c, t); err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to queue task", err)
	}

	return BuildTaskResponse(t, nil, hasher), nil
}

// CreateCompressTask Create task to create an archive file
func (service *ArchiveWorkflowService) CreateCompressTask(c *gin.Context) (*TaskResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionArchiveTask)) {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Group not allowed to compress files", nil)
	}

	dst, err := fs.NewUriFromString(service.Dst)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}

	// Create a placeholder file then delete it to validate the destination
	session, err := m.PrepareUpload(c, &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:             dst,
			Size:            0,
			UploadSessionID: uuid.Must(uuid.NewV4()).String(),
			ExpireAt:        time.Now().Add(time.Second * 3600),
		},
	})
	if err != nil {
		return nil, err
	}
	m.OnUploadFailed(c, session)

	// Create task
	t, err := workflows.NewCreateArchiveTask(c, service.Src, service.Dst)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to create task", err)
	}

	if err := dep.IoIntenseQueue(c).QueueTask(c, t); err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to queue task", err)
	}

	return BuildTaskResponse(t, nil, hasher), nil
}

type (
	ImportWorkflowService struct {
		Src              string `json:"src" binding:"required"`
		Dst              string `json:"dst" binding:"required"`
		ExtractMediaMeta bool   `json:"extract_media_meta"`
		UserID           string `json:"user_id" binding:"required"`
		Recursive        bool   `json:"recursive"`
		PolicyID         int    `json:"policy_id" binding:"required"`
	}
	CreateImportParamCtx struct{}
)

func (service *ImportWorkflowService) CreateImportTask(c *gin.Context) (*TaskResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Only admin can import files", nil)
	}

	userId, err := hasher.Decode(service.UserID, hashid.UserID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid user id", err)
	}

	owner, err := dep.UserClient().GetLoginUserByID(c, userId)
	if err != nil || owner.ID == 0 {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get user", err)
	}

	dst, err := fs.NewUriFromString(fs.NewMyUri(service.UserID))
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}

	// Create task
	t, err := workflows.NewImportTask(c, owner, service.Src, service.Recursive, dst.Join(service.Dst).String(), service.PolicyID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to create task", err)
	}

	if err := dep.IoIntenseQueue(c).QueueTask(c, t); err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to queue task", err)
	}

	return BuildTaskResponse(t, nil, hasher), nil
}

type (
	ListTaskService struct {
		PageSize      int    `form:"page_size" binding:"required,min=10,max=100"`
		Category      string `form:"category" binding:"required,eq=general|eq=downloading|eq=downloaded"`
		NextPageToken string `form:"next_page_token"`
	}
	ListTaskParamCtx struct{}
)

func (service *ListTaskService) ListTasks(c *gin.Context) (*TaskListResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	taskClient := dep.TaskClient()

	args := &inventory.ListTaskArgs{
		PaginationArgs: &inventory.PaginationArgs{
			UseCursorPagination: true,
			PageToken:           service.NextPageToken,
			PageSize:            service.PageSize,
		},
		Types:  []string{queue.CreateArchiveTaskType, queue.ExtractArchiveTaskType, queue.RelocateTaskType, queue.ImportTaskType},
		UserID: user.ID,
	}

	if service.Category != "general" {
		args.Types = []string{queue.RemoteDownloadTaskType}
		if service.Category == "downloading" {
			args.PageSize = intsets.MaxInt
			args.Status = []task.Status{task.StatusSuspending, task.StatusProcessing, task.StatusQueued}
		} else if service.Category == "downloaded" {
			args.Status = []task.Status{task.StatusCanceled, task.StatusError, task.StatusCompleted}
		}
	}

	// Get tasks
	res, err := taskClient.List(c, args)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to query tasks", err)
	}

	tasks := make([]queue.Task, 0, len(res.Tasks))
	nodeMap := make(map[int]*ent.Node)
	for _, t := range res.Tasks {
		task, err := queue.NewTaskFromModel(t)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to parse task", err)
		}

		summary := task.Summarize(hasher)
		if summary != nil && summary.NodeID > 0 {
			if _, ok := nodeMap[summary.NodeID]; !ok {
				nodeMap[summary.NodeID] = nil
			}
		}
		tasks = append(tasks, task)
	}

	// Get nodes
	nodes, err := dep.NodeClient().ListActiveNodes(c, lo.Keys(nodeMap))
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to query nodes", err)
	}
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	// Build response
	return BuildTaskListResponse(tasks, res, nodeMap, hasher), nil
}

func TaskPhaseProgress(c *gin.Context, taskID int) (queue.Progresses, error) {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)
	r := dep.TaskRegistry()
	t, found := r.Get(taskID)
	if !found || (t.Owner().ID != u.ID && !u.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin))) {
		return queue.Progresses{}, nil
	}

	return t.Progress(c), nil
}

// StreamWorkflowEvents pushes an owner's task changes as they are persisted by
// the queue. The payload deliberately contains no task data; clients reload
// through the existing owner-scoped list endpoint after each signal.
func StreamWorkflowEvents(c *gin.Context) error {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	updates, unsubscribe := dep.TaskRegistry().Subscribe(user.ID)
	defer unsubscribe()

	WriteEventSourceHeader(c)
	WriteEventSource(c, "subscribed", nil)
	heartbeat := time.NewTicker(workflowEventHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return nil
		case <-c.Writer.CloseNotify():
			return nil
		case <-updates:
			WriteEventSource(c, "task", nil)
		case <-heartbeat.C:
			WriteEventSourceComment(c, "keepalive")
		}
	}
}

func CancelDownloadTask(c *gin.Context, taskID int) error {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)
	r := dep.TaskRegistry()
	t, found := r.Get(taskID)
	if !found || t.Owner().ID != u.ID {
		return serializer.NewError(serializer.CodeNotFound, "Task not found", nil)
	}
	if t.Status() == task.StatusQueued {
		if err := dep.RemoteDownloadQueue(c).CancelTask(c, t); err != nil {
			return serializer.NewError(serializer.CodeParamErr, "Task is no longer waiting", err)
		}
		return nil
	}

	if downloadTask, ok := t.(*workflows.RemoteDownloadTask); ok {
		if err := downloadTask.CancelDownload(c); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to cancel download task", err)
		}
		if err := dep.RemoteDownloadQueue(c).CancelTask(c, t); err != nil {
			return serializer.NewError(serializer.CodeParamErr, "Task is no longer cancellable", err)
		}
	}

	return nil
}

type (
	BatchDownloadTaskService struct {
		IDs []string `json:"ids" binding:"required,max=100"`
	}
	BatchDownloadTaskParamCtx struct{}
)

// RetryDownloadTask creates a fresh workflow from a terminal task. It never
// mutates the old record, so task history remains auditable and a missing
// Gopeed task handle cannot be resurrected.
func RetryDownloadTask(c *gin.Context, taskID int) (*TaskResponse, error) {
	downloadTask, err := getOwnedTerminalDownloadTask(c, taskID)
	if err != nil {
		return nil, err
	}
	state, err := remoteDownloadTaskState(downloadTask)
	if err != nil {
		return nil, err
	}
	service := &DownloadWorkflowService{
		Src:            []string{state.SrcUri},
		SrcFile:        state.SrcFileUri,
		Dst:            state.Dst,
		RequestOptions: state.RequestOptions,
		TaskOptions:    state.TaskOptions,
		DisplayName:    state.PresentationName,
	}
	if state.SrcFileUri != "" {
		service.Src = nil
	}
	if err := service.Validate(c); err != nil {
		return nil, err
	}
	newTask, err := workflows.NewRemoteDownloadTaskWithConfig(c, state.SrcUri, state.SrcFileUri, state.Dst, service.RequestOptions, service.TaskOptions, state.SelectedFiles, service.DisplayName)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeInternalSetting, "Failed to create retry task", err)
	}
	dep := dependency.FromContext(c)
	if err := dep.RemoteDownloadQueue(c).QueueTask(c, newTask); err != nil {
		return nil, serializer.NewError(serializer.CodeInternalSetting, "Failed to queue retry task", err)
	}
	return BuildTaskResponse(newTask, nil, dep.HashIDEncoder()), nil
}

func (service *BatchDownloadTaskService) Retry(c *gin.Context) ([]*TaskResponse, error) {
	ids, err := decodeDownloadTaskIDs(c, service.IDs)
	if err != nil {
		return nil, err
	}
	responses := make([]*TaskResponse, 0, len(ids))
	for _, id := range ids {
		response, err := RetryDownloadTask(c, id)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func (service *BatchDownloadTaskService) Delete(c *gin.Context) error {
	ids, err := decodeDownloadTaskIDs(c, service.IDs)
	if err != nil {
		return err
	}
	tasks := make([]*workflows.RemoteDownloadTask, 0, len(ids))
	for _, id := range ids {
		downloadTask, err := getOwnedTerminalDownloadTask(c, id)
		if err != nil {
			return err
		}
		tasks = append(tasks, downloadTask)
	}
	for _, downloadTask := range tasks {
		if err := downloadTask.CancelDownload(c); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to clean up download task", err)
		}
	}
	dep := dependency.FromContext(c)
	if err := dep.TaskClient().DeleteByIDs(c, ids...); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete download tasks", err)
	}
	for _, id := range ids {
		dep.TaskRegistry().Delete(id)
	}
	return nil
}

func decodeDownloadTaskIDs(c *gin.Context, encodedIDs []string) ([]int, error) {
	dep := dependency.FromContext(c)
	seen := make(map[int]struct{}, len(encodedIDs))
	ids := make([]int, 0, len(encodedIDs))
	for _, encodedID := range encodedIDs {
		id, err := dep.HashIDEncoder().Decode(encodedID, hashid.TaskID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid task ID", fmt.Errorf("decode task ID: %w", err))
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "No task IDs", nil)
	}
	return ids, nil
}

func getOwnedTerminalDownloadTask(c *gin.Context, taskID int) (*workflows.RemoteDownloadTask, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	registry := dep.TaskRegistry()
	t, found := registry.Get(taskID)
	if !found {
		loadCtx := context.WithValue(c, inventory.LoadTaskUser{}, true)
		model, err := dep.TaskClient().GetTaskByID(loadCtx, taskID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeNotFound, "Task not found", nil)
		}
		t, err = queue.NewTaskFromModel(model)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to parse task", err)
		}
	}
	if t.Type() != queue.RemoteDownloadTaskType || t.Owner() == nil || t.Owner().ID != user.ID {
		return nil, serializer.NewError(serializer.CodeNotFound, "Task not found", nil)
	}
	if !isTerminalDownloadStatus(t.Status()) {
		return nil, serializer.NewError(serializer.CodeParamErr, "Task is not terminal", nil)
	}
	downloadTask, ok := t.(*workflows.RemoteDownloadTask)
	if !ok {
		return nil, serializer.NewError(serializer.CodeDBError, "Invalid download task", nil)
	}
	return downloadTask, nil
}

func remoteDownloadTaskState(downloadTask *workflows.RemoteDownloadTask) (*workflows.RemoteDownloadTaskState, error) {
	state := &workflows.RemoteDownloadTaskState{}
	if err := json.Unmarshal([]byte(downloadTask.State()), state); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to parse download task", err)
	}
	return state, nil
}

func isTerminalDownloadStatus(status task.Status) bool {
	return status == task.StatusCanceled || status == task.StatusCompleted || status == task.StatusError
}

type (
	SetDownloadFilesService struct {
		Files []*downloader.SetFileToDownloadArgs `json:"files" binding:"required"`
	}
	SetDownloadFilesParamCtx struct{}
)

func (service *SetDownloadFilesService) SetDownloadFiles(c *gin.Context, taskID int) error {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)
	r := dep.TaskRegistry()

	t, found := r.Get(taskID)
	if !found || t.Owner().ID != u.ID {
		return serializer.NewError(serializer.CodeNotFound, "Task not found", nil)
	}

	status := t.Status()
	summary := t.Summarize(dep.HashIDEncoder())
	// Task must be in processing state
	if status != task.StatusSuspending && status != task.StatusProcessing {
		return serializer.NewError(serializer.CodeNotFound, "Task not in processing state", nil)
	}

	// Task must in monitoring loop
	if summary.Phase != workflows.RemoteDownloadTaskPhaseMonitor {
		return serializer.NewError(serializer.CodeNotFound, "Task not in monitoring loop", nil)
	}

	if downloadTask, ok := t.(*workflows.RemoteDownloadTask); ok {
		if err := downloadTask.SetDownloadTarget(c, service.Files...); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to set download files", err)
		}
	}

	return nil
}

type (
	RebuildFTSIndexWorkflowService struct {
		FilteredStoragePolicy []int `json:"filtered_storage_policy"`
	}
	CreateRebuildFTSIndexParamCtx struct{}
)

func (service *RebuildFTSIndexWorkflowService) CreateRebuildFTSIndexTask(c *gin.Context) (*TaskResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Only admin can import files", nil)
	}

	// Create task
	t, err := workflows.NewRebuildIndexTask(c, user, service.FilteredStoragePolicy)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to create task", err)
	}

	if err := dep.MediaMetaQueue(c).QueueTask(c, t); err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to queue task", err)
	}

	return BuildTaskResponse(t, nil, hasher), nil
}
