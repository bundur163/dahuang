# 修改日志

## 2026-06-04 — 子服务属性重新设计 & 批量启停

### 概述

重新设计子服务数据模型，新增显示名称、服务程序路径、配置服务器URL、API服务器URL字段；增加全部启动和全部停止功能。

### 修改文件

| 文件 | 变更类型 | 说明 |
|---|---|---|
| `model/model.go` | 修改 | SubService 结构体新增 4 个字段 |
| `api/sub_api.go` | 修改 | 新增 stopall/startall 端点，适配新字段 |
| `web/index.html` | 修改 | 重新设计卡片、弹窗、表单 |

### model/model.go — 数据模型

SubService 结构体变更：

**新增字段：**
- `DisplayName string` — 显示名称（如 "钉钉消息服务"），在 UI 中作为卡片标题
- `BinPath string` — 可执行程序路径（如 "subs/dingtalk_msg/dingtalk_msg"），启动时读取
- `ConfigServerURL string` — 配置服务器URL（如 "https://cloud.peopleone.cn"）
- `APIServerURL string` — API服务器URL（如 "http://localhost/dev"）

**保留字段：**
- `ServiceName` — 内部唯一标识，子服务注册/查询时的 key
- `Enable`, `PollInterval`, `QueueAPI`, `FeedbackAPI` — 原有属性
- `RealPID` — 运行时计算字段（非数据库列）

### api/sub_api.go — API 变更

**新增端点：**
- `POST /api/v1/sub/stopall` — 停止所有运行中的子服务（不更新数据库 `enable` 字段）
- `POST /api/v1/sub/startall` — 启动数据库中 `enable=true` 的子服务（使用 `BinPath` 字段）

**修改端点：**
- `/start` — `bin_path` 参数改为可选，未传时自动从数据库 `BinPath` 字段读取
- `/register` — 接受新增字段：`DisplayName`, `BinPath`, `ConfigServerURL`, `APIServerURL`
- `/update` — 更新全部 8 个属性字段（原有 4 个 + 新增 4 个）

### web/index.html — 前端变更

**卡片展示：**
- 标题改为 `DisplayName`（回退到 `ServiceName`）
- 副标题显示内部标识名
- 新增信息行：服务程序、配置服务器、API服务器

**配置弹窗：**
- 新增可编辑字段：显示名称、服务程序、配置服务器URL、API服务器URL

**新增弹窗：**
- 服务名称（必填）、显示名称、服务程序等全部字段

**交互优化：**
- `toggleService` 改为先查询 `/config` 获取 `BinPath`，再调用 `/start`
- HTML 转义处理防止 XSS

**新增按钮：**
- 顶部栏增加「全部启动」和「全部停止」按钮

---

## 2026-06-04 — MD5标识 & 启动模式 & 管理器自动唤起

### 概述

1. 内部唯一标识改为 BinPath 的 MD5 值
2. 卡片副标题显示可执行程序路径，精简属性列表
3. 新增启动模式字段（自动启动/手工启动/禁用）
4. 管理器启动时自动唤起「自动启动」模式的子服务
5. PID 查找兼容 MD5 标识名

### 修改文件

| 文件 | 变更类型 | 说明 |
|---|---|---|
| `model/model.go` | 修改 | 新增 `StartMode` 字段 |
| `api/sub_api.go` | 修改 | MD5 生成 ServiceName、StartMode 支持、PID 查找修复 |
| `main.go` | 修改 | 启动时自动唤起 `start_mode=auto` 的子服务 |
| `web/index.html` | 修改 | 精简卡片、副标题显示 BinPath、启动模式编辑器 |

### model/model.go — 新增字段

- `StartMode string` — 启动模式，取值：`"auto"`（自动启动）、`"manual"`（手工启动，默认）、`"disabled"`（禁用）

### api/sub_api.go — 详细变更

**MD5 标识：**
- 新增 `md5sum()` 辅助函数
- `/register`：若未提供 `ServiceName` 但提供了 `BinPath`，自动以 `MD5(BinPath)` 生成唯一标识
- `/register` 返回新生成的 `service_name` 供前端记录

**PID 查找修复：**
- 新增 `resolvePID()` 辅助函数，先用 `ServiceName` 查 PID，失败时用 `filepath.Base(BinPath)` 兼容 MD5 标识名不匹配进程名的情况
- `/list`、`/config`、`/stopall`、`/startall`、`/start` 全部改用 `resolvePID()`

**StartMode 支持：**
- `/register` — 接受 `StartMode` 字段
- `/update` — 更新 `start_mode` 字段
- `/startall` — 排除 `start_mode = "disabled"` 的服务

### main.go — 自动唤起

- 新增 `"path/filepath"` 和 `"time"` 依赖
- HTTP 服务启动 2 秒后，查询 `enable=true AND start_mode="auto"` 的子服务并依次启动
- 跳过未配置 BinPath 或二进制不存在的服务并记录告警

### web/index.html — 前端变更

**卡片布局：**
- 标题：`DisplayName`（回退到 `ServiceName`）
- 副标题：`BinPath`（等宽字体），替代原来的内部标识名
- 属性列表精简为 5 项：进程PID、轮询周期、API服务器、队列API、反馈API
- 新增启动模式徽章（蓝=自动、橙=手工、灰=禁用）

