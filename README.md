# League Timer Backend（微信云托管 Go）

League Timer 长跑接力计时小程序后端，基于「微信云托管 Go 官方模板」二次开发。

- **主裁判（chief）**：全局唯一、账号硬编码，拥有全部权限 —— 发令/终止/重置比赛、管理副裁判账号、记录/撤销全部比赛事件、导入参赛名单
- **副裁判（deputy）**：由主裁判创建，存 MySQL，可记录比赛事件（交接/备用带/补录/退赛）
- **观众**：无需登录，轮询查看实时排名（团体计分 + 个人用时）

时钟口径：全部使用服务器时钟，毫秒差值计算，时区无关。

## 赛制（本系统按此规则实现）

- 全队六棒、每棒 8 公里，共 48 公里；第一/三/五棒 A 段（1 号点→2 号点，男生），
  第二/四/六棒 B 段（2 号点→1 号点，男生；第六棒女生，终点在 1 号点）
- 第一棒随发令（视为比赛 0ms）自动出发，常规接力带
- **交接**：前棒完成与下棒开始为同一时刻；第 6 棒交接即冲线
- **个人时限**：每名选手自实际出发时刻起 50 分钟；满 50 分钟未交接，裁判安排下一棒用
  **备用黑色接力带**出发。备用带最早时刻 = 前棒出发 + 50 分钟；提前放行需显式确认并标记
- **补录**：备用带已发出后，超时选手抵达时补记实际完赛时刻；团体计分按 50:00 封顶，实际用时另记
- **退赛**：本队失去团体排名资格；下一棒须等退赛者实际出发满 50 分钟才能备用带出发；退赛者不参与个人奖
- **团体计分**：六棒计分用时之和（未超 50 分钟按实际、超时按 50:00）；
  **全员完赛且无退赛**才参与排名，同分并列
- **无暂停**：按赛制比赛发令后计时不可中止；误发令用「终止 + 重置」补救

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
│   └── model/                数据模型（race_event 为事件流核心）
├── go.mod / go.sum           Go module（无新增依赖，仅 gorm）
├── index.html                占位页（Dockerfile 需要该文件存在，接口不读取）
├── main.go                   主函数入口
└── service                   接口服务逻辑目录
    ├── engine.go             ★ 规则推导引擎（纯函数：快照/校验/计分/排名）
    ├── engine_test.go        引擎单测（13 个用例）
    └── ...
