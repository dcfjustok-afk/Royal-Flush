# Royal Flush 接手说明

更新时间：2026 年 8 月 24 日

## 项目与部署

- Repository: `https://github.com/dcfjustok-afk/Royal-Flush`
- Zeabur Project ID: `6a8bfce9b1c569569969b2b5`
- Environment ID: `6a8bfce9da286b97dff2fd97`
- Region: Tencent Cloud Singapore
- Server: 2C / 4GB

服务：

- PostgreSQL: `service-6a8c12dcb1c569569969be31`
- Redis: `service-6a8c1330b1c569569969be5b`
- Server: `service-6a8c13ecb1c569569969bee8`
- Web: `service-6a8c1516b1c569569969bf88`
- Admin: `service-6a8c1f73acc1be0073702c98`

访问地址：

- 玩家端：[royal-flush.zeabur.app](https://royal-flush.zeabur.app/)
- 运营端：[royal-flush-admin.zeabur.app](https://royal-flush-admin.zeabur.app/)
- 就绪接口：[royal-flush.zeabur.app/api/v1/ready](https://royal-flush.zeabur.app/api/v1/ready)

## CI/CD

流程：

```text
feature branch
-> Pull Request
-> GitHub Actions required checks
-> merge main
-> Zeabur GitHub App
-> Watch Paths
-> 自动重建受影响服务
```

- `main` 受保护，不能直接 push，必须通过 PR。
- Ruleset: `main deployment gate`
- Zeabur GitHub App Installation ID: `156156046`
- GitHub App 仅授权仓库 `dcfjustok-afk/Royal-Flush`。
- 当前配置文件：`ZEABUR.md`、`deploy/zeabur/watch-paths/server.txt`、`deploy/zeabur/watch-paths/web.txt`、`deploy/zeabur/watch-paths/admin.txt`。

## 运行环境变量

`server`：

```dotenv
ZBPACK_DOCKERFILE_NAME=server
ENVIRONMENT=development
DATABASE_URL=${POSTGRES_CONNECTION_STRING}
REDIS_URL=${REDIS_CONNECTION_STRING}
INSTANCE_ID=${ZEABUR_SERVICE_ID}
ADMIN_ACCOUNT=由 Zeabur Secret 注入
ADMIN_PASSWORD=由 Zeabur Secret 注入
LIVEKIT_URL=
LIVEKIT_API_KEY=
LIVEKIT_API_SECRET=
```

`web`：

```dotenv
ZBPACK_DOCKERFILE_NAME=web
API_UPSTREAM=http://${SERVER_HOST}:8080
```

`admin`：

```dotenv
ZBPACK_DOCKERFILE_NAME=admin
API_UPSTREAM=http://${SERVER_HOST}:8080
```

管理员登录已经改为账号密码模式：运营端调用 `/api/v1/auth/password/login`，服务端从 `ADMIN_ACCOUNT`、`ADMIN_PASSWORD` 校验，成功后签发 `rf_session`。真实密码只应存在 Zeabur Secret，不要提交到仓库或写入文档。

## 当前代码状态

已完成：

- 玩家端运行时演示房间、演示用户、演示积分、演示账本、演示消息、演示举报和演示审计数据已移除。
- 空数据库会显示真实空状态；未连接后端时不会伪造创建房间、加分、举报、快捷消息或房主管理成功。
- 玩家端测试已改为显式权威快照和 API/mock 命令。
- 运营端空状态不再展示假房间、假用户、假举报或假审计。
- 运营端封禁、举报处理、房间详情在服务未配置时不会本地伪造结果。
- 新增管理员账号密码登录后端接口、前端登录页、OpenAPI 路径和测试。

未完成或需要上线前确认：

- Zeabur `server` 服务需要设置 `ADMIN_ACCOUNT` 和 `ADMIN_PASSWORD`；本仓库不包含真实密码。
- `ENVIRONMENT=development` 仍需在正式环境改为 production，并接入真实短信前再开放玩家手机号登录。
- `LIVEKIT_*` 当前为空，语音仍是可选降级状态。
- 本地环境未安装 Go，当前会话无法运行 `gofmt`、Go tests 和 Go contract generator；CI/Zeabur 构建环境应执行完整 Go 检查。
- 尚未执行线上 PostgreSQL 业务数据清理；清理前必须先只读统计业务表，再精确删除测试记录，不删除 schema、migration、服务或卷。
- 尚未 push 当前本地提交，也未触发 Zeabur 部署。

## 原子提交建议

按模块拆分本地提交：

1. `fix(web): 移除运行时演示数据`
2. `feat(admin): 增加管理员账号密码登录`
3. `docs: 增加项目部署与接手说明`

用户明确要求每个小模块原子化提交；除非另有明确要求，不执行 push。
