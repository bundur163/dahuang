package api

import (
	"msg-service/common"
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
}