```

## 快速开始

前往 [微信云托管快速开始页面](https://developers.weixin.qq.com/miniprogram/dev/wxcloudrun/src/basic/guide.html)，选择 Go 语言模板，根据引导完成部署；或自行复制本仓库代码，在控制台新建服务并部署（见下文部署要点）。

本地调试请参考[微信云托管本地调试指南](https://developers.weixin.qq.com/miniprogram/dev/wxcloudrun/src/guide/debug/)；代码变动免重建预览见[实时开发指南](https://developers.weixin.qq.com/miniprogram/dev/wxcloudrun/src/guide/debug/dev.html)。

本地跑测试（无需数据库）：

```bash
go test ./...
```

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

- 建表与种子数据由服务启动时的 gorm `AutoMigrate` 自动完成，幂等可重复执行；
  **从旧版（record 表）升级**：新增 `race_event` 表自动创建，旧 `record` 表可留可删，
  建议正式比赛前「重置」并重新导入名单
- 服务监听容器 80 端口（本地可用 `PORT` 环境变量覆盖），与模板一致
- 密码哈希为 PBKDF2-SHA256（10000 轮、16 字节盐、32 字节派生），忘记副裁判密码可用 chief 删除后重建账号

## 权限模型

| 角色 | 来源 | 允许的接口 |
| --- | --- | --- |
| chief（主裁判） | 硬编码，不落库 | 除登录外全部接口：比赛控制、副裁判管理、事件记录/撤销、名单导入 |
| deputy（副裁判） | MySQL `referee` 表，由 chief 创建 | `POST /api/events`（四类事件均可记录）、登录登出 |
| 未登录 | - | `GET /api/race`、`GET /api/participants`、`GET /api/ranking`、`GET /` |

鉴权为无状态 token：每次请求携带请求头 `Authorization: Bearer <token>`。token 自登录起 7 天有效，服务端可随时通过更换 `TOKEN_SECRET` 使全部 token 失效。

## 通用约定

- 响应包统一为 `{"code":int, "errorMsg":string(omitempty), "data":any}`，HTTP 状态码恒为 `200`，业务结果由 `code` 表达
- 所有 POST/PUT 请求体为 JSON（`Content-Type: application/json`）
- `data` 内字段名为 lowerCamelCase，时间戳一律 Unix 毫秒
- 事件时刻 `ms` 一律为「自开赛累计毫秒」（与前端比赛时钟同基准），墙钟时刻才用 Unix 毫秒

### 错误码表

| code | 含义 |
| --- | --- |
| 0 | 成功 |
| -1 | 通用业务错误（参数非法、状态流转非法、凭据错误等），errorMsg 会给出中文说明 |
| 500 | 内部错误（细节仅记日志，不外泄 SQL 等信息） |
| 40100 | 未登录 / token 无效或已过期 |
| 40300 | 已登录但无权限（角色不匹配） |
| 40001 | 比赛未开始或已结束（记录事件时） |
| 40004 | 事件状态校验失败（交接/备用带/补录/退赛的规则不满足），errorMsg 说明原因 |
| 40005 | 备用带早于规则时刻，需确认放行；`data.earliestMs` 给出最早合法时刻 |
| 40006 | 撤销被后续记录依赖，需先撤销更晚的记录 |
| 40007 | 名单已锁定（比赛开始后不可修改） |
| 40008 | 名单格式或内容不合法（每队须 6 棒、第 1-5 棒男生、第 6 棒女生等） |

## 比赛状态机

单例比赛（`race` 表 id=1）：`idle`（初始）→ `running`（发令，计时开始）→ `finished`（终止）→ `reset` 回 `idle`。

- **无暂停**：按赛制发令后计时不可中止；`POST /api/race/pause` 已移除（保留路由返回说明文案）
- 已用时长 `elapsedMs = base_ms + (running ? now - started_at : 0)`
- `gun_at` 记录首次发令的墙钟时刻（前端显示「发令 8:00」用）
- `terminate` 冻结时钟后置 `finished`；`reset` 将 `base_ms` 清零、**删除全部比赛事件**，但保留名单
- 非法/幂等转换返回 `code -1`，errorMsg 如「比赛已终止，请先重置」

## 事件模型（核心）

`race_event` 表为追加式事件流，一切比赛状态由 **名单 + 事件顺序重放** 推导（`service/engine.go`，纯函数）：

| type | 含义 | 主要字段 |
| --- | --- | --- |
| `exchange` | 交接：前棒完成 + 下棒开始（第 6 棒为冲线，nextRunnerId 为 null） | `runnerId`（到达者）、`nextRunnerId`（自动派生的下一棒） |
| `backup_start` | 备用黑带出发 | `runnerId`、`early`（是否提前放行） |
| `solo_finish` | 补录到达（备用带已发出后超时选手完赛） | `runnerId` |
| `withdraw` | 退赛 | `runnerId` |

所有事件写入/撤销都在 `SELECT ... FOR UPDATE` 行锁事务内完成（串行化避免并发推导竞态）。

### 推导规则要点

- 发令后各队第一棒自动视为 0ms 常规带出发（不落事件）
- 交接校验：到达者在途、下一棒未出发；黑带已发出则要求改用补录
- 备用带校验：本棒未出发、上一棒在途或已退赛；最早时刻 = 上一棒出发 + 50 分钟
- 补录校验：本棒在途且下一棒已用备用带出发
- 退赛校验：本棒在途
- 撤销保护：交接/备用带若有后续事件依赖（撤销会破坏推导一致性）则拒绝（40006）
- 团体计分：每棒 `min(实际用时, 50:00)` 求和；全员完赛且无退赛才有名次；同分并列
- 在途看板：按「距个人时限剩余时间」升序，附将到达的交接点（奇数棒→2 号点、偶数棒→1 号点）

## 数据表

| 表 | 说明 |
| --- | --- |
| `referee` | 副裁判账号（username 唯一，PBKDF2 密码摘要） |
| `team` / `runner` | 队伍与选手（runner 含性别 M/F；名单由 chief 整体导入替换） |
| `race` | 比赛单例行（id=1，含 `gun_at` 发令墙钟） |
| `race_event` | 比赛事件流（追加式；按 ms、id 重放推导全部状态） |

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

校验规则：username 2-32 字符且仅限字母/数字/下划线；password ≥ 6 字符；displayName 非空且 ≤ 32 字符。不允许创建用户名为 `chief` 的账号。

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

`startedAt` 在终止/重置后为 `null`；`elapsedMs` 按 `base_ms + (running ? now - started_at : 0)` 实时计算。

### 6/7/8. 比赛控制 `POST /api/race/start` `/terminate` `/reset`

角色：chief。请求体为空 JSON `{}` 或空均可。

状态转换与响应（三者响应结构相同）：

| 接口 | 转换 | 备注 |
| --- | --- | --- |
| start | idle\|paused → running | started_at=now；首次发令记录 gun_at |
| terminate | running\|idle → finished | 冻结时钟后置 finished；gun_at 保留 |
| reset | 任意 → idle | base_ms=0、清空 gun_at，**删除全部比赛事件**，名单保留 |

响应（start 示例）：

```json
{
  "code": 0,
  "data": {
    "race": { "status": "running", "startedAt": 1764180000000, "elapsedMs": 0, "updatedAt": 1764180000000 }
  }
}
```

错误（`code -1`）：已 finished 时 start/terminate 返回 `比赛已终止，请先重置`；running 时 start 返回 `比赛已在进行中`。非 chief 调用返回 `40300`。

> `POST /api/race/pause` 已按赛制移除：调用返回 `-1 按赛制比赛不支持暂停：误发令请用「终止 + 重置」补救`。

### 9. 队伍与选手 `GET /api/participants`

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
          { "id": 1, "no": 1, "name": "陈家豪", "gender": "M" },
          { "id": 6, "no": 6, "name": "苏婉清", "gender": "F" }
        ]
      }
    ]
  }
}
```

