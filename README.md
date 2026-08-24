# Royal Flush

Royal Flush 是面向熟人邀请组局的在线 No-Limit Texas Hold'em Web 项目。产品只使用娱乐积分，不提供充值、支付、提现、兑换、转赠、交易、奖品或任何可兑现能力。

## 项目组成

- `apps/web`：Vue 3 玩家端，覆盖邀请登录、房间大厅、等候室、牌桌、语音和个人账本。
- `apps/admin`：独立 Vue 3 运营端，覆盖用户、房间、举报、审计和全站积分重置。
- `services/server`：Go 模块化单体，提供 REST、WebSocket、牌局状态机、积分账本和运营接口。
- `packages/contracts`：由 OpenAPI 与 WebSocket JSON Schema 生成的共享类型。
- `api`：HTTP 与实时消息契约源文件。
- `e2e`：四种目标视口、实时重连和运营工作流的 Playwright 测试。
- `deploy`：Nginx 与 LiveKit/TURN 的本地容器配置。

## 环境要求

- Node.js 22 或更高版本，npm 10 或更高版本。
- Go 1.27 或与 `services/server/go.mod` 兼容的版本。
- 运行完整依赖栈时需要 Docker Engine 和 Docker Compose v2。

## 前端演示模式

不启动后端也可以查看完整的玩家端和运营端演示数据：

```powershell
npm ci
npm run dev
```

- 玩家端：<http://localhost:5173>
- 运营端：<http://localhost:5174>

演示牌桌会常驻显示“本地演示数据”。演示模式不会连接真实 PostgreSQL、Redis 或 LiveKit。

## 完整本地栈

先创建本地环境文件，再启动六个服务：

```powershell
Copy-Item .env.example .env
docker compose up --build
```

- 玩家端：<http://localhost:5173>
- 运营端：<http://localhost:5174>
- Go API：<http://localhost:8080>
- LiveKit WebSocket：`ws://localhost:7880`
- LiveKit 内置 TURN：`localhost:3478/udp`
- PostgreSQL：`localhost:5432`
- Redis：`localhost:6379`

`.env.example` 中的密钥只适用于本机开发。进入共享测试或生产环境前，必须替换数据库密码、LiveKit 密钥、域名、TLS 证书和允许来源，并将 LiveKit 的 `use_external_ip`、TURN 域名及端口范围调整为实际网络配置。

## 单独启动后端

PowerShell 示例：

```powershell
$env:DATABASE_URL='postgres://royal_flush:royal_flush_dev@localhost:5432/royal_flush?sslmode=disable'
$env:REDIS_URL='redis://localhost:6379/0'
$env:LIVEKIT_URL='ws://localhost:7880'
$env:LIVEKIT_API_KEY='devkey'
$env:LIVEKIT_API_SECRET='secretsecretsecretsecretsecretsecret'
$env:ALLOWED_ORIGINS='http://localhost:5173,http://localhost:5174'
$env:ADMIN_USER_IDS='local-admin'
go run ./cmd/server
```

以上命令需要在 `services/server` 目录执行。服务启动时会自动运行 PostgreSQL migration；非开发环境缺少 PostgreSQL 或 Redis 配置时会拒绝启动。

## 健康检查

- `GET /api/v1/health`：只表示 HTTP 进程存活。
- `GET /api/v1/ready`：验证启动时注册的 PostgreSQL 和 Redis 依赖仍然可用；依赖异常时返回 `503`。

容器编排使用 `/api/v1/ready` 作为 Go 服务的 readiness gate，前端只会在后端就绪后启动。

## 验证命令

```powershell
npm test
npm run typecheck
npm run build
npm run contracts:check
npm run test:e2e
```

Go 验证需在 `services/server` 目录执行：

```powershell
go test ./...
go vet ./...
go test -race ./...
```

`go test -race` 在 Windows 上需要可用的 C 编译器。契约检查会重新生成 Go/TypeScript 类型，并在生成结果与仓库不一致时失败。

## 容量基线

容量工具会通过开发环境身份头创建真实房间和座位、保持 WebSocket 连接、开始牌局、提交真实弃牌行动，并在真实断线后验证首帧权威快照。它仅应在隔离的本地或测试环境运行，因为测试账号和房间会保存在目标服务配置的存储中。

先启动开发后端，再从 `services/server` 目录运行完整基线：

```powershell
go run ./cmd/loadtest `
  -target http://127.0.0.1:8080 `
  -connections 1000 `
  -rooms 120 `
  -actions 120 `
  -reconnects 120 `
  -output capacity-report.json
```

默认验收条件为行动确认 p95 不超过 `200ms`、所有重连首帧不超过 `3s`，任一指标失败时命令返回非零退出码。报告为机器可读 JSON。用于本地快速检查或 CI 的缩小烟测可以使用：

```powershell
go run ./cmd/loadtest -connections 16 -rooms 4 -actions 4 -reconnects 4
```

## 当前运行边界

- 非开发环境的短信验证码目前只生成挑战码，尚未接入真实短信服务商。
- 登录 session 当前保存在单个 Go 进程内；多实例部署前需迁移到共享 session 存储。
- 运营端活跃房间列表来自当前服务实例；多实例运营视图需要增加跨实例聚合。
- 语音不录制、不保存、不转写；LiveKit 或 TURN 不可用时不阻止继续打牌。
- 上线前仍需完成 ICP、隐私政策、用户协议、手机号与语音个人信息处理、未成年人策略及非博彩边界专项合规审查。
