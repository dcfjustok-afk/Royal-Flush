# Royal Flush 部署到 Zeabur

本文档对应当前的前后端分离架构，并从 Zeabur 的“构建计划预览”界面开始。最终在同一个 Zeabur Project 中运行五个服务：

| 服务名       | 来源                        | 是否公开 | 作用                            |
| ------------ | --------------------------- | -------- | ------------------------------- |
| `postgresql` | Zeabur 官方 PostgreSQL 模板 | 否       | 权威牌局、积分、房间和审计数据  |
| `redis`      | Zeabur 官方 Redis 模板      | 否       | 房间租约与单实例所有权          |
| `server`     | 本仓库 `Dockerfile.server`  | 否       | Go REST、WebSocket 和游戏状态机 |
| `web`        | 本仓库 `Dockerfile.web`     | 是       | 玩家端与 `/api/` 反向代理       |
| `admin`      | 本仓库 `Dockerfile.admin`   | 是       | 运营端与 `/api/` 反向代理       |

Zeabur 当前不支持直接从 Docker Compose YAML 部署。仓库根目录的 `compose.yaml` 只用于本地完整栈，不能作为 Zeabur 的部署入口。

## 先处理当前构建预览

如果当前页面显示以下自动识别结果，不要点击紫色“部署”按钮：

```text
Framework: none
Package manager: npm
Install: npm install
Build: npm run build
Start: node index.js
```

仓库没有根目录 `index.js`，这套计划无法启动项目。先关闭该预览，或进入“配置”添加 `ZBPACK_DOCKERFILE_NAME=server`。正确识别后，构建来源应是根目录的 `Dockerfile.server`，而不是 `node index.js`。

本地上传是一次性源码快照。仓库已经加入 Zeabur 构建文件和运营端登录，因此此前上传的旧快照不会自动更新。请使用当前仓库目录重新上传；后续若需要频繁发布，建议将同一仓库连接到三个 Zeabur 服务，让每次提交触发对应 Dockerfile 的重新构建。

## 1. 创建 PostgreSQL

1. 回到当前 Zeabur Project 的服务列表。
2. 选择“添加服务”，从 Marketplace 添加官方 PostgreSQL 服务。
3. 等待状态变为运行中。
4. 不需要给 PostgreSQL 绑定公开域名或 TCP 公网端口。

官方模板会提供 `POSTGRES_CONNECTION_STRING`。Go 服务稍后通过 `${POSTGRES_CONNECTION_STRING}` 引用它，不要把数据库密码复制进仓库。

## 2. 创建 Redis

1. 再次选择“添加服务”，从 Marketplace 添加官方 Redis 服务。
2. 等待状态变为运行中。
3. 不需要给 Redis 绑定公开域名或 TCP 公网端口。

官方模板会提供 `REDIS_CONNECTION_STRING`。Go 服务稍后通过 `${REDIS_CONNECTION_STRING}` 引用它。

## 3. 部署私有 Go 服务

1. 使用更新后的本地目录创建一个源码服务。
2. 将服务名设为小写的 `server`。这个名称会生成供其他服务使用的 `SERVER_HOST`。
3. 在服务变量中录入 [server.env.example](deploy/zeabur/server.env.example) 的内容。
4. 将 `ADMIN_PHONES` 替换为实际运营手机号；多个号码使用英文逗号分隔，例如 `手机号A,手机号B`。
5. 不要手动设置 `PORT`。Zeabur 会注入监听端口，程序默认兼容 `8080`。
6. 确认构建计划使用 `Dockerfile.server` 后再部署。
7. 将健康检查路径设为 `/api/v1/ready`。
8. 不给 `server` 绑定公开域名。玩家端和运营端会通过 Zeabur 私有网络访问它。

`ZBPACK_DOCKERFILE_NAME` 的值必须写 `server`，不能写成 `Dockerfile.server`。

首次预览部署必须暂时保留：

```dotenv
ENVIRONMENT=development
```

在此模式下验证码固定为 `123456`，并由接口返回给登录页面。它只适合限制访问的功能验收，任何人都可以为任意格式正确的手机号请求并使用这个验证码，不可作为公开生产配置。

当前仓库尚未接入真实短信服务商。直接改成 `ENVIRONMENT=production` 后，服务虽然会生成随机验证码，但不会把验证码发送给用户，所有新登录都会失败。接入短信发送、频率限制和共享会话存储后，才能切换生产模式。

## 4. 部署玩家端

1. 将同一个更新后的仓库目录再次上传，创建第二个源码服务。
2. 将服务名设为小写的 `web`。
3. 在服务变量中录入 [web.env.example](deploy/zeabur/web.env.example) 的内容。
4. 确认构建计划使用 `Dockerfile.web` 后部署。
5. 将健康检查路径设为 `/`。
6. 在 Networking 中为 `web` 生成 Zeabur 域名或绑定自己的 HTTPS 域名。

