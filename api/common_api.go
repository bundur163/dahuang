package api

import (
	"msg-service/common"
	"msg-service/model"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func RegisterCommonAPI(r *gin.Engine, db *gorm.DB, zapLog *zap.Logger) {
	g := r.Group("/api/v1/common")

	g.GET("/platform", func(c *gin.Context) {
		pat := c.Query("patname")
		cfg, err := common.GetPlatformConfig(pat, db, "")
		if err != nil {
			zapLog.Error("获取平台配置失败", zap.String("patname", pat), zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}
		c.JSON(http.StatusOK, cfg)
	})

	g.GET("/token", func(c *gin.Context) {
		pat := c.Query("patname")
		tk, err := common.GetAccessToken(pat, db)
		if err != nil {
			zapLog.Error("获取Token失败", zap.String("patname", pat), zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"access_token": tk})
	})

	// 主服务公共配置（APIServerURL / AppID / Secret 默认值）
	g.GET("/settings", func(c *gin.Context) {
		var cfg model.MainConfig
		if err := db.First(&cfg).Error; err != nil {
			db.Create(&cfg) // 首次自动创建
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": cfg})
	})

	g.POST("/settings", func(c *gin.Context) {
		var req model.MainConfig
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效的请求数据"})
			return
		}
		var cfg model.MainConfig
		if err := db.First(&cfg).Error; err != nil {
			db.Create(&req)
		} else {
			db.Model(&cfg).Updates(map[string]interface{}{
				"api_server_url": req.APIServerURL,
				"app_id":         req.AppID,
				"secret":         req.Secret,
			})
		}
		zapLog.Info("更新主服务配置")
		c.JSON(http.StatusOK, gin.H{"code": 0})
	})
}
