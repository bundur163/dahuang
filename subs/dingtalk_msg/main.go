package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	ServiceName = "dingtalk_msg"
	MasterAddr  = "http://127.0.0.1:8080"
)

type SubConfig struct {
	Enable       bool   `json:"Enable"`
	PollInterval int    `json:"PollInterval"`
	QueueAPI     string `json:"QueueAPI"`
	FeedbackAPI  string `json:"FeedbackAPI"`
}

type Task struct {
	TaskID  string `json:"task_id"`
	PatName string `json:"patname"`
	Content string `json:"content"`
}

func httpGet(url string) []byte {
	client := &http.Client{Timeout: 15 * time.Second}
	for i := 0; i < 3; i++ {
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			var buf bytes.Buffer
			buf.ReadFrom(resp.Body)
			return buf.Bytes()
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

func httpPost(url string, data interface{}) {
	body, _ := json.Marshal(data)
	client := &http.Client{Timeout: 15 * time.Second}
	client.Post(url, "application/json", bytes.NewBuffer(body))
}

func main() {
	fmt.Println("[dingtalk_msg] 启动，注册到主服务...")
	httpPost(MasterAddr+"/api/v1/sub/register", map[string]string{"service_name": ServiceName})

	for {
		cfgData := httpGet(MasterAddr + "/api/v1/sub/config?service_name=" + ServiceName)
		var cfg SubConfig
		json.Unmarshal(cfgData, &cfg)

		if !cfg.Enable {
			time.Sleep(3 * time.Second)
			continue
		}

		taskData := httpGet(cfg.QueueAPI + "?service=" + ServiceName)
		var tasks []Task
		json.Unmarshal(taskData, &tasks)

		for _, task := range tasks {
			// 调用主服务获取平台配置和 token
			platform := httpGet(MasterAddr + "/api/v1/common/platform?patname=" + task.PatName)
			token := httpGet(MasterAddr + "/api/v1/common/token?patname=" + task.PatName)
			fmt.Printf("处理任务 %s, platform=%s, token=%s\n", task.TaskID, string(platform), string(token))

			// TODO: 具体业务逻辑（发送钉钉消息）

			httpPost(cfg.FeedbackAPI, map[string]interface{}{
				"task_id": task.TaskID,
				"result":  true,
				"message": "success",
			})
		}

		time.Sleep(time.Duration(cfg.PollInterval) * time.Second)
	}
}
