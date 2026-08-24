# 用户行为与边界测试矩阵

更新时间：2026-08-25

本矩阵把可操作角色、状态、输入边界和并发方式拆成有限的等价类。只有下列自动化套件全部通过，版本才允许进入 `main`。随机网络中断、数据库失败和浏览器权限拒绝以可重复故障注入覆盖，不以一次手工点击代替。

## 玩家账号与导航

| 场景 | 正常路径 | 失败、并发与恢复路径 | 自动化证据 |
| --- | --- | --- | --- |
| 匿名访问 | 浏览大厅；登录后创建或加入 | 未登录访问创建/邀请；安全重定向；后端离线 | `e2e/player.spec.ts`、`internal/httpapi/server_test.go` |
| 注册与登录 | 手机号、密码、昵称注册；密码登录；跨设备恢复 | 手机号/密码/昵称边界；重复手机号；错误密码；无权限；封禁；会话过期 | `internal/auth/service_test.go`、`internal/httpapi/server_test.go` |
| 资料与会话 | 改昵称；账本；退出登录 | 账本局部失败保留会话；退出立即关闭房间和语音连接；多端会话 | `internal/httpapi/server_test.go`、`e2e/player.spec.ts` |
| 路由恢复 | 当前等候室/牌桌刷新恢复 | 陈旧房间 URL；被移出；房间结束；跨标签切房；提交中刷新 | `e2e/player.spec.ts`、`e2e/realtime.spec.ts` |

## 房间生命周期与多人行为

| 状态 | 行为集合 | 必测边界 | 自动化证据 |
| --- | --- | --- | --- |
| 创建 | 配置人数、盲注、行动时间、语音、筹码 | 非法配置；创建事务失败不遗留房间/房主席位 | `internal/room/config_test.go`、`internal/room/actor_test.go` |
| 加入 | 空位加入；邀请码加入；空房接任房主 | 满桌/占座；重复加入；入座后立即重启；入座事务失败回滚 | `internal/httpapi/multiuser_test.go`、`internal/room/actor_test.go` |
| 切换房间 | 等待状态从 A 直接进入 B | B 满员/占座保留 A；手牌中拒绝；多连接清理；重试成功 | `internal/httpapi/multiuser_test.go`、`e2e/player.spec.ts` |
| 等候室 | 准备/取消；房主开局；主动离开 | 快速双击；提交中刷新；断线玩家不计准备；非房主开局；至少两人；离开失败留在原房 | `internal/room/actor_test.go`、`e2e/player.spec.ts` |
| 房主管理 | 禁言；移出；转移房主；轮换邀请；结束房间 | 非房主拒绝；不能移出自己；旧邀请失效；持久化失败不提前改变映射；操作幂等 | `internal/room/actor_test.go`、`apps/web/src/components/RoomControls.test.ts` |
| 断线/刷新 | 多标签连接计数；断线保留；重连 | 最后标签断开；保留期超时；落库失败回滚；服务重启统一标记断线 | `internal/room/actor_test.go`、`internal/httpapi/server_test.go` |
| 离桌/结算 | 等候离开；手牌后离开；移出；结束房间 | 结算失败保留座位；部分结算重试且恰好一次；离桌后重启不复活；空房超时 | `internal/room/actor_test.go` |

## 牌局、积分与消息