队伍按 id 升序，选手按棒次 no（1-6）升序。

### 10. 导入名单 `PUT /api/participants`

角色：chief，且仅 `idle` 状态（发令后返回 40007）。整体覆盖：清空旧队伍/选手/事件后写入新名单。

请求：

```json
{
  "teams": [
    {
      "name": "青云跑团",
      "runners": [
        { "no": 1, "name": "陈家豪", "gender": "M" },
        { "no": 2, "name": "林亦舟", "gender": "M" },
        { "no": 3, "name": "周子昂", "gender": "M" },
        { "no": 4, "name": "何宇航", "gender": "M" },
        { "no": 5, "name": "邵一鸣", "gender": "M" },
        { "no": 6, "name": "苏婉清", "gender": "F" }
      ]
    }
  ]
}
```

校验规则：每队恰好 6 人且棒次 1-6 各一棒；第一至五棒男生、第六棒女生（按赛制）；
队名不重复且 ≤ 32 字。响应同 `GET /api/participants`（返回导入后的名单）。

错误：`40007 比赛已开始，重置比赛后才能修改名单`；`40008` 具体规则文案（如「按赛制第6棒应为女选手」）。

### 11. 记录比赛事件 `POST /api/events`

角色：chief 或 deputy。四类事件共用一个接口。

请求（以交接为例）：

```json
{
  "type": "exchange",
  "runnerId": 1,
  "ms": 654321,
  "clientAt": 1764180000123,
  "confirmEarly": false
}
```

| 字段 | 说明 |
| --- | --- |
| `type` | `exchange` \| `backup_start` \| `solo_finish` \| `withdraw` |
| `runnerId` | 事件主体选手（交接 = 到达交接区的选手） |
| `ms` | 事件发生时刻（自开赛累计毫秒，与比赛时钟同基准） |
| `clientAt` | 可选，客户端墙钟毫秒时间戳（审计用） |
| `confirmEarly` | 可选，备用带早于规则时刻时的显式放行确认 |

响应（交接示例）：

```json
{
  "code": 0,
  "data": {
    "event": {
      "id": 3,
      "type": "exchange",
      "runnerId": 1,
      "runnerNo": 1,
      "runnerName": "陈家豪",
      "nextRunnerId": 2,
      "nextRunnerNo": 2,
      "nextRunnerName": "林亦舟",
      "teamId": 1,
      "teamName": "青云跑团",
      "ms": 654321,
      "early": false,
      "recordedBy": "张副裁",
      "createdAt": 1764180000456
    }
  }
}
```

错误码：

