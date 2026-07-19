# PI SDK 深度行为事实

Type: research
Status: resolved

## Question

Stella 基于 `@earendil-works/pi-coding-agent` 的 PI SDK 构建（仓库已装依赖，源码在 `node_modules`，另有 `.docs/resource/SDK · Docs · Pi.html` 与 pi-sdk 技能可查）。以下行为事实决定多个下游设计：

1. 多会话并发：单进程内同时常驻多个 AgentSession（每 QQ 用户/群一个）是否可行？有无共享状态、事件串扰、资源代价的坑？源码里 SessionManager / AgentSession 的边界如何？
2. 上下文窗口与 compaction：SDK 如何计数 token、何时触发 compaction？compaction 是总结式还是丢弃式？settingsManager 的 compaction 配置项有哪些？能否实现"到达 100K 时硬丢弃最旧消息"（关闭自动 compaction 后自行裁剪 messages 是否可行）？
3. 每轮注入：在 `prompt()` 之前动态修改当轮系统提示词/上下文的可行挂载点（群聊场景需要标注"当前说话人"）：systemPromptOverride 是启动时一次性的还是每轮可变？有没有 per-prompt 的上下文注入手段（PromptOptions、扩展事件等）？
4. 自定义工具与权限门控：defineTool 的全部能力；能否在运行时按"当前用户身份"动态启用/禁用工具（工具白名单随调用者变化）？tools 白名单是创建时固定的还是可变的？
5. 会话持久化：`SessionManager.create` 的文件格式（jsonl）与跨进程恢复语义；进程重启后恢复一批会话的正确做法。

事实须引用一手来源（`node_modules` 内的 SDK 源码/类型定义、官方文档 HTML）。

## Answer

- 多会话可行：`Agent`/`AgentSession`/`ExtensionRunner` 均为实例级、无全局事件总线；但每会话必须独立 `ResourceLoader`（共享会导致扩展 `pi.*` 动作被 bindCore 路由到最后绑定的会话），且 `session.reload()` 会进程级 `resetApiProviders()`。
- Compaction 为总结式（调 LLM），阈值 `contextTokens > contextWindow - reserveTokens`（默认 16384），配置仅 `compaction.{enabled,reserveTokens,keepRecentTokens}`；100K 硬丢弃可行：关 `enabled` 后用 `context` 扩展事件（每次 LLM 调用前触发）沿 turn 边界裁剪——只影响发给 LLM 的消息，不动 session 文件；`convertToLlm` 不修复 tool 配对，必须在 user 消息边界切。
- 每轮注入：`systemPromptOverride` 是 reload 级一次性；per-prompt 用 `before_agent_start`（可替换当轮 systemPrompt/注入消息，轮后自动还原）或 `input` 事件 transform 文本（入库）。
- 工具门控：`tools` 白名单是创建时硬上限（不进注册表），运行时 `setActiveToolsByName` 只能在上限内切换、下轮生效；`tool_call` 事件可 block 兜底；customTool `execute` 第 5 参为会话 ExtensionContext，无调用者身份需闭包自维护。
- 持久化：JSONL（header v3 + id/parentId 树），默认目录 `~/.pi/agent/sessions/--<编码cwd>--/`，文件名 `<时间戳>_<id>.jsonl`；恢复一批会话 = `SessionManager.list` → `open` → `createAgentSession`（自动恢复消息/模型）；文件无锁，同文件单持。

成果文件：[../research/03-pi-sdk-behaviors.md](../research/03-pi-sdk-behaviors.md)
