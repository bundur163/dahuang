package api

import (
	"bufio"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"msg-service/model"
	"msg-service/service"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// resolvePID 通过 ServiceName 和 BinPath 查找进程 PID
func resolvePID(svc model.SubService) int {
	pid := service.GetPIDByName(svc.ServiceName)
	if pid > 0 {
		return pid
	}
	if svc.BinPath != "" {
		pid = service.GetPIDByName(filepath.Base(svc.BinPath))
	}
	return pid
}

// md5sum 计算字符串的 MD5 十六进制摘要
func md5sum(s string) string {
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}

func RegisterSubAPI(r *gin.Engine, db *gorm.DB, zapLog *zap.Logger) {
	v1 := r.Group("/api/v1")
	{
		sub := v1.Group("/sub")
		{
			sub.GET("/list", func(c *gin.Context) {
				var list []model.SubService
				db.Find(&list)
				for i := range list {
					list[i].RealPID = resolvePID(list[i])
				}
				c.JSON(http.StatusOK, gin.H{"code": 0, "data": list})
			})

			sub.GET("/log", func(c *gin.Context) {
				name := c.Query("service_name")
				if name == "" {
					c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "service_name不能为空"})
					return
				}

				content, err := service.ReadSubLog(name)
				if err != nil {
					content = ""
				}

				childLines := []string{}
				if content != "" {
					for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
						line = strings.TrimSpace(line)
						if line != "" {
							childLines = append(childLines, line)
						}
					}
				}
				if len(childLines) > 120 {
					childLines = childLines[len(childLines)-120:]
				}

				mainLogPath := filepath.Join("/var/log/msg-service", "service.log")
				mainLines := []string{}
				seen := map[string]struct{}{}
				if data, readErr := os.ReadFile(mainLogPath); readErr == nil {
					scanner := bufio.NewScanner(strings.NewReader(string(data)))
					for scanner.Scan() {
						line := strings.TrimSpace(scanner.Text())
						if line == "" {
							continue
						}

						var entry struct {
							Level   string `json:"level"`
							TS      string `json:"ts"`
							Msg     string `json:"msg"`
							Service string `json:"service"`
							Enable  bool   `json:"enable"`
						}
						if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.Service != name {
							continue
						}
						if _, ok := seen[line]; ok {
							continue
						}
						seen[line] = struct{}{}

						when := entry.TS
						if when != "" {
							if ts, err := json.Number(when).Int64(); err == nil {
								when = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
							}
						}
						mainLines = append(mainLines, when+" ["+strings.ToUpper(entry.Level)+"] "+entry.Msg)
					}
				}
				if len(mainLines) > 100 {
					mainLines = mainLines[len(mainLines)-100:]
				}

				var builder strings.Builder
				if len(childLines) > 0 {
					builder.WriteString("[子服务日志]\n")
					builder.WriteString(strings.Join(childLines, "\n"))
				}
				if len(mainLines) > 0 {
					if builder.Len() > 0 {
						builder.WriteString("\n\n")
					}
					builder.WriteString("[主服务操作日志]\n")
					builder.WriteString(strings.Join(mainLines, "\n"))
				}
				if builder.Len() == 0 {
					builder.WriteString("暂无日志")
				}

				c.JSON(http.StatusOK, gin.H{"code": 0, "data": builder.String()})
			})

			sub.GET("/config", func(c *gin.Context) {
				name := c.Query("service_name")
				var svc model.SubService
				db.Where("service_name = ?", name).First(&svc)
				svc.RealPID = resolvePID(svc)

				// 子服务未设置时，从主服务配置继承
				if svc.APIServerURL == "" || svc.AppID == "" || svc.Secret == "" {
					var mainCfg model.MainConfig
					if db.First(&mainCfg).Error == nil {
						if svc.APIServerURL == "" {
							svc.APIServerURL = mainCfg.APIServerURL
						}
						if svc.AppID == "" {
							svc.AppID = mainCfg.AppID
						}
						if svc.Secret == "" {
							svc.Secret = mainCfg.Secret
						}
					}
				}

				// 将 ExtraAttrs 合并到响应 JSON 顶层
				raw, _ := json.Marshal(svc)
				var merged map[string]interface{}
				json.Unmarshal(raw, &merged)
				if svc.ExtraAttrs != "" {
					var extras map[string]interface{}
					if json.Unmarshal([]byte(svc.ExtraAttrs), &extras) == nil {
						for k, v := range extras {
							if _, exists := merged[k]; !exists {
								merged[k] = v
							}
						}
					}
				}
				c.JSON(http.StatusOK, merged)
			})

			sub.POST("/start", func(c *gin.Context) {
				var req struct {
					ServiceName string `json:"service_name"`
					BinPath     string `json:"bin_path"`
				}
				c.ShouldBindJSON(&req)

				// 优先从数据库读取 BinPath
				binPath := req.BinPath
				if binPath == "" {
					var svc model.SubService
					if db.Where("service_name = ?", req.ServiceName).First(&svc).Error == nil {
						binPath = svc.BinPath
					}
				}
				if binPath == "" {
					c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "未配置服务程序路径"})
					return
				}

				// 获取主服务当前工作目录
				workDir, _ := os.Getwd()
				// 将相对路径转换为绝对路径
				absBinPath := binPath
				if !filepath.IsAbs(binPath) {
					absBinPath = filepath.Join(workDir, binPath)
				}
				// 获取二进制所在目录作为子服务工作目录
				binDir := filepath.Dir(absBinPath)

				// 检查文件是否存在且可执行
				if _, err := os.Stat(absBinPath); os.IsNotExist(err) {
					zapLog.Error("子服务二进制不存在", zap.String("path", absBinPath))
					c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "二进制文件不存在: " + absBinPath})
					return
				}

				// 先用 ServiceName 查 PID，再用二进制文件名查
				pid := service.GetPIDByName(req.ServiceName)
				if pid == 0 {
					pid = service.GetPIDByName(filepath.Base(binPath))
				}
				if pid == 0 {
					if err := service.StartSubService(req.ServiceName, absBinPath, binDir); err != nil {
						zapLog.Error("启动子服务失败", zap.String("service", req.ServiceName), zap.String("bin_path", absBinPath), zap.Error(err))
						c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
						return
					}
					zapLog.Info("启动子服务", zap.String("service", req.ServiceName), zap.String("bin_path", absBinPath))
				} else {
					zapLog.Info("子服务已在运行，跳过启动", zap.String("service", req.ServiceName))
				}

				db.Model(&model.SubService{}).
					Where("service_name = ?", req.ServiceName).
					Update("enable", true)

				c.JSON(http.StatusOK, gin.H{"code": 0})
			})

			sub.POST("/stop", func(c *gin.Context) {
				var req struct {
					ServiceName string `json:"service_name"`
				}
				c.ShouldBindJSON(&req)

				service.StopSubService(req.ServiceName)
				zapLog.Info("停止子服务", zap.String("service", req.ServiceName))

				db.Model(&model.SubService{}).
					Where("service_name = ?", req.ServiceName).
					Update("enable", false)

				c.JSON(http.StatusOK, gin.H{"code": 0})
			})

			sub.POST("/stopall", func(c *gin.Context) {
				var list []model.SubService
				db.Find(&list)

				stopped := 0
				for _, svc := range list {
					if resolvePID(svc) > 0 {
						service.StopSubService(svc.ServiceName)
						stopped++
						zapLog.Info("批量停止子服务", zap.String("service", svc.ServiceName))
					}
				}

				zapLog.Info("批量停止完成", zap.Int("stopped", stopped), zap.Int("total", len(list)))
				c.JSON(http.StatusOK, gin.H{"code": 0, "msg": fmt.Sprintf("已停止 %d 个运行中的子服务", stopped)})
			})

			sub.POST("/startall", func(c *gin.Context) {
				var list []model.SubService
				db.Where("enable = ? AND start_mode != ?", true, "disabled").Find(&list)

				started := 0
				workDir, _ := os.Getwd()
				for _, svc := range list {
					if resolvePID(svc) > 0 {
						zapLog.Info("子服务已在运行，跳过", zap.String("service", svc.ServiceName))
						continue
					}

					if svc.BinPath == "" {
						zapLog.Error("子服务未配置程序路径，跳过", zap.String("service", svc.ServiceName))
						continue
					}

					absBinPath := svc.BinPath
					if !filepath.IsAbs(svc.BinPath) {
						absBinPath = filepath.Join(workDir, svc.BinPath)
					}
					binDir := filepath.Dir(absBinPath)

					if _, err := os.Stat(absBinPath); os.IsNotExist(err) {
						zapLog.Error("子服务二进制不存在，跳过", zap.String("service", svc.ServiceName), zap.String("path", absBinPath))
						continue
					}

					if err := service.StartSubService(svc.ServiceName, absBinPath, binDir); err != nil {
						zapLog.Error("启动子服务失败", zap.String("service", svc.ServiceName), zap.Error(err))
						continue
					}

					started++
					zapLog.Info("批量启动子服务", zap.String("service", svc.ServiceName))
				}

				zapLog.Info("批量启动完成", zap.Int("started", started), zap.Int("enabled_total", len(list)))
				c.JSON(http.StatusOK, gin.H{"code": 0, "msg": fmt.Sprintf("已启动 %d 个子服务（共 %d 个已启用）", started, len(list))})
			})

			sub.POST("/remove", func(c *gin.Context) {
				name := c.PostForm("service_name")
				service.StopSubService(name)
				db.Unscoped().Where("service_name = ?", name).Delete(&model.SubService{})
				zapLog.Info("移除子服务", zap.String("service", name))
				c.JSON(http.StatusOK, gin.H{"code": 0})
			})

			sub.POST("/register", func(c *gin.Context) {
				var req struct {
					ServiceName     *string `json:"service_name"`
					NameAlias       *string `json:"ServiceName"`
					DisplayName     *string `json:"DisplayName"`
					BinPath         *string `json:"BinPath"`
					StartMode       *string `json:"StartMode"`
					Enable          *bool   `json:"Enable"`
					PollInterval    *int    `json:"PollInterval"`
					ConfigServerURL *string `json:"ConfigServerURL"`
					APIServerURL    *string `json:"APIServerURL"`
					AppID           *string `json:"AppID"`
					Secret          *string `json:"Secret"`
					QueueAPI        *string `json:"QueueAPI"`
					FeedbackAPI     *string `json:"FeedbackAPI"`
					ExtraAttrs      *string `json:"ExtraAttrs"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效的请求数据"})
					return
				}

				// 确定 ServiceName：优先使用显式传入的，否则由 BinPath 的 MD5 生成
				name := ""
				if req.ServiceName != nil && *req.ServiceName != "" {
					name = *req.ServiceName
				} else if req.NameAlias != nil && *req.NameAlias != "" {
					name = *req.NameAlias
				}
				if name == "" {
					if req.BinPath != nil && *req.BinPath != "" {
						name = md5sum(*req.BinPath)
					}
				}
				if name == "" {
					c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "服务名称不能为空，请提供服务名称或服务程序路径"})
					return
				}

				svc := model.SubService{ServiceName: name}
				if req.DisplayName != nil {
					svc.DisplayName = *req.DisplayName
				}
				if req.BinPath != nil {
					svc.BinPath = *req.BinPath
				}
				if req.StartMode != nil {
					svc.StartMode = *req.StartMode
				}
				if req.Enable != nil {
					svc.Enable = *req.Enable
				}
				if req.PollInterval != nil {
					svc.PollInterval = *req.PollInterval
				}
				if req.ConfigServerURL != nil {
					svc.ConfigServerURL = *req.ConfigServerURL
				}
				if req.APIServerURL != nil {
					svc.APIServerURL = *req.APIServerURL
				}
				if req.AppID != nil {
					svc.AppID = *req.AppID
				}
				if req.Secret != nil {
					svc.Secret = *req.Secret
				}
				if req.QueueAPI != nil {
					svc.QueueAPI = *req.QueueAPI
				}
				if req.FeedbackAPI != nil {
					svc.FeedbackAPI = *req.FeedbackAPI
				}
				if req.ExtraAttrs != nil {
					svc.ExtraAttrs = *req.ExtraAttrs
				}

				var existing model.SubService
				err := db.Unscoped().Where("service_name = ?", name).First(&existing).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					if createErr := db.Create(&svc).Error; createErr != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": createErr.Error()})
						return
					}
				} else if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
					return
				} else {
					if existing.DeletedAt.Valid {
						if restoreErr := db.Unscoped().Model(&existing).Update("deleted_at", nil).Error; restoreErr != nil {
							c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": restoreErr.Error()})
							return
						}
					}
					updateMap := map[string]interface{}{}
					if req.DisplayName != nil {
						updateMap["display_name"] = svc.DisplayName
					}
					if req.BinPath != nil {
						updateMap["bin_path"] = svc.BinPath
					}
					if req.StartMode != nil {
						updateMap["start_mode"] = svc.StartMode
					}
					if req.Enable != nil {
						updateMap["enable"] = svc.Enable
					}
					if req.PollInterval != nil {
						updateMap["poll_interval"] = svc.PollInterval
					}
					if req.ConfigServerURL != nil {
						updateMap["config_server_url"] = svc.ConfigServerURL
					}
					if req.APIServerURL != nil {
						updateMap["api_server_url"] = svc.APIServerURL
					}
					if req.AppID != nil {
						updateMap["app_id"] = svc.AppID
					}
					if req.Secret != nil {
						updateMap["secret"] = svc.Secret
					}
					if req.QueueAPI != nil {
						updateMap["queue_api"] = svc.QueueAPI
					}
					if req.FeedbackAPI != nil {
						updateMap["feedback_api"] = svc.FeedbackAPI
					}
					if req.ExtraAttrs != nil {
						updateMap["extra_attrs"] = svc.ExtraAttrs
					}
					if len(updateMap) > 0 {
						if updateErr := db.Model(&existing).Updates(updateMap).Error; updateErr != nil {
							c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": updateErr.Error()})
							return
						}
					}
				}

				zapLog.Info("注册子服务", zap.String("service", name), zap.Bool("enable", svc.Enable), zap.String("start_mode", svc.StartMode))
				c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"service_name": name}})
			})

			sub.POST("/update", func(c *gin.Context) {
				var svc model.SubService
				if err := c.ShouldBindJSON(&svc); err != nil || svc.ServiceName == "" {
					c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效的服务名称"})
					return
				}

				var existing model.SubService
				err := db.Unscoped().Where("service_name = ?", svc.ServiceName).First(&existing).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					if createErr := db.Create(&svc).Error; createErr != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": createErr.Error()})
						return
					}
				} else if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
					return
				} else {
					if existing.DeletedAt.Valid {
						if restoreErr := db.Unscoped().Model(&existing).Update("deleted_at", nil).Error; restoreErr != nil {
							c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": restoreErr.Error()})
							return
						}
					}
					updateMap := map[string]interface{}{
						"display_name":      svc.DisplayName,
						"bin_path":          svc.BinPath,
						"enable":            svc.Enable,
						"poll_interval":     svc.PollInterval,
						"config_server_url": svc.ConfigServerURL,
						"api_server_url":    svc.APIServerURL,
						"app_id":            svc.AppID,
						"secret":            svc.Secret,
						"queue_api":         svc.QueueAPI,
						"feedback_api":      svc.FeedbackAPI,
						"extra_attrs":       svc.ExtraAttrs,
					}
					if svc.StartMode != "" {
						updateMap["start_mode"] = svc.StartMode
					}
					if updateErr := db.Model(&existing).Updates(updateMap).Error; updateErr != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": updateErr.Error()})
						return
					}
				}

				zapLog.Info("更新子服务配置", zap.String("service", svc.ServiceName), zap.Bool("enable", svc.Enable), zap.String("start_mode", svc.StartMode))
				c.JSON(http.StatusOK, gin.H{"code": 0})
			})
		}
	}
}