| code | 场景示例 |
| --- | --- |
| 40001 | 比赛非 running（未发令/已终止） |
| 40004 | 规则校验失败：如「第2棒 xxx 已出发」「未到最早出发时刻需确认」的兄弟场景、交接对象不在途、下一棒已黑带出发需改补录、未出发选手退赛等（errorMsg 给出完整说明） |
| 40005 | 备用带早于「上一棒出发 + 50 分钟」，`data: {"earliestMs": 3000000}`；客户端确认后带 `confirmEarly:true` 重发即放行（事件标记 `early=true`） |
| -1 | 参数无效 / 选手不存在 |

### 12. 撤销事件 `DELETE /api/events?id=123`

角色：chief。按依赖规则校验后物理删除。

- 交接/备用带事件若有后续事件依赖（如交接启动的下一棒已有新记录）→ `40006 该交接启动的下一棒已有后续记录，请先撤销其后更晚的记录`
- 补录与退赛不阻塞任何记录，可随时撤销
- 删除后全部推导状态自动回滚（排行榜、看板随之更新）

响应：`{ "code": 0, "data": {} }`

### 13. 实时榜单 `GET /api/ranking`

角色：无需登录（观众 1 秒轮询；服务端为 4 条轻量查询 + 内存推导，无锁无 N+1）。
任何比赛状态都返回当前推导结果；未发令时各队均为候发。

响应（running 中场示例，节选）：

```json
{
  "code": 0,
  "data": {
    "status": "running",
    "startedAtMs": 1764180000000,
    "elapsedMs": 660000,
    "teams": [
      {
        "id": 1, "name": "青云跑团",
        "status": "running", "qualified": false,
        "rank": null, "scoreMs": null,
        "activeLegNo": 2, "onCourseLegNo": 2,
        "finishedLegs": 1, "totalLegs": 6,
        "legs": [
          {
            "runnerId": 1, "no": 1, "name": "陈家豪", "gender": "M",
            "status": "finished", "started": true, "startMs": 0, "belt": "normal",
            "earlyStart": false, "finishMs": 1500000, "finishType": "exchange",
            "durationMs": 1500000, "scoredMs": 1500000, "overtime": false,
            "deadlineMs": null
          },
          {
            "runnerId": 2, "no": 2, "name": "林亦舟", "gender": "M",
            "status": "oncourse", "started": true, "startMs": 1500000, "belt": "normal",
            "earlyStart": false, "finishMs": null, "finishType": "",
            "durationMs": null, "scoredMs": null, "overtime": false,
            "deadlineMs": 4500000
          }
        ]
      }
    ],
    "individualM": [
      { "rank": 1, "runnerId": 1, "no": 1, "name": "陈家豪", "teamId": 1, "teamName": "青云跑团", "durationMs": 1500000, "overtime": false }
    ],
    "individualF": [],
    "onCourse": [
      {
        "runnerId": 2, "no": 2, "name": "林亦舟", "gender": "M",
        "teamId": 1, "teamName": "青云跑团",
        "startMs": 1500000, "elapsedMs": 60000, "remainingMs": 2940000,
        "overdue": false, "station": 1
      }
    ],
    "events": [
      {
        "id": 3, "type": "exchange", "runnerId": 1, "runnerNo": 1, "runnerName": "陈家豪",
        "nextRunnerId": 2, "nextRunnerNo": 2, "nextRunnerName": "林亦舟",
        "teamId": 1, "teamName": "青云跑团", "ms": 1500000, "early": false,
        "recordedBy": "张副裁", "createdAt": 1764181500456
      }
    ]
  }
}
```

- `teams`：团体榜（资格队伍按总分在前 → 进行中按进度 → 退赛除名殿后）。
  `status`: `waiting|running|finished|disqualified`；`rank`/`scoreMs` 仅全员完赛且无退赛的非 null；
  `legs[]` 每棒含 `startMs/finishMs/durationMs/scoredMs/overtime/belt(常规 normal / 备用黑带 black)/earlyStart/deadlineMs(在途)`
- `individualM` / `individualF`：个人实际用时榜（按实际用时升序，退赛者不在内，超时者带 `overtime:true`）
- `onCourse`：在途选手看板（按剩余时间升序；`station` = 将到达的交接点，奇数棒 2、偶数棒 1；`overdue` 已超时）
- `events`：事件流水（按发生时刻升序）

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
