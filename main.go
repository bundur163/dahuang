package main

import (
	"log"
	"msg-service/api"
	"msg-service/common"
	"msg-service/model"
	"msg-service/service"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

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
	db.AutoMigrate(&model.SubService{}, &model.PlatformConfig{}, &model.AccessToken{}, &model.MainConfig{})

	// 确保主服务配置存在（单行）
	if err := db.FirstOrCreate(&model.MainConfig{}).Error; err != nil {
		logger.Warn("初始化主服务配置失败", zap.Error(err))
	}

	// 初始化公共工具（传入配置）
	common.InitHTTPClient(GlobalConfig.HTTP.Timeout, GlobalConfig.HTTP.RetryCount)
	common.InitRemoteConfig(GlobalConfig.RemoteAPI.BaseURL, GlobalConfig.RemoteAPI.ConfigAPI)

	// 设置 Gin
	r := gin.Default()
	api.RegisterSubAPI(r, db, logger) // logger 是 *zap.Logger 类型
	api.RegisterCommonAPI(r, db, logger)

	// 静态文件
	r.Static("/admin", "./web")

	// 自动启动 StartMode="auto" 的子服务
	go func() {
		time.Sleep(2 * time.Second) // 等待 HTTP 服务就绪
		var autoList []model.SubService
		db.Where("start_mode = ?", "auto").Find(&autoList)
		for _, svc := range autoList {
			if svc.BinPath == "" {
				logger.Warn("自动启动跳过：未配置服务程序路径", zap.String("service", svc.ServiceName))
				continue
			}
			absBinPath := svc.BinPath
			if !filepath.IsAbs(svc.BinPath) {
				wd, _ := os.Getwd()
				absBinPath = filepath.Join(wd, svc.BinPath)
			}
			if _, err := os.Stat(absBinPath); os.IsNotExist(err) {
				logger.Warn("自动启动跳过：二进制不存在", zap.String("service", svc.ServiceName), zap.String("path", absBinPath))
				continue
			}
			if err := service.StartSubService(svc.ServiceName, absBinPath, filepath.Dir(absBinPath)); err != nil {
				logger.Error("自动启动子服务失败", zap.String("service", svc.ServiceName), zap.Error(err))
				continue
			}
			db.Model(&model.SubService{}).Where("service_name = ?", svc.ServiceName).Update("enable", true)
			logger.Info("自动启动子服务", zap.String("service", svc.ServiceName), zap.String("path", absBinPath))
		}
	}()

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
