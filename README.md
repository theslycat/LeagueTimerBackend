# League Timer Backend（微信云托管 Go）

League Timer 长跑计时小程序后端，基于「微信云托管 Go 官方模板」二次开发。

- **主裁判（chief）**：全局唯一、账号硬编码，拥有全部权限 —— 开始/暂停/终止/重置比赛、管理副裁判账号、记录交接时间
- **副裁判（deputy）**：由主裁判创建，存 MySQL，仅可登录/登出与记录选手交接时间
- **观众**：无需登录，轮询查看实时排名

时钟口径：全部使用服务器时钟，毫秒差值计算，时区无关。

## 目录结构

```
.
├── Dockerfile                Dockerfile（沿用模板，未改动）
├── LICENSE                   LICENSE 文件
├── README.md                 本文档
├── container.config.json     模板部署「服务设置」初始化配置（二开请忽略）
├── db                        数据库逻辑目录
│   ├── init.go               连接初始化
│   ├── migrate.go            AutoMigrate + 幂等种子数据
│   ├── dao/                  数据访问层
│   └── model/                数据模型
├── go.mod / go.sum           Go module（无新增依赖，仅 gorm）
├── index.html                占位页（Dockerfile 需要该文件存在，接口不读取）
├── main.go                   主函数入口
└── service                   接口服务逻辑目录
```

## 快速开始

