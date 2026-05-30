package main

import (
"encoding/json"
"fmt"
"msg-service/common"
"time"
)

const (
ServiceName = "feishu_msg"
ServiceDesc = "飞书消息&待办推送服务"
MasterAddr  = "http://127.0.0.1:8080"
)

func main() {
registerRes := common.HttpPost(MasterAddr+"/api/v1/sub/register", map[string]string{
"service_name": ServiceName,
"desc":         ServiceDesc,
})
fmt.Printf("注册结果：%s\n", registerRes)

for {
cfgRes := common.HttpGet(MasterAddr + "/api/v1/sub/config?service_name=" + ServiceName)
var subCfg common.SubConfig
if err := json.Unmarshal(cfgRes, &subCfg); err != nil {
fmt.Printf("解析配置失败：%v\n", err)
time.Sleep(3 * time.Second)
continue
}

if !subCfg.Enable {
time.Sleep(1 * time.Second)
continue
}

taskRes := common.HttpGet(subCfg.QueueAPI + "?service=" + ServiceName)
var tasks []common.Task
if err := json.Unmarshal(taskRes, &tasks); err != nil {
fmt.Printf("解析任务失败：%v\n", err)
time.Sleep(time.Duration(subCfg.PollInterval) * time.Second)
continue
}

for _, task := range tasks {
fmt.Printf("处理飞书消息任务：%s\n", task.TaskID)

platformRes := common.HttpGet(MasterAddr + "/api/v1/common/platform?patname=" + task.PatName)
tokenRes := common.HttpGet(MasterAddr + "/api/v1/common/token?patname=" + task.PatName)

success := true
message := "处理成功"

feedbackData := map[string]interface{}{
"task_id": task.TaskID,
"result":  success,
"message": message,
}
common.HttpPost(subCfg.FeedbackAPI, feedbackData)
}

time.Sleep(time.Duration(subCfg.PollInterval) * time.Second)
}
}
