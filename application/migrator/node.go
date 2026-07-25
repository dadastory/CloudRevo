package migrator

import (
	"context"
	"fmt"

	"github.com/dadastory/CloudRevo/application/migrator/model"
	"github.com/dadastory/CloudRevo/ent/node"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/boolset"
)

func (m *Migrator) migrateNode() error {
	m.l.Info("Migrating nodes...")

	var nodes []model.Node
	if err := model.DB.Find(&nodes).Error; err != nil {
		return fmt.Errorf("failed to list v3 nodes: %w", err)
	}

	for _, n := range nodes {
		nodeType := node.TypeSlave
		nodeStatus := node.StatusSuspended
		if n.Type == model.MasterNodeType {
			nodeType = node.TypeMaster
		}
		if n.Status == model.NodeActive {
			nodeStatus = node.StatusActive
		}

		cap := &boolset.BooleanSet{}
		settings := &types.NodeSetting{Provider: types.DownloaderProviderGopeed}

		if n.Type == model.MasterNodeType {
			boolset.Sets(map[types.NodeCapability]bool{
				types.NodeCapabilityExtractArchive: true,
				types.NodeCapabilityCreateArchive:  true,
			}, cap)
		}

		stm := m.v4client.Node.Create().
			SetRawID(int(n.ID)).
			SetCreatedAt(formatTime(n.CreatedAt)).
			SetUpdatedAt(formatTime(n.UpdatedAt)).
			SetName(n.Name).
			SetType(nodeType).
			SetStatus(nodeStatus).
			SetServer(n.Server).
			SetSlaveKey(n.SlaveKey).
			SetCapabilities(cap).
			SetSettings(settings).
			SetWeight(n.Rank)

		if err := stm.Exec(context.Background()); err != nil {
			return fmt.Errorf("failed to create node %q: %w", n.Name, err)
		}

	}

	return nil
}
