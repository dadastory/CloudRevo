package controllers

import (
	"github.com/dadastory/CloudRevo/pkg/hashid"
	"github.com/dadastory/CloudRevo/pkg/queue"
	"github.com/dadastory/CloudRevo/pkg/serializer"
	"github.com/dadastory/CloudRevo/service/explorer"
	"github.com/gin-gonic/gin"
)

func ListTasks(c *gin.Context) {
	service := ParametersFromContext[*explorer.ListTaskService](c, explorer.ListTaskParamCtx{})
	resp, err := service.ListTasks(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		c.Abort()
		return
	}

	if resp != nil {
		c.JSON(200, serializer.Response{
			Data: resp,
		})
	}
}

func WorkflowEvents(c *gin.Context) {
	if err := explorer.StreamWorkflowEvents(c); err != nil {
		c.JSON(200, serializer.Err(c, err))
		c.Abort()
	}
}

func GetTaskPhaseProgress(c *gin.Context) {
	taskId := hashid.FromContext(c)
	resp, err := explorer.TaskPhaseProgress(c, taskId)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		c.Abort()
		return
	}

	if resp != nil {
		c.JSON(200, serializer.Response{
			Data: resp,
		})
	} else {
		c.JSON(200, serializer.Response{Data: queue.Progresses{}})
	}
}

func SetDownloadTaskTarget(c *gin.Context) {
	taskId := hashid.FromContext(c)
	service := ParametersFromContext[*explorer.SetDownloadFilesService](c, explorer.SetDownloadFilesParamCtx{})
	err := service.SetDownloadFiles(c, taskId)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		c.Abort()
		return
	}

	c.JSON(200, serializer.Response{})
}

func CancelDownloadTask(c *gin.Context) {
	taskId := hashid.FromContext(c)
	err := explorer.CancelDownloadTask(c, taskId)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		c.Abort()
		return
	}

	c.JSON(200, serializer.Response{})
}

func RetryDownloadTask(c *gin.Context) {
	taskID := hashid.FromContext(c)
	resp, err := explorer.RetryDownloadTask(c, taskID)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		c.Abort()
		return
	}
	c.JSON(200, serializer.Response{Data: resp})
}

func PreviewRemoteDownload(c *gin.Context) {
	service := ParametersFromContext[*explorer.DownloadWorkflowService](c, explorer.PreviewDownloadParamCtx{})
	resp, err := service.PreviewDownload(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		c.Abort()
		return
	}
	c.JSON(200, serializer.Response{Data: resp})
}

func RetryDownloadTasks(c *gin.Context) {
	service := ParametersFromContext[*explorer.BatchDownloadTaskService](c, explorer.BatchDownloadTaskParamCtx{})
	resp, err := service.Retry(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		c.Abort()
		return
	}
	c.JSON(200, serializer.Response{Data: resp})
}

func DeleteDownloadTasks(c *gin.Context) {
	service := ParametersFromContext[*explorer.BatchDownloadTaskService](c, explorer.BatchDownloadTaskParamCtx{})
	if err := service.Delete(c); err != nil {
		c.JSON(200, serializer.Err(c, err))
		c.Abort()
		return
	}
	c.JSON(200, serializer.Response{})
}