前往 [微信云托管快速开始页面](https://developers.weixin.qq.com/miniprogram/dev/wxcloudrun/src/basic/guide.html)，选择 Go 语言模板，根据引导完成部署；或自行复制本仓库代码，在控制台新建服务并部署（见下文部署要点）。

本地调试请参考[微信云托管本地调试指南](https://developers.weixin.qq.com/miniprogram/dev/wxcloudrun/src/guide/debug/)；代码变动免重建预览见[实时开发指南](https://developers.weixin.qq.com/miniprogram/dev/wxcloudrun/src/guide/debug/dev.html)。

## 主裁判初始账号

按需求硬编码在代码中（`service/auth_service.go`），主裁判**不落库**：

```
用户名：chief
密　码：league-timer-2026
```

建议部署后立即用该账号调用 `POST /api/login` 自查一次；如需更换，请修改源码中的 `ChiefPassword` 常量并重新部署。

## 部署要点（环境变量）

如果不是通过微信云托管控制台部署模板代码，而是自行复制/下载代码后手动新建一个服务并部署，需要在「服务设置」中补全以下环境变量，否则会因无法连接数据库导致部署失败：

| 变量 | 说明 |
| --- | --- |
| `MYSQL_ADDRESS` | MySQL 连接地址（host:port），使用云托管内 MySQL 可在控制台 MySQL 页面获取 |
| `MYSQL_USERNAME` | MySQL 用户名 |
| `MYSQL_PASSWORD` | MySQL 密码 |
| `MYSQL_DATABASE` | 可选，默认 `league_timer`。手工部署时请确保该库已存在（或填成已有库名），服务启动时会自动建表并写入种子数据 |
| `TOKEN_SECRET` | 可选，无状态 token 的 HMAC 签名密钥，默认 `league-timer-dev-secret`。**生产环境务必设置强随机值；多实例部署时所有实例必须配置完全一致**，否则 token 跨实例校验失败导致 40100 |

其余说明：

- 建表与种子数据（race 单例、5 支演示队伍与 10 名选手）由服务启动时的 gorm `AutoMigrate` 自动完成，幂等可重复执行；`container.config.json` 仅配合模板首次部署生成服务设置，后续发布与本文件无关（其内的 `golang_demo`/`Counters` 演示建表 SQL 对本服务无害，可忽略）
- 服务监听容器 80 端口，与模板一致
- 密码哈希为 PBKDF2-SHA256（10000 轮、16 字节盐、32 字节派生），忘记副裁判密码可用 chief 删除后重建账号

## 权限模型

| 角色 | 来源 | 允许的接口 |
| --- | --- | --- |
| chief（主裁判） | 硬编码，不落库 | 除登录外全部接口：比赛控制、副裁判管理、记录/查询交接时间 |
| deputy（副裁判） | MySQL `referee` 表，由 chief 创建 | `POST /api/records`、`GET /api/records`、登录登出 |
| 未登录 | - | `GET /api/race`、`GET /api/participants`、`GET /api/ranking`、`GET /` |

鉴权为无状态 token：每次请求携带请求头 `Authorization: Bearer <token>`。token 自登录起 7 天有效，服务端可随时通过更换 `TOKEN_SECRET` 使全部 token 失效。

## 通用约定

- 响应包统一为 `{"code":int, "errorMsg":string(omitempty), "data":any}`，HTTP 状态码恒为 `200`，业务结果由 `code` 表达
- 所有 POST 请求体为 JSON（`Content-Type: application/json`）
- `data` 内字段名为 lowerCamelCase，时间戳一律 Unix 毫秒

### 错误码表

| code | 含义 |
| --- | --- |
| 0 | 成功 |
| -1 | 通用业务错误（参数非法、状态流转非法、凭据错误等），errorMsg 会给出中文说明 |
| 500 | 内部错误（细节仅记日志，不外泄 SQL 等信息） |
| 40100 | 未登录 / token 无效或已过期 |
| 40300 | 已登录但无权限（角色不匹配） |
| 40001 | 比赛未开始或已暂停（仅 `POST /api/records`） |
| 40002 | 该选手已记录交接时间（仅 `POST /api/records`） |
| 40003 | 选手不存在（仅 `POST /api/records`） |

## 比赛状态机

单例比赛（`race` 表 id=1）：`idle`（初始）→ `running`（进行中）⇄ `paused`（暂停）→ `finished`（终止）→ `reset` 回 `idle`。

- 已用时长 `elapsedMs = base_ms + (running ? now - started_at : 0)`，暂停时冻结累计毫秒到 `base_ms` 并将 `started_at` 置空
- `terminate` 先按暂停逻辑冻结时钟再置 `finished`；`reset` 将 `base_ms` 清零并**删除全部交接记录**
- 非法/幂等转换（如已 finished 再 start/pause）返回 `code -1`，errorMsg 如「比赛已终止，请先重置」

## 用户量与数据表

| 表 | 说明 |
| --- | --- |
| `referee` | 副裁判账号（username 唯一，PBKDF2 密码摘要） |
| `team` / `runner` | 队伍与选手（演示名单种子：青云跑团 101-102、山岳学院 103-104、江畔大学 105-106、北岭俱乐部 107-108、曙光中学 109-110） |
| `race` | 比赛单例行（id=1） |
| `record` | 交接记录，唯一索引 (race_id, runner_id) 防重复 |

---

## API 文档

以下 `<base>` 指服务域名。所有示例中 `Authorization: Bearer <token>` 分别对应两种角色的 token。

### 1. 登录 `POST /api/login`

角色：无需登录。

请求：

```json
{
  "username": "chief",
  "password": "league-timer-2026"
}
```

响应（主裁判示例）：

```json
{
  "code": 0,
  "data": {
    "token": "eyJ1aWQiOjAsInVzZXJuYW1lIjoiY2hpZWYiLCJyb2xlIjoiY2hpZWYiLCJkaXNwbGF5TmFtZSI6IuS4reWKoOS5puG5mCIsImV4cCI6MTc2MzA5NzYwMH0.9a8b7c...64位hex签名",
    "username": "chief",
    "role": "chief",
    "displayName": "主裁判"
  }
}
```

副裁判登录响应同构，`role` 为 `"deputy"`。凭据错误：`{"code":-1,"errorMsg":"用户名或密码错误"}`。

### 2. 副裁判列表 `GET /api/referees`

角色：chief。请求头需带 token。

请求：无参数。

响应：

```json
{
  "code": 0,
  "data": {
    "referees": [
      { "username": "deputy_01", "displayName": "张副裁", "createdAt": 1764180000000 }
    ]
  }
}
```

按创建时间升序，仅副裁判（不含 chief）。

### 3. 创建副裁判 `POST /api/referees`

角色：chief。

请求：

```json
{
  "username": "deputy_01",
  "password": "abc123",
  "displayName": "张副裁"
}
```

校验规则：username 2-32 字符且仅限字母/数字/下划线；password ≥ 6 字符；displayName 非空且 ≤ 32 字符。不允许创建 chief 角色或用户名为 `chief`。

响应：

```json
{
  "code": 0,
  "data": {
    "referee": { "username": "deputy_01", "displayName": "张副裁", "createdAt": 1764180000000 }
  }
}
```

错误：`-1 用户名已存在`；`-1` 具体参数校验文案；`40300` 非 chief。

### 4. 删除副裁判 `DELETE /api/referees?username=deputy_01`

角色：chief。

响应：

```json
{ "code": 0, "data": {} }
```

错误：`-1 裁判不存在`（username 不存在）；`-1 缺少 username 参数`。

### 5. 查询比赛 `GET /api/race`

角色：无需登录（前端 1 秒轮询或按需查询）。

请求：无参数。

响应（running 示例）：

```json
{
  "code": 0,
  "data": {
    "race": {
      "status": "running",
      "startedAt": 1764180000000,
      "elapsedMs": 654321,
      "updatedAt": 1764180000000
    }
  }
}
```

`startedAt` 在暂停/终止/重置后为 `null`；`elapsedMs` 按 `base_ms + (running ? now - started_at : 0)` 实时计算。

### 6/7/8/9. 比赛控制 `POST /api/race/start` `/pause` `/terminate` `/reset`

角色：chief。请求体为空 JSON `{}` 或空均可。

状态转换与响应（四者响应结构相同）：

| 接口 | 转换 | 备注 |
| --- | --- | --- |
| start | idle\|paused → running | started_at=now，base_ms 保留 |
| pause | running → paused | base_ms 累计本段，started_at 置 null |
| terminate | running\|paused\|idle → finished | 先按 pause 冻结时钟再置 finished |
| reset | 任意 → idle | base_ms=0，**删除全部 record** |

响应（pause 示例）：

```json
{
  "code": 0,
  "data": {
    "race": { "status": "paused", "startedAt": null, "elapsedMs": 654321, "updatedAt": 1764180100000 }
  }
}
```

错误（`code -1`）：已 finished 时 start/pause/terminate 返回 `比赛已终止，请先重置`；running 时 start 返回 `比赛已在进行中`；idle 时 pause 返回 `比赛尚未开始`；paused 时 pause 返回 `比赛已暂停`。非 chief 调用返回 `40300`。

### 10. 队伍与选手 `GET /api/participants`

角色：无需登录。

响应：

```json
{
  "code": 0,
  "data": {
    "teams": [
      {
        "id": 1,
        "name": "青云跑团",
        "runners": [
          { "id": 1, "no": 101, "name": "陈家豪" },
          { "id": 2, "no": 102, "name": "林亦舟" }
        ]
      }
    ]
  }
}
```

队伍按 id 升序，选手按号码 no 升序。

### 11. 记录交接时间 `POST /api/records`

角色：chief 或 deputy。

请求：

```json
{
  "runnerId": 1,
  "passMs": 654321,
  "clientAt": 1764180000123
}
```

`passMs` 为该选手自开赛累计毫秒（成绩）；`clientAt` 为客户端上报的墙钟毫秒时间戳，可省略（可空）。`serverMs` 为服务端落库那一刻比赛的累计毫秒（审计用）。

响应：

```json
{
  "code": 0,
  "data": {
    "record": {
      "id": 1,
      "runnerId": 1,
      "passMs": 654321,
      "serverMs": 654400,
      "recordedBy": "张副裁",
      "createdAt": 1764180000456
    }
  }
}
```

错误码：

| code | errorMsg | 场景 |
| --- | --- | --- |
| 40001 | 比赛未开始或已暂停 | 比赛非 running 状态 |
| 40002 | 该选手已记录交接时间 | 同一比赛同一选手重复记录 |
| 40003 | 选手不存在 | runnerId 无效 |
| -1 | 参数 xxx 无效 / 请求参数格式错误 | 参数非法 |

### 12. 交接记录列表 `GET /api/records`

角色：chief 或 deputy。

响应（按 passMs 升序）：

```json
{
  "code": 0,
  "data": {
    "records": [
      { "id": 2, "runnerId": 3, "passMs": 601000, "serverMs": 601050, "recordedBy": "主裁判", "createdAt": 1764180001456 },
      { "id": 1, "runnerId": 1, "passMs": 654321, "serverMs": 654400, "recordedBy": "张副裁", "createdAt": 1764180000456 }
    ]
  }
}
```

### 13. 实时排名 `GET /api/ranking`

角色：无需登录（观众 1 秒轮询；服务端为 4 条轻量查询 + 内存组装，无锁无 N+1）。比赛未运行时也照常返回已记录成绩。

响应：

```json
{
  "code": 0,
  "data": {
    "status": "running",
    "elapsedMs": 660000,
    "rows": [
      { "rank": 1,   "runnerId": 3, "no": 103, "name": "周子昂", "teamId": 2, "teamName": "山岳学院", "passMs": 601000, "gapMs": 0,     "status": "recorded" },
      { "rank": 2,   "runnerId": 1, "no": 101, "name": "陈家豪", "teamId": 1, "teamName": "青云跑团", "passMs": 654321, "gapMs": 53321, "status": "recorded" },
      { "rank": null, "runnerId": 2, "no": 102, "name": "林亦舟", "teamId": 1, "teamName": "青云跑团", "passMs": null, "gapMs": null, "status": "running" }
    ],
    "teams": [
      { "id": 1, "name": "青云跑团", "recorded": 1, "total": 2 },
      { "id": 2, "name": "山岳学院", "recorded": 1, "total": 2 }
    ]
  }
}
```

- `rows`：已记录者按 passMs 升序（rank=1..k，gapMs=与第一名之差），随后未记录者 rank/passMs/gapMs 均为 `null`、status 为 `"running"`，按队伍 id、号码 no 排序
- `teams`：每队完成人数 `recorded` / 总人数 `total`

### 14. 健康检查 `GET /`

角色：无需登录。

响应：

```json
{ "service": "league-timer-backend", "status": "ok" }
```

注意：该响应**不是**模板的 JSON envelope，而是纯 JSON 字符串，便于网关健康探测。

---

## License

[MIT](./LICENSE)
