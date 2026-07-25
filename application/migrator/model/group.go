package model

import (
	"github.com/jinzhu/gorm"
)

// Group 用户组模型
type Group struct {
	gorm.Model
	Name          string
	Policies      string
	MaxStorage    uint64
	ShareEnabled  bool
	WebDAVEnabled bool
	SpeedLimit    int
	Options       string `json:"-" gorm:"size:4294967295"`

	// 数据库忽略字段
	PolicyList        []uint      `gorm:"-"`
	OptionsSerialized GroupOption `gorm:"-"`
}

// GroupOption 用户组其他配置
type GroupOption struct {
	ArchiveDownload         bool                   `json:"archive_download,omitempty"` // 打包下载
	ArchiveTask             bool                   `json:"archive_task,omitempty"`     // 在线压缩
	CompressSize            uint64                 `json:"compress_size,omitempty"`    // 可压缩大小
	DecompressSize          uint64                 `json:"decompress_size,omitempty"`
	OneTimeDownload         bool                   `json:"one_time_download,omitempty"`
	ShareDownload           bool                   `json:"share_download,omitempty"`
	RemoteDownload          bool                   `json:"remote_download,omitempty"`
	RemoteDownloadOptions   map[string]interface{} `json:"remote_download_options,omitempty"`
	SourceBatchSize         int                    `json:"source_batch,omitempty"`
	RedirectedSource        bool                   `json:"redirected_source,omitempty"`
	RemoteDownloadBatchSize int                    `json:"remote_download_batch,omitempty"`
	AdvanceDelete           bool                   `json:"advance_delete,omitempty"`
	WebDAVProxy             bool                   `json:"webdav_proxy,omitempty"`
}