关键变量是：

```dotenv
API_UPSTREAM=http://${SERVER_HOST}:8080
```

浏览器只访问 `web` 的公开域名。Nginx 会把 REST 和 WebSocket 请求转发到私有 `server`，因此不需要公开 API 域名，也不需要配置跨域来源。

## 5. 部署运营端

1. 将同一个更新后的仓库目录第三次上传，创建第三个源码服务。
2. 将服务名设为小写的 `admin`。
3. 在服务变量中录入 [admin.env.example](deploy/zeabur/admin.env.example) 的内容。
4. 确认构建计划使用 `Dockerfile.admin` 后部署。
5. 将健康检查路径设为 `/`。
6. 为 `admin` 绑定与玩家端不同的 HTTPS 域名。

运营端首次打开时会要求手机号验证码登录。只有 `ADMIN_PHONES` 中的手机号能进入控制台；其他手机号即使通过验证码，也会停留在“没有运营权限”页面。玩家端和运营端域名不同，所以两边需要分别登录一次。

## 6. 配置语音

首次部署可以把三个 `LIVEKIT_*` 变量留空。语音不可用不会阻止牌局继续，便于先验证身份、房间、积分和实时牌局。

需要语音时，建议创建 LiveKit Cloud 项目，并在 `server` 服务中设置：

```dotenv
LIVEKIT_URL=wss://你的-livekit-cloud-地址
LIVEKIT_API_KEY=你的-api-key
LIVEKIT_API_SECRET=你的-api-secret
```

不要把真实密钥写进 Git。自托管 LiveKit 需要额外处理 WebSocket 信令、RTC TCP/UDP、外部 IP、TURN 和媒体端口范围，不适合作为这次 Zeabur 首发部署的一部分。

## 7. 验收顺序

按以下顺序验证，出现问题时更容易定位到具体服务：

1. `server` 日志显示 PostgreSQL migration 成功，并开始监听 Zeabur 注入的端口。
2. `https://玩家域名/api/v1/ready` 返回 `{"status":"ready"...}`。
3. 玩家域名能打开邀请/大厅页面，使用预览验证码 `123456` 完成登录。
4. 创建房间后，第二个浏览器会话可以加入；行动和断线重连正常。
5. 运营域名能使用 `ADMIN_PHONES` 中的号码登录，并看到用户、房间、举报和审计数据。
6. 执行全站积分重置前填写原因并完成二次确认；活跃牌局不应中断。
7. 配置 LiveKit Cloud 后，再验证麦克风授权、设备切换、房主禁言和语音断线降级。

## 变量检查表

| 服务     | 必填变量                                                                                            | 可选变量                                               |
| -------- | --------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| `server` | `ZBPACK_DOCKERFILE_NAME`、`ENVIRONMENT`、`DATABASE_URL`、`REDIS_URL`、`INSTANCE_ID`、`ADMIN_PHONES` | `LIVEKIT_URL`、`LIVEKIT_API_KEY`、`LIVEKIT_API_SECRET` |
| `web`    | `ZBPACK_DOCKERFILE_NAME`、`API_UPSTREAM`                                                            | 无                                                     |
| `admin`  | `ZBPACK_DOCKERFILE_NAME`、`API_UPSTREAM`                                                            | 无                                                     |

不要提交真实手机号、数据库连接串、Redis 密码、LiveKit 密钥或正式域名。Zeabur 的 `${VARIABLE}` 引用应保留在服务变量中，让平台在运行时解析。

## 上线前仍需完成

- 接入真实短信服务商，并为验证码请求增加防滥用策略。
- 将登录 challenge、用户和 session 从单进程内存迁移到共享持久化存储；在此之前 `server` 只能运行一个实例，重启后用户需要重新登录。
- 为玩家域名配置访问控制或仅邀请范围，避免 development OTP 暴露给公网。
- 完成 ICP、隐私政策、用户协议、手机号和语音个人信息处理、未成年人策略及非博彩边界专项合规审查。
- 配置日志、备份、告警和 PostgreSQL 恢复演练，再开放长期使用。

## Zeabur 官方参考

- [使用 Dockerfile 部署](https://zeabur.com/docs/en-US/deploy/methods/dockerfile)
- [创建服务与本地上传](https://zeabur.com/docs/en-US/deploy/create/create-service)
- [环境变量与服务变量引用](https://zeabur.com/docs/en-US/deploy/config/environment-variables)
- [私有网络](https://zeabur.com/docs/en-US/deploy/networking/private-networking)
- [Monorepo 根目录配置](https://zeabur.com/docs/en-US/deploy/config/root-directory)
- [官方 PostgreSQL 模板](https://zeabur.com/templates/B20CX0)
- [官方 Redis 模板](https://zeabur.com/templates/KQZHXT)
