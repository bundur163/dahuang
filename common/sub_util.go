package common

import (
	"encoding/json"
	"fmt"
	"msg-service/model"
	"time"

	"gorm.io/gorm"
)

var (
	httpTimeout     int
	retryCount      int
	remoteBaseURL   string
	remoteConfigAPI string
	db              *gorm.DB
)

func InitHTTPClient(timeout, retry int) {
	httpTimeout = timeout
	retryCount = retry
}

func InitRemoteConfig(baseURL, configAPI string) {
	remoteBaseURL = baseURL
	remoteConfigAPI = configAPI
}

func SetDB(database *gorm.DB) {
	db = database
}

// GetPlatformConfig 优先读缓存，无则从远端拉取
func GetPlatformConfig(patname string, db *gorm.DB, base string) (*model.PlatformConfig, error) {
	var cfg model.PlatformConfig
	result := db.Where("patname = ?", patname).First(&cfg)
	if result.Error == nil {
		return &cfg, nil
	}

	// 远端拉取
	url := fmt.Sprintf("%s%s?patname=%s", remoteBaseURL, remoteConfigAPI, patname)
	body := HttpGet(url)
	if body == nil {
		return nil, fmt.Errorf("远端获取平台配置失败: %s", url)
	}

	var remoteCfg model.PlatformConfig
	if err := json.Unmarshal(body, &remoteCfg); err != nil {
		return nil, err
	}
	// 存储或更新
	db.Where("patname = ?", patname).Assign(remoteCfg).FirstOrCreate(&cfg)
	return &remoteCfg, nil
}

// GetAccessToken 缓存 + 自动刷新
func GetAccessToken(patname string, db *gorm.DB) (string, error) {
	var token model.AccessToken
	now := time.Now().Unix()

	result := db.Where("patname = ?", patname).First(&token)
	if result.Error == nil && token.ExpireAt > now+60 {
		return token.Token, nil
	}

	// 需要刷新 token
	platformCfg, err := GetPlatformConfig(patname, db, "")
	if err != nil {
		return "", err
	}

	// 根据 platform 类型调用对应的 token 获取接口（示例实现钉钉）
	var newToken string
	var expiresIn int64
	switch platformCfg.Platform {
	case "dingtalk":
		newToken, expiresIn, err = fetchDingTalkToken(platformCfg.AppKey, platformCfg.AppSecret)
	case "workwechat":
		newToken, expiresIn, err = fetchWorkWechatToken(platformCfg.CorpID, platformCfg.AppSecret)
	case "feishu":
		newToken, expiresIn, err = fetchFeishuToken(platformCfg.AppKey, platformCfg.AppSecret)
	default:
		return "", fmt.Errorf("不支持的平台: %s", platformCfg.Platform)
	}
	if err != nil {
		return "", err
	}

	// 更新数据库
	token = model.AccessToken{
		PatName:  patname,
		Token:    newToken,
		ExpireAt: now + expiresIn,
	}
	db.Where("patname = ?", patname).Assign(token).FirstOrCreate(&token)
	return newToken, nil
}

// 示例：钉钉 token 获取（需替换真实URL）
func fetchDingTalkToken(appKey, appSecret string) (string, int64, error) {
	// 实际调用 https://oapi.dingtalk.com/gettoken?appkey=xxx&appsecret=xxx
	// 这里返回 mock
	return "mock_dingtalk_token", 7200, nil
}

func fetchWorkWechatToken(corpID, secret string) (string, int64, error) {
	return "mock_work_token", 7200, nil
}

func fetchFeishuToken(appKey, appSecret string) (string, int64, error) {
	return "mock_feishu_token", 7200, nil
}
