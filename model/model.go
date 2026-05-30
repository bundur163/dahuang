package model

import "gorm.io/gorm"

type SubService struct {
	gorm.Model
	ServiceName  string `gorm:"unique;not null" json:"ServiceName"` // 前端使用 ServiceName
	Enable       bool   `json:"Enable"`
	PollInterval int    `json:"PollInterval"`
	QueueAPI     string `json:"QueueAPI"`
	FeedbackAPI  string `json:"FeedbackAPI"`
	RealPID      int    `json:"RealPID" gorm:"-"`
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
