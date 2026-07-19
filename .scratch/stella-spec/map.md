# Stella spec 寻路地图

Labels: wayfinder:map

## Destination

一份可交付实施的《Stella 设计 spec v1》：架构、模块划分与全部关键决策（含理由与被否掉的备选）齐备，交给实施会话即可动工。本地图只产出决策与 spec，不写实现代码。

**状态：✅ spec 已评审通过并冻结（2026-07-19）→ [`spec.md`](spec.md)。可交付实施会话动工；后续变更须开新决策票。**

## Notes

- 领域：AI 智能体（QQ + HTTP API 双平台、工具、记忆）；术语以仓库根 `CONTEXT.md` 为准
- 每个会话默认使用 /grilling 与 /domain-modeling；research 票走 /research 子代理，prototype 票走 /prototype
- PI SDK 问题先查 pi-sdk 技能（`.pi/skills/pi-sdk/SKILL.md`）
- 决策、票与 spec 均以中文撰写

## Decisions so far

绘图会话中敲定的头条决策（理由与备选随 spec 记录）：

- 能力边界与权限模型 —— Stella 是带工具的干活 Agent；白名单工具全员可用（私聊/群聊同权），非白名单工具仅主人；主人 = 配置登记的 QQ 号 + API token
- 会话边界 —— QQ 私聊按用户一条会话；QQ 群聊按群一条（全群共享上下文）；API 由调用方以会话 id 指定，缺省新建
- 触发与倾听 —— 私聊逐条必回；群聊仅被 @ 时回应；未触发的群消息仍写入会话历史
- 长期记忆机制 —— 纯工具化：存取全走记忆工具，系统零提取、零注入；作用域限当前说话人；v1 无群记忆
- API 使命 —— 面向对话与未来 Web 管理端的完整后端：对话、会话管理、记忆管理全暴露；swag 自动文档
- [OneBot v11 / napcat 协议事实](issues/01-onebot-v11-napcat-facts.md) — 接入定反向 WS（Stella 内嵌 WS 服务端等 NapCat 反连、Bearer 鉴权、断线由 NapCat 按 5s 无限重连）；@ 判定 = at 段 `qq === String(self_id)`；回复用 `[at, text]` 段数组；message_id 为 LRU 短 ID（约 5000 条过期）
- [Bun HTTP 框架与自动文档选型事实](issues/02-bun-http-framework-openapi.md) — OpenAPI 自动程度 Elysia（schema 即文档、零注解）> Hono（手写 createRoute）> Bun.serve 原生；SSE 三家皆可（注意 Bun idleTimeout 默认 10s）；Elysia 在 `bun build --compile` 下有 3 个 open issue，Hono 无报告；文档 UI 前端默认走 CDN，离线单文件须自托管
- [PI SDK 深度行为事实](issues/03-pi-sdk-behaviors.md) — 多会话可行（每会话须独立 ResourceLoader，会话文件单持）；compaction 为总结式可关闭，`context` 事件可沿 user 边界实现 100K 硬裁剪（不动会话文件）；per-prompt 注入走 `before_agent_start`；工具门控 = 创建时白名单上限 + `setActiveToolsByName` / `tool_call` block 兜底；持久化 JSONL，`list → open` 可恢复一批会话
- [群聊消息格式](issues/05-group-message-format.md) — 注入：群消息入库标注 `[#id 群名片(QQ号) HH:MM] 内容`，触发注记走 `before_agent_start` 不入库，私聊不标注；输出：群回复自动 at 当前说话人 + 行首 `[reply:#id]` 引用 + 正文 `@QQ号` 提及；@全体成员不触发（仅倾听入库）
- [v1 工具集与白名单](issues/06-v1-toolset-whitelist.md) — 主人 = SDK 内置 7 件全开；全员白名单 = 仅记忆工具（内置只读工具暴露主人文件系统不进白名单；不引入网页搜索）；v1 自定义工具仅记忆工具；记忆工具全员开放、作用域限当前说话人
- [记忆工具设计](issues/07-memory-tool-design.md) — 4 工具（save 带 id 覆盖/search/list/delete）；存 bun:sqlite；条目挂 user_id；**用户模型** users(role: admin|guest) + user_identities 跨平台同一人识别（未命中自动建档、v1 API 提供合并端点）；作用域靠"schema 无 user 参数 + 会话闭包注入"强制；存取引导写工具侧（敏感凭据不存）
- [配置与部署](issues/08-config-deployment.md) — 单文件 TOML；模型 key 归 SDK（agentDir/auth.json），TOML 只配 Stella 自有概念；布局 = 部署目录内二进制 + config.toml + data/（agentDir：auth.json/models.json/sessions//memory.db），相对 CWD；NapCat 文档承诺到接口契约（配置片段+联调清单，安装运维不兜底）
- [会话运行时模型](issues/09-session-runtime-model.md) — 惰性唤起 + 常驻不回收（LRU 留演进）；注册表 sqlite `chat_sessions`，启动不批量恢复、消息驱动 open，`appendSessionInfo(name)` 兜底可扫描重建；API 与 QQ 同一套运行时，chat_key 命名空间区分
- [API 框架选型](issues/11-api-framework-choice.md) — Elysia + @elysia/openapi（schema 即文档零注解；SSE generator 自带取消）；四条写法纪律（不 minify / 不用 static 插件 / 不用 macro / 不用 fromTypes）；compile 实测为实施任务；决策可逆降级 Hono
- [短期记忆语义](issues/04-short-term-memory-semantics.md) — 硬截断（关 SDK compaction，`context` 事件沿 user turn 边界裁最旧消息，会话文件留全量）；计数口径 = SDK `getContextUsage()`（只计会话历史，系统提示词不占额）；双重淘汰：token 上限可配置默认 10K（下限约束 = 最大单轮工具体量）+ 消息年龄 3 天 TTL，每次调用幂等重裁；被动倾听消息与正式轮一视同仁按时间裁剪

## Not yet specified（spec §10 已收录）

- API 错误模型与限流具体策略——实施中成形后补记
- Stella 的人格与系统提示词正文——结构已定（spec §4），文案待调
- napcat 断连补偿——策略已定（spec §3.1/§10），实施期验收

## Out of scope

- 检索式记忆召回（embedding 等）：v1 记忆量小用不上，属 v1 之后的演进路径
- 多模型切换与负载均衡：v1 配置单一模型
