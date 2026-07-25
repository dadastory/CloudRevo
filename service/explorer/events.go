package explorer

import (
	"context"
	"time"

	"github.com/dadastory/CloudRevo/application/constants"
	"github.com/dadastory/CloudRevo/application/dependency"
	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/pkg/auth/requestinfo"
	"github.com/dadastory/CloudRevo/pkg/filemanager/fs"
	"github.com/dadastory/CloudRevo/pkg/filemanager/fs/dbfs"
	"github.com/dadastory/CloudRevo/pkg/filemanager/manager"
	"github.com/dadastory/CloudRevo/pkg/logging"
	"github.com/dadastory/CloudRevo/pkg/serializer"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid"
)

type (
	ExplorerEventService struct {
		Uri string `form:"uri" binding:"required"`
	}
	ExplorerEventParamCtx struct{}
)

func (s *ExplorerEventService) HandleExplorerEventsPush(c *gin.Context) error {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	m := manager.NewFileManager(dep, user)
	l := logging.FromContext(c)
	defer m.Recycle()

	uri, err := fs.NewUriFromString(s.Uri)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Unknown uri", err)
	}

	// Make sure target is a valid folder that the user can listen to
	parent, listRes, err := m.List(c, uri, &manager.ListArgs{
		Page:     0,
		PageSize: 1,
	})
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Requested uri not available", err)
	}

	// Reject event subscriptions on single-file views (e.g. single-file shares).
	// The listed parent is the underlying owner-side folder containing the file,
	// while the subscriber is only authorized to observe the shared file itself.
	// Subscribing to that folder topic would leak events about unshared siblings.
	if listRes != nil && listRes.SingleFileView {
		return serializer.NewError(serializer.CodeNoPermissionErr, "Event subscriptions are not supported on this view", nil)
	}
	if uri.FileSystem() == constants.FileSystemShare &&
		!parent.Capabilities().Enabled(int(dbfs.NavigatorCapabilityVersionControl)) {
		return serializer.NewError(serializer.CodeNoPermissionErr, "Update permission is required to view shared activity", nil)
	}

	requestInfo := requestinfo.RequestInfoFromContext(c)
	if requestInfo.ClientID == "" {
		return serializer.NewError(serializer.CodeParamErr, "Client ID is required", nil)
	}

	// Client ID must be a valid UUID
	if _, err := uuid.FromString(requestInfo.ClientID); err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid client ID", err)
	}

	// Subscribe
	eventHub := dep.EventHub()
	rx, resumed, err := eventHub.Subscribe(c, parent.ID(), requestInfo.ClientID)
	if err != nil {
		return serializer.NewError(serializer.CodeInternalSetting, "Failed to subscribe to events", err)
	}

	// SSE Headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	keepAliveTicker := time.NewTicker(30 * time.Second)
	defer keepAliveTicker.Stop()

	if resumed {
		c.SSEvent("resumed", nil)
		c.Writer.Flush()
	} else {
		c.SSEvent("subscribed", nil)
		c.Writer.Flush()
	}

	for {
		select {
		// TODO: close connection after access token expired
		case <-c.Request.Context().Done():
			// Server shutdown or request cancelled
			eventHub.Unsubscribe(c, parent.ID(), requestInfo.ClientID)
			l.Debug("Request context done, unsubscribed from event hub")
			return nil
		case <-c.Writer.CloseNotify():
			eventHub.Unsubscribe(c, parent.ID(), requestInfo.ClientID)
			l.Debug("Unsubscribed from event hub")
			return nil
		case evt, ok := <-rx:
			if !ok {
				// Channel closed, EventHub is shutting down
				l.Debug("Event hub closed, disconnecting client")
				return nil
			}
			// A share's ACL can change while an SSE request is open. Recreate the
			// file manager for each event so no navigator state, link rule, or
			// descendant rule from the original subscription is reused. A deleted
			// or now-denied object terminates the stream rather than leaking even
			// its name through an event payload.
			if uri.FileSystem() == constants.FileSystemShare && !canReceiveSharedEvent(c, dep, user, uri, evt.From) {
				eventHub.Unsubscribe(c, parent.ID(), requestInfo.ClientID)
				l.Debug("Shared event permission changed, unsubscribed from event hub")
				return nil
			}
			c.SSEvent("event", evt)
			l.Debug("Event sent: %+v", evt)
			c.Writer.Flush()
			if c.Errors.Last() != nil {
				l.Error("Error occurred: %+v", c.Errors.Last().Error())
				eventHub.Unsubscribe(c, parent.ID(), requestInfo.ClientID)
				l.Debug("Unsubscribed from event hub")
				return nil
			}
		case <-keepAliveTicker.C:
			if uri.FileSystem() == constants.FileSystemShare && !canReceiveSharedEvent(c, dep, user, uri, "/") {
				eventHub.Unsubscribe(c, parent.ID(), requestInfo.ClientID)
				l.Debug("Shared activity permission changed, unsubscribed from event hub")
				return nil
			}
			c.SSEvent("keep-alive", nil)
			c.Writer.Flush()
		}
	}
}

func canReceiveSharedEvent(c *gin.Context, dep dependency.Dep, user *ent.User, root *fs.URI, relativePath string) bool {
	// Reload the recipient and its group relationship as well as the share
	// itself. Otherwise an in-memory group change could keep an old grant alive
	// for the lifetime of this long-running HTTP request.
	freshUser, err := dep.UserClient().GetByID(context.WithValue(c, inventory.LoadUserGroup{}, true), user.ID)
	if err != nil {
		return false
	}
	m := manager.NewFileManager(dep, freshUser)
	defer m.Recycle()
	_, err = m.Get(c, root.JoinRaw(relativePath),
		dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityVersionControl))
	return err == nil
}