**新增弹窗：**
- 移除「服务名称」输入框（由后端根据 BinPath 的 MD5 自动生成）
- 「服务程序」改为必填
- 新增「启动模式」下拉框（自动启动/手工启动/禁用）

**配置弹窗：**
- 新增「启动模式」下拉框
- 保留「运行状态」开关

### 已知问题修复 (2026-06-04)

- **start_mode 列缺失**：旧数据库没有 `start_mode` 列，导致配置保存失败且无错误提示。修复：
  - 手动为现有数据库添加 `start_mode TEXT DEFAULT 'manual'` 列
  - `/update` 处理函数改为动态构建更新映射，仅在 `StartMode` 非空时包含该字段
  - 前端 `saveConfig` 添加 `.catch()` 错误提示

### 已知问题修复 (2026-06-04)

- **自动启动未生效**：
  - 查询条件从 `enable=true AND start_mode='auto'` 改为只查 `start_mode='auto'`（自动启动语义本身就包含启用）
  - `main.go` 自动启动成功后追加 `db.Update("enable", true)` 以与 `/start` API 行为一致

---

## 2026-06-04 — AppID / Secret & 主服务公共配置继承

### 概述

新增 `AppID` 和 `Secret` 字段用于调用 API 服务器。这三个属性（APIServerURL、AppID、Secret）作为主服务的公共配置，子服务可继承或覆盖。

### 修改文件

| 文件 | 变更类型 | 说明 |
|---|---|---|
| `model/model.go` | 修改 | SubService 新增 AppID/Secret；新增 MainConfig 结构体 |
| `main.go` | 修改 | AutoMigrate 增加 MainConfig，启动时种子默认行 |
| `api/common_api.go` | 修改 | 新增 GET/POST `/settings` 端点 |
| `api/sub_api.go` | 修改 | `/config` 增加主服务配置继承回退；/register、/update 支持 AppID/Secret |
| `web/index.html` | 修改 | 主服务设置弹窗；子服务配置/新增界面增加 AppID/Secret 字段 |

### model/model.go

- SubService 新增：`AppID string`、`Secret string`
- 新增 `MainConfig` 结构体（单行表），作为所有子服务的默认值来源

### api/common_api.go

- `GET /api/v1/common/settings` — 获取主服务公共配置
- `POST /api/v1/common/settings` — 保存主服务公共配置

### api/sub_api.go — 配置继承

- `/config` 端点：子服务的 APIServerURL/AppID/Secret 为空时，自动从 `MainConfig` 表中读取填充
- `/register`、`/update`：支持 AppID、Secret 字段

### web/index.html

- 顶部栏新增「⚙ 主服务设置」按钮（紫色），弹出设置弹窗编辑 APIServerURL、AppID、Secret
- 子服务配置弹窗和新增弹窗增加 AppID、Secret 输入框（placeholder 提示"留空则继承主服务设置"）
- 新增子服务时自动从主服务配置预填 APIServerURL、AppID、Secret

### 已知问题修复 (2026-06-04)

- **禁用模式下启动按钮仍可用**：卡片渲染时 `startMode==="disabled"` 且未运行则按钮加 `disabled` 属性；`toggleService` 增加守卫拒绝启动

---

## 2026-06-04 — 弹窗布局优化

### 概述

所有弹窗统一改为三区域 flex 布局：固定标题头 + 可滚动中间内容区 + 固定底部按钮栏。配置弹窗字段多时可滚动不撑破屏幕。

### 修改文件

| 文件 | 变更类型 | 说明 |
|---|---|---|
| `web/index.html` | 修改 | CSS 新增 `.form-scroll`、`.modal-btns`；配置/设置/新增/日志弹窗统一布局 |

---

## 2026-06-04 — 扩展属性（ExtraAttrs）

### 概述

新增 `ExtraAttrs` 字段，存储单层 JSON 键值对。管理界面提供可视化键值对编辑器。子服务请求配置时，扩展属性自动合并到 JSON 响应顶层。

### 修改文件

| 文件 | 变更类型 | 说明 |
|---|---|---|
| `model/model.go` | 修改 | SubService 新增 `ExtraAttrs string` 字段 |
| `api/sub_api.go` | 修改 | `/config` 解析 ExtraAttrs 并合并到响应顶层（固定属性优先）；/register、/update 支持 ExtraAttrs |
| `web/index.html` | 修改 | 配置/新增弹窗增加只读输入框+编辑按钮；新增键值对编辑器弹窗 |

### 数据存储

- `ExtraAttrs` 列存储 JSON 字符串，如 `{"pool_size":"5","timeout":"30"}`
- 仅支持单层键值对，不嵌套

### /config 合并逻辑

```json
{
  "ServiceName": "abc123",
  "DisplayName": "钉钉",
  "ExtraAttrs": "{\"pool_size\":\"5\"}",
  "pool_size": "5"          ← 从 ExtraAttrs 展开
}
```

- ExtraAttrs 中的键值平铺到顶层
- 固定属性与扩展属性键名冲突时，固定属性优先（不覆盖）
- 保留 `ExtraAttrs` 原始字段供管理界面回显

### web/index.html — 键值对编辑器

- 配置/新增弹窗底部增加「扩展属性」只读输入框 + `[编辑]` 按钮
- 点击编辑弹出键值对编辑器：表格形式展示键名列+值列+删除按钮，底部 `[+]` 添加行
- 确定后序列化为 JSON 回填
