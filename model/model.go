package model

import "gorm.io/gorm"

type SubService struct {
	gorm.Model
	ServiceName     string `gorm:"unique;not null" json:"ServiceName"`     // 内部唯一标识（BinPath的MD5值，或子服务自注册时的名称）
	DisplayName     string `json:"DisplayName"`                             // 显示名称，如 "钉钉消息服务"
	BinPath         string `json:"BinPath"`                                 // 可执行程序路径，如 "subs/dingtalk_msg/dingtalk_msg"
	StartMode       string `gorm:"default:manual" json:"StartMode"`        // 启动模式: "auto"(自动启动) / "manual"(手工启动) / "disabled"(禁用)
	Enable          bool   `json:"Enable"`
	PollInterval    int    `json:"PollInterval"`
	ConfigServerURL string `json:"ConfigServerURL"`                         // 配置服务器URL
	APIServerURL    string `json:"APIServerURL"`                            // API服务器URL（为空则继承主服务配置）
	AppID           string `json:"AppID"`                                   // 调用API的AppID（为空则继承主服务配置）
	Secret          string `json:"Secret"`                                  // 调用API的Secret（为空则继承主服务配置）
	QueueAPI        string `json:"QueueAPI"`                                // 任务队列API路径
	FeedbackAPI     string `json:"FeedbackAPI"`                             // 结果反馈API路径
	ExtraAttrs      string `json:"ExtraAttrs"`                              // 扩展属性，JSON字符串格式（单层键值对）
	RealPID         int    `json:"RealPID" gorm:"-"`
}

// MainConfig 主服务公共配置（单行表，所有子服务的默认值）
type MainConfig struct {
	gorm.Model
	APIServerURL string `json:"APIServerURL"` // API服务器URL
	AppID        string `json:"AppID"`        // 调用API的AppID
	Secret       string `json:"Secret"`       // 调用API的Secret
}

type PlatformConfig struct {
	gorm.Model
	PatName   string `gorm:"unique" json:"patname"`
	Platform  string `json:"platform"`
	CorpID    string `json:"corp_id"`
	AppKey    string `json:"app_key"`
	AppSecret string `json:"app_secret"`
	AgentID   string `json:"agent_id"`
}

type AccessToken struct {
	gorm.Model
	PatName  string `gorm:"unique" json:"patname"`
	Token    string `json:"token"`
	ExpireAt int64  `json:"expire_at"`
}
