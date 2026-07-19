# 会话运行时模型

Type: grilling
Status: closed (2026-07-19)
Blocked by: 03

## Question

会话边界已定（私聊按用户、群聊按群、API 按会话 id），本票定运行时的承载方式：

- 单进程内"每会话一个常驻 AgentSession"还是"消息到达时按需唤起 + 用完回收"？
- 空闲会话的回收策略（内存占用 vs 唤起延迟）。
- 进程重启后会话如何恢复（SessionManager 持久化的利用方式）。
- API 平台创建的会话与 QQ 会话是否同一套运行时。

依赖 [PI SDK 深度行为事实] 中多会话并发与持久化恢复的事实。

## Resolution

1. **惰性唤起 + 唤起后常驻，v1 不主动回收**：消息到达才 `SessionManager.open` + `createAgentSession`（避免冷启动内存尖峰）；唤起后入内存注册表常驻；演进路径 = LRU 驱逐（dispose 后映射仍在 sqlite，再唤起只是重开文件）。
2. **重启恢复**：映射注册表存 sqlite（`chat_sessions(platform, chat_key, session_path, created_at, last_active)`，chat_key = `qq:private:*` / `qq:group:*` / `api:*`）；启动不批量恢复，消息驱动惰性 open；创建会话时把 chat_key 经 `appendSessionInfo(name)` 写入会话文件，注册表损毁可 `SessionManager.list` 扫描重建。
3. **API 与 QQ 同一套运行时**：同一内存注册表/惰性唤起/sqlite 映射，chat_key 命名空间区分；差异仅在平台适配层。运行时行为全平台一致（工具门控、记忆作用域、10K+3 天裁剪、关闭 compaction）；同一用户跨平台记忆贯通（票 07），会话本身仍按平台隔离。