| 行为 | 正常与边界 | 自动化证据 |
| --- | --- | --- |
| 开局与发牌 | 仅准备且在线玩家；两人盲注与行动顺序；私牌不广播 | `internal/poker/game_test.go`、`internal/room/actor_test.go` |
| 行动 | 跟注、过牌、弃牌、加注、全下；筹码组合；最小/最大加注 | `internal/room/actor_test.go`、`internal/poker/game_test.go`、`ChipComposer.test.ts` |
| 冲突与重复 | 过期版本拒绝；同请求幂等；快速双击只发一次；未知命令拒绝 | `internal/room/actor_test.go`、`e2e/player.spec.ts` |
| 超时 | 有待跟注自动弃牌；无需跟注自动过牌；房间活动不延长行动时限 | `internal/poker/game_test.go`、`internal/room/actor_test.go` |
| 摊牌与边池 | 所有牌型；边池；短码全下重开规则；奇数筹码分配 | `internal/poker/game_test.go` |
| 局外积分 | 正整数边界；限流；账本；房间广播 | `internal/score` 测试、`internal/httpapi/server_test.go`、`ScoreAddPanel.test.ts` |
| 补充牌桌分 | 仅零筹码且非手牌中；失败回滚；重试只累计一次 | `internal/room/actor_test.go`、`internal/poker/game_test.go` |
| 消息与举报 | 固定快捷消息；非法文本拒绝；举报必填/幂等 | `internal/room/actor_test.go`、`ReportPanel.test.ts`、`internal/httpapi/server_test.go` |
| 全站重置 | 原因和确认短语；幂等；房间广播跨重启保留 | `e2e/admin.spec.ts`、`internal/room/actor_test.go` |

## 语音与实时连接

| 场景 | 必测结果 | 自动化证据 |
| --- | --- | --- |
| 房间未开启/服务未配置 | 不阻塞牌局；展示明确不可用状态 | `e2e/support/api-mocks.ts`、玩家浏览器套件 |
| 令牌与成员资格 | 仅已登录且在座用户获得；离房/移出/登出立即撤销 | `internal/httpapi/server_test.go` |
| WebRTC 信令 | 两端 offer/answer/ICE；离线目标；背压 | `internal/httpapi/server_test.go`、`voice_websocket_test.go` |
| 多标签 | 同用户语音连接替换；普通房间连接按数量计算 | `apps/web/src/lib/api.test.ts`、`internal/room/actor_test.go` |
| 连接角色 | 每对用户只有一个 polite peer；重连保持角色稳定 | `apps/web/src/lib/voice.test.ts` |
| 房主禁言 | 非房主拒绝；目标不存在拒绝；快照同步禁言状态 | `internal/room/actor_test.go` |

## 运营、安全与界面边界

| 场景 | 必测结果 | 自动化证据 |
| --- | --- | --- |
| 运营登录 | 匿名门禁；错误凭据；无权限；授权后加载真实数据 | `App.auth.test.ts`、`internal/httpapi/server_test.go`、`e2e/admin.spec.ts` |
| 用户/举报/审计 | 搜索；封禁/解封；解决/驳回；永久审计 | `apps/admin/src/api.test.ts`、`e2e/admin.spec.ts`、`internal/httpapi/server_test.go` |
| 异步竞态 | 搜索 A→B、详情 A→B、关闭未完成详情均以最后操作为准 | `e2e/admin.spec.ts` |
| 故障降级 | 部分接口失败保留其他数据；内部 SQL 错误不暴露 | `apps/admin/src/App.test.ts`、`internal/httpapi/server_test.go` |
| 剪贴板 | 成功才显示已复制；权限拒绝提供手动方案 | `e2e/player.spec.ts` |
| 可访问与视口 | 键盘表单；语义标签；桌面与横/竖屏完整布局；视觉基线 | Playwright 四视口项目、`e2e/player.spec.ts`、`e2e/admin.spec.ts` |
| 主题与尺度 | 黑曜/象牙/午夜主题切换与刷新恢复；浏览器配色同步；控件尺寸；整页无横向溢出 | `theme.test.ts`、`e2e/theme.spec.ts`、`docs/design/theme-system.md` |
| 契约与容量 | OpenAPI/WS 生成物一致；16 连接/4 房间容量冒烟 | `npm run contracts:check`、CI `capacity-smoke` |

## 合并门禁

```text
go test ./...
go vet ./...
npm test
npm run typecheck
npm run build
npm run contracts:check
npm run test:e2e
CI: go test -race ./...
CI: capacity-smoke
CI: Docker image build (server/web/admin)
部署后: npm run smoke:production
```

本地开发不要求启动 Docker；镜像构建由 GitHub Actions 门禁执行。生产发布只有在 required checks 全绿后通过 PR 合并到 `main`，Zeabur 随后自动部署并执行生产冒烟。
