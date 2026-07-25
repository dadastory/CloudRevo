package workflows

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/dadastory/CloudRevo/application/dependency"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/cluster"
	"github.com/dadastory/CloudRevo/pkg/queue"
	"github.com/dadastory/CloudRevo/pkg/util"
)

const (
	TaskTempPath                 = "fm_workflows"
	slaveProgressRefreshInterval = 5 * time.Second
)

type NodeState struct {
	NodeID int `json:"node_id"`

	progress queue.Progresses
}

// allocateNode allocates a node for the task.
func allocateNode(ctx context.Context, dep dependency.Dep, state *NodeState, capability types.NodeCapability) (cluster.Node, error) {
	np, err := dep.NodePool(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get node pool: %w", err)
	}

	node, err := np.Get(ctx, capability, state.NodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	state.NodeID = node.ID()
	return node, nil
}

// AllocateNode exposes the standard workflow node allocator to request-time
// preflight checks without duplicating policy selection logic.
func AllocateNode(ctx context.Context, dep dependency.Dep, state *NodeState, capability types.NodeCapability) (cluster.Node, error) {
	return allocateNode(ctx, dep, state, capability)
}

// prepareSlaveTaskCtx prepares the context for the slave task.
func prepareSlaveTaskCtx(ctx context.Context, props *types.SlaveTaskProps) context.Context {
	ctx = context.WithValue(ctx, cluster.SlaveNodeIDCtx{}, strconv.Itoa(props.NodeID))
	ctx = context.WithValue(ctx, cluster.MasterSiteUrlCtx{}, props.MasterSiteURl)
	ctx = context.WithValue(ctx, cluster.MasterSiteVersionCtx{}, props.MasterSiteVersion)
	ctx = context.WithValue(ctx, cluster.MasterSiteIDCtx{}, props.MasterSiteID)
	return ctx
}

func prepareTempFolder(ctx context.Context, dep dependency.Dep, t queue.Task) (string, error) {
	settings := dep.SettingProvider()
	tempPath := util.DataPath(path.Join(settings.TempPath(ctx), TaskTempPath, strconv.Itoa(t.ID())))
	if err := util.CreatNestedFolder(tempPath); err != nil {
		return "", fmt.Errorf("failed to create temp folder: %w", err)
	}

	dep.Logger().Info("Temp folder created: %s", tempPath)
	return tempPath, nil
}
