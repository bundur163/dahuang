package api

import (
	"bufio"
	"encoding/json"
	"errors"
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

func RegisterSubAPI(r *gin.Engine, db *gorm.DB, zapLog *zap.Logger) { // 改名避免冲突
	v1 := r.Group("/api/v1")
	{
		sub := v1.Group("/sub")
		{
			sub.GET("/list", func(c *gin.Context) {
				var list []model.SubService
				db.Find(&list)
				for i := range list {
					list[i].RealPID = service.GetPIDByName(list[i].ServiceName)
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
				c.JSON(http.StatusOK, svc)
			})

			sub.POST("/start", func(c *gin.Context) {
				var req struct {
					ServiceName string `json:"service_name"`
					BinPath     string `json:"bin_path"`
				}
				c.ShouldBindJSON(&req)

				// 获取主服务当前工作目录
				workDir, _ := os.Getwd()
				// 将相对路径转换为绝对路径
				absBinPath := req.BinPath
				if !filepath.IsAbs(req.BinPath) {
					absBinPath = filepath.Join(workDir, req.BinPath)
				}
				// 获取二进制所在目录作为子服务工作目录
				binDir := filepath.Dir(absBinPath)

				// 检查文件是否存在且可执行
				if _, err := os.Stat(absBinPath); os.IsNotExist(err) {
					zapLog.Error("子服务二进制不存在", zap.String("path", absBinPath))
					c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "二进制文件不存在: " + absBinPath})
					return
				}

				if service.GetPIDByName(req.ServiceName) == 0 {
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

			sub.POST("/remove", func(c *gin.Context) {
				name := c.PostForm("service_name")
				service.StopSubService(name)
				db.Unscoped().Where("service_name = ?", name).Delete(&model.SubService{})
				zapLog.Info("移除子服务", zap.String("service", name))
				c.JSON(http.StatusOK, gin.H{"code": 0})
			})

			sub.POST("/register", func(c *gin.Context) {
				var req struct {
					ServiceName  *string `json:"service_name"`
					NameAlias    *string `json:"ServiceName"`
					Enable       *bool   `json:"Enable"`
					PollInterval *int    `json:"PollInterval"`
					QueueAPI     *string `json:"QueueAPI"`
					FeedbackAPI  *string `json:"FeedbackAPI"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效的请求数据"})
					return
				}
				name := ""
				if req.ServiceName != nil && *req.ServiceName != "" {
					name = *req.ServiceName
				} else if req.NameAlias != nil && *req.NameAlias != "" {
					name = *req.NameAlias
				}
				if name == "" {
					c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "服务名称不能为空"})
					return
				}

				svc := model.SubService{ServiceName: name}
				if req.Enable != nil {
					svc.Enable = *req.Enable
				}
				if req.PollInterval != nil {
					svc.PollInterval = *req.PollInterval
				}
				if req.QueueAPI != nil {
					svc.QueueAPI = *req.QueueAPI
				}
				if req.FeedbackAPI != nil {
					svc.FeedbackAPI = *req.FeedbackAPI
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
					if req.Enable != nil {
						updateMap["enable"] = svc.Enable
					}
					if req.PollInterval != nil {
						updateMap["poll_interval"] = svc.PollInterval
					}
					if req.QueueAPI != nil {
						updateMap["queue_api"] = svc.QueueAPI
					}
					if req.FeedbackAPI != nil {
						updateMap["feedback_api"] = svc.FeedbackAPI
					}
					if len(updateMap) > 0 {
						if updateErr := db.Model(&existing).Updates(updateMap).Error; updateErr != nil {
							c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": updateErr.Error()})
							return
						}
					}
				}

				zapLog.Info("注册子服务", zap.String("service", name), zap.Bool("enable", svc.Enable))
				c.JSON(http.StatusOK, gin.H{"code": 0})
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
					if updateErr := db.Model(&existing).Updates(map[string]interface{}{
						"enable":        svc.Enable,
						"poll_interval": svc.PollInterval,
						"queue_api":     svc.QueueAPI,
						"feedback_api":  svc.FeedbackAPI,
					}).Error; updateErr != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": updateErr.Error()})
						return
					}
				}

				zapLog.Info("更新子服务配置", zap.String("service", svc.ServiceName), zap.Bool("enable", svc.Enable))
				c.JSON(http.StatusOK, gin.H{"code": 0})
			})
		}
	}
}
