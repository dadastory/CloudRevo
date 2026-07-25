package model

import (
	"github.com/jinzhu/gorm"
)

// Node 从机节点信息模型
type Node struct {
	gorm.Model
	Status    NodeStatus // 节点状态
	Name      string     // 节点别名
	Type      ModelType  // 节点状态
	Server    string     // 服务器地址
	SlaveKey  string     `gorm:"type:text"` // 主->从 通信密钥
	MasterKey string     `gorm:"type:text"` // 从->主 通信密钥
	Rank      int        // 负载均衡权重
}

type NodeStatus int
type ModelType int

const (
	NodeActive NodeStatus = iota
	NodeSuspend
)

const (
	SlaveNodeType ModelType = iota
	MasterNodeType
)
