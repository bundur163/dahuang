package main

import (
	"log"
	"msg-service/api"
	"msg-service/common"
	"msg-service/model"
	"msg-service/service"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	// 加载配置
	if err := LoadConfig("config.yaml"); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化日志
	logger := common.InitLogger(GlobalConfig.Server.LogPath)
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	// 初始化数据库
	db, err := gorm.Open(sqlite.Open("service.db"), &gorm.Config{})
	if err != nil {
		logger.Fatal("数据库连接失败", zap.Error(err))
	}
	db.AutoMigrate(&model.SubService{}, &model.PlatformConfig{}, &model.AccessToken{})

	// 初始化公共工具（传入配置）
	common.InitHTTPClient(GlobalConfig.HTTP.Timeout, GlobalConfig.HTTP.RetryCount)
	common.InitRemoteConfig(GlobalConfig.RemoteAPI.BaseURL, GlobalConfig.RemoteAPI.ConfigAPI)

	// 设置 Gin
	r := gin.Default()
	api.RegisterSubAPI(r, db, logger) // logger 是 *zap.Logger 类型
	api.RegisterCommonAPI(r, db, logger)

	// 静态文件
	r.Static("/admin", "./web")

	// 优雅退出
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		logger.Info("收到退出信号，停止所有子服务")
		service.StopAll()
		os.Exit(0)
	}()

	logger.Info("主服务启动", zap.String("port", GlobalConfig.Server.WebPort))
	if err := r.Run("0.0.0.0:" + GlobalConfig.Server.WebPort); err != nil {
		logger.Fatal("启动失败", zap.Error(err))
	}
}
