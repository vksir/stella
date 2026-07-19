# PI SDK 深度行为事实（票 03）

- 研究对象：`@earendil-works/pi-coding-agent` **0.80.9**（`node_modules/@earendil-works/pi-coding-agent/package.json`）
- 一手来源：`node_modules` 内 dist 源码与 `.d.ts`、官方文档 `/.docs/resource/SDK · Docs · Pi.html`、包内 `examples/`
- 标注约定：【源码】= dist 源码/类型定义确证；【文档】= 官方 HTML；【推断】= 基于源码的推理，未经实测
- 路径缩写：`PKG = node_modules/@earendil-works/pi-coding-agent`，`AI = node_modules/@earendil-works/pi-ai`，`CORE = node_modules/@earendil-works/pi-agent-core`

## 0. 前置校正：蒸馏技能与 0.80.9 源码的偏差

以源码为准，`.pi/skills/pi-sdk/SKILL.md` 有以下过时之处：

1. 技能的快速开始传 `authStorage` / `modelRegistry` 给 `createAgentSession`，但 0.80.9 的 `CreateAgentSessionOptions` **没有这两个字段**（`PKG/dist/core/sdk.d.ts`，接口仅含 `cwd/agentDir/modelRuntime/model/thinkingLevel/scopedModels/noTools/tools/excludeTools/customTools/resourceLoader/sessionManager/settingsManager/sessionStartEvent`）。`AuthStorage` 类也未从包根导出（`PKG/dist/index.d.ts:4` 只导出 `readStoredCredential`）。0.80.9 的认证/模型入口是 `ModelRuntime.create({ authPath, modelsPath })`（`PKG/dist/core/model-runtime.d.ts`；`PKG/dist/core/sdk.js:71-72` 默认按 `agentDir/auth.json`、`agentDir/models.json` 创建）。
2. 技能称 `thinkingLevel` 默认 `"off"`；源码 `DEFAULT_THINKING_LEVEL = "medium"`（`PKG/dist/core/defaults.js:1`），sdk.d.ts 注释亦为 "from settings, else 'medium'"。
3. 技能称内置工具含 `read/bash/edit/write/grep/find/ls` 且默认 `["read","bash","edit","write"]`，与源码一致（`PKG/dist/core/sdk.js:127` `defaultActiveToolNames`；文档 HTML "Tools" 节）。

---

## 1. 多会话并发：单进程常驻多个 AgentSession

### 1.1 结论（推断）

**可行，且 SDK 的实例边界基本干净**：`Agent`、`AgentSession`、`ExtensionRunner`、`SessionManager` 均为实例级状态，无全局事件总线。但有四个必须绕开的坑（§1.4）。

### 1.2 实例边界（源码确证）

- 每次 `createAgentSession()` 新建一个 `Agent`（`PKG/dist/core/sdk.js:156`）与一个 `AgentSession`（`sdk.js:249`）。`Agent` 的 transcript、listeners、steering/followUp 队列全部是实例字段（`CORE/dist/agent.d.ts` `AgentOptions/Agent`；`CORE/dist/agent.js:113` `createMutableAgentState`）。
- 事件订阅挂在各自 `Agent` 实例上（`PKG/dist/core/agent-session.d.ts` `subscribe()` 注释 "Session persistence is handled internally"），不存在跨会话事件分发。`createEventBus()` 每次调用新建独立 `EventEmitter`（`PKG/dist/core/event-bus.js:2-15`）——事件不串扰的前提是**不共享同一个 bus**。
- `ModelRuntime`、`SettingsManager`、`SessionManager`、`ResourceLoader` 由调用方选择共享或独占：未传时 `createAgentSession` 各自新建（`sdk.js:69-75`）。
- `bash` 工具按会话创建时的 `cwd` spawn 子进程，不用 `process.chdir`（`PKG/dist/core/tools/bash.js:41-55, 114-115, 202-214`）——不同会话不同 cwd 互不影响。
- `session.dispose()` 按 `sessionId` 清理 pi-ai 层会话资源（`PKG/dist/core/agent-session.js:576` 调 `cleanupSessionResources(this.sessionId)`；`AI/dist/session-resources.js:8-21`，目前注册者是 codex websocket `AI/dist/api/openai-codex-responses.js:626`）——按 sessionId 键控，销毁 A 会话不波及 B 会话。

### 1.3 可安全共享的组件（源码确证）

- `AuthStorage`：文件后端 + `withLock` + 内存快照（`PKG/dist/core/auth-storage.js:142-181`），多读少写，适合全进程共享一个实例（经 `ModelRuntime.create({ authPath })` 或直接共享 `ModelRuntime`）。
- `ModelRuntime`：持有 provider/model 组合与凭据引用，设计上是进程级服务（`model-runtime.d.ts`）。注意 `registerProvider/unregisterProvider` 是实例方法——**共享一个 ModelRuntime 时，任一会话的扩展注册 provider 会影响所有会话**（推断）。

### 1.4 坑（源码确证 + 推断）

1. **切勿跨会话共享同一个 `DefaultResourceLoader`（及其扩展实例）**。【源码】`AgentSession._buildRuntime` 复用 `resourceLoader.getExtensions().runtime`，而 `ExtensionRunner.bindCore()` 会把 `sendMessage/setActiveTools/setModel/...` 等动作方法**写进这个共享 runtime 对象**（`PKG/dist/core/extensions/runner.js:157-172`，注释原文 "Copy actions into the shared runtime (all extension APIs reference this)"；runtime 创建于 `loader.js:131 createExtensionRuntime`）。【推断】两个会话共用一个 loader 时，扩展闭包里的 `pi.*` 动作会路由到**最后绑定的那个会话**（sendMessage 串会话、setActiveTools 改错对象），且 `pi.events` bus 也共享。Stella 必须为每个会话构造独立的 `DefaultResourceLoader`（各自 `extensionFactories`），或自写 `ResourceLoader` 保证每会话独立 runtime。
2. **API provider 注册表是 pi-ai 模块级全局**。【源码】`AI/dist/compat.js:134-139`：模块加载即 `registerBuiltInApiProviders()`；`resetApiProviders()` 全局清空重建。`AgentSession.reload()` 会调 `resetApiProviders()`（`agent-session.js:2055`）。【推断】某一会话 reload 会在进程级重置 provider 注册表；若 Stella 通过扩展 `pi.registerProvider` 注册了自定义 provider，需确保重注册路径可靠（ModelRuntime 层是实例级的，但 compat 流函数解析走全局表）。
3. **文件写串行化队列是进程级单例**（有益而非 bug）。【源码】`PKG/dist/core/tools/file-mutation-queue.js:3-4`：`fileMutationQueues = new Map()` 模块级。多个会话同时 edit/write 同一文件会被串行化，不同文件并行。
4. **会话文件无文件锁**。【源码】`SessionManager._persist` 直接 `appendFileSync`（`PKG/dist/core/session-manager.js:663-691`），对比 settings 用 proper-lockfile（`settings-manager.d.ts` `FileSettingsStorage`）。【推断】同一 `.jsonl` 只能由一个 `SessionManager` 持有；恢复会话时要防止同文件被打开两次（同进程或跨进程）。
5. **资源代价**（推断）：每会话持有完整 transcript（`agent.state.messages` + `SessionManager.fileEntries` 双份）、独立的 ResourceLoader 磁盘扫描与 jiti 扩展模块加载。N 个 QQ 会话 ≈ N 份上述内存；LLM 并发受共享 API key 的速率限制约束。
6. **共享文件型 SettingsManager 的串扰**（推断）：`setModel` 等 setter 会写全局 settings（`settings-manager.d.ts` 注释 "Setters modify global settings by default"），多会话共用一个文件型 SettingsManager 时 A 会话切模型会污染 B 会话的默认值。建议 Stella 每会话 `SettingsManager.inMemory(...)`。

---

## 2. 上下文窗口与 compaction

### 2.1 token 计数（源码确证）

- 有 usage 时：`calculateContextTokens(usage) = usage.totalTokens || (input + output + cacheRead + cacheWrite)`（`PKG/dist/core/compaction/compaction.js:62-64`）。数据来自最近一次 assistant 响应的 usage。
- 无 usage 时估算：`estimateTokens(message) = ceil(chars / 4)`（`compaction.js:165-195`，图片按 4800 chars 估），`estimateContextTokens` 用"最后一条有效 usage + 其后消息的 chars/4 估算"（`compaction.js:104-127`）。

### 2.2 触发时机与类型（源码确证）

- 触发检查 `_checkCompaction` 在两处调用：`agent_end` 事件处理后（`PKG/dist/core/agent-session.js:782`）与 `prompt()` 发送前（`agent-session.js:866-869`，用于捕获 aborted 响应）。
- 阈值条件：`shouldCompact = contextTokens > contextWindow - reserveTokens`（`compaction.js:137-141`），reason 为 `"threshold"`，不自动重试。
- 溢出恢复：assistant 消息被判定 context overflow（`AI/dist/utils/overflow.js:125-151 isContextOverflow`：错误模式匹配 / usage 超窗 / length-stop 三种）时，reason 为 `"overflow"`，**先 compact 再自动重试一次**（`agent-session.js:1535-1560`；失败一次后不再重试并报错）。
- 手动 `session.compact()` 不检查 `enabled`（`agent-session.js:1373-1393`），只受 `prepareCompaction` 的"足够大才 compact"约束。

### 2.3 compaction 是总结式（源码确证）

- `generateSummary()` 用当前模型调一次 LLM 生成结构化摘要（`PKG/dist/core/compaction/compaction.d.ts` `generateSummary`；系统提示词 `SUMMARIZATION_SYSTEM_PROMPT` 见 `compaction/utils.d.ts`），支持基于上一次摘要迭代更新（`previousSummary`）。
- 切点：`findCutPoint` 从最新往前累积约 `keepRecentTokens` 后切，**可在 user 或 assistant 处切、绝不在 toolResult 处切**，保证 toolCall/toolResult 配对完整（`compaction.d.ts` `findCutPoint` 注释）。
- 落盘：`sessionManager.appendCompaction(summary, firstKeptEntryId, tokensBefore, details)`（`agent-session.js:1678` 附近）；context 重建时旧消息被摘要替换（`session-manager.d.ts` `buildContextEntries` 注释）。
- 扩展可接管：`session_before_compact` 事件可 `cancel` 或直接返回自定义 `compaction: CompactionResult`（`PKG/dist/core/extensions/types.d.ts` `SessionBeforeCompactEvent/SessionBeforeCompactResult`；调用点 `agent-session.js:1636-1660`；官方示例 `PKG/examples/extensions/custom-compaction.ts`）。

### 2.4 settingsManager 的 compaction 配置全量（源码确证）

`Settings.compaction`（`PKG/dist/core/settings-manager.d.ts` `CompactionSettings`）：

| 键 | 类型 | 默认值 | 出处 |
|---|---|---|---|
| `compaction.enabled` | boolean | `true` | `settings-manager.js:509-511` |
| `compaction.reserveTokens` | number | `16384` | `settings-manager.js:520-522`；`compaction.js:51-55 DEFAULT_COMPACTION_SETTINGS` |
| `compaction.keepRecentTokens` | number | `20000` | `settings-manager.js:523-525`；同上 |

关联配置：`branchSummary.reserveTokens`（默认 16384）、`branchSummary.skipPrompt`（默认 false）（`settings-manager.d.ts BranchSummarySettings`；`settings-manager.js:533-537`），用于树导航分支摘要而非主 compaction。读法：`getCompactionSettings()` 一次取全（`settings-manager.js:526-532`）。

### 2.5 "到达 100K 硬丢弃最旧消息"的实现路径

【源码】关闭自动 compaction：`SettingsManager.inMemory({ compaction: { enabled: false } })` 或 `applyOverrides`——`_checkCompaction` 首行即返（`agent-session.js:1511-1513`）。注意：关闭后 overflow 恢复也不再发生，API 溢出错误会直接抛给调用方（推断）。

裁剪的三条可行路径：

1. **扩展 `context` 事件（推荐）**。【源码】每次 LLM 调用前（含工具循环中的每个续轮），`Agent.transformContext` 被调用（`CORE/dist/agent-loop.js:180-181`），SDK 将其接到扩展 `context` 事件（`PKG/dist/core/sdk.js:215-219` → `runner.emitContext`，`extensions/runner.js:702-727`）。handler 返回 `{ messages }` 即替换**发给 LLM 的消息**，不改 transcript 与 session 文件。天然实现"会话存全量、模型只看最近 100K"。代价：`emitContext` 每次调用前 `structuredClone` 全量消息（`runner.js:704`），长会话有 CPU/内存开销（推断）。
2. **直接改 `agent.state.messages`**。【源码】getter 返回内部活引用、setter 浅拷贝（`CORE/dist/agent.js:39-44`；`AgentSession.messages` 直通 `agent-session.js:659-661`）；内部先例：overflow 恢复时 `this.agent.state.messages = messages.slice(0, -1)`（`agent-session.js:1554-1557`）。session 文件不受影响（持久化在 `message_end` 时增量 append，见 §5）。【推断】可行但绕过 SDK 的不变量维护，切点必须自行保证在 turn 边界。
3. **伪 compaction**：保留 `enabled: true`，用 `session_before_compact` 返回自定义"摘要=空/丢弃"结果（推断：可行但语义hack，且 threshold 仍按 `contextWindow - reserveTokens` 触发而非 100K；要把 `reserveTokens` 调成 `contextWindow - 100000` 来对齐触发点）。

**配对完整性约束（源码确证）**：`convertToLlm` 对 user/assistant/toolResult 原样透传，**不修复孤儿 toolCall/toolResult**（`PKG/dist/core/messages.js` `convertToLlm`，case "user"/"assistant"/"toolResult" 直接 `return m`）。无论哪条路径，裁剪都必须沿"user 消息开始的 turn 边界"切（参照 `findCutPoint` 的约束：绝不在 toolResult 处切），否则 Anthropic 类 API 会因 tool_use/tool_result 不配对拒请求（推断）。

**观测当前 token 量**：`session.getContextUsage()` / `getSessionStats()`（`agent-session.d.ts`；`ContextUsage.tokens` 估算同上算法）可用于实现 100K 水位判断。

---

## 3. 每轮注入（群聊"当前说话人"标注）

### 3.1 `systemPromptOverride` 是启动/重载级，不是每轮（源码确证）

`DefaultResourceLoader` 在 `reload()` 时求值一次 `systemPromptOverride(base)` 并缓存进 `this.systemPrompt`（`PKG/dist/core/resource-loader.js:332-333`），`getSystemPrompt()` 只是读缓存（`resource-loader.js:177-179`）。`AgentSession._rebuildSystemPrompt` 在工具集变化/重载时取用（`agent-session.js:716-746`）。**不是每轮求值**。

### 3.2 per-prompt 挂载点（源码确证，按推荐度排序）

1. **`before_agent_start` 扩展事件**：每次 `prompt()` 通过预检后、agent loop 启动前触发（`agent-session.js:887-911`）。handler 返回：
   - `systemPrompt?: string` —— **替换当轮系统提示词**；多扩展链式（`types.d.ts BeforeAgentStartEventResult` 注释）；agent run 结束后自动还原为基础提示词（`agent-session.js:_runAgentPrompt` finally 块，`this._systemPromptOverride = undefined`，约 758-759 行）。
   - `message?: CustomMessage` —— 作为 custom 消息与当轮 user 消息一起注入（`agent-session.js:890-901`）。
   - 事件载荷含 `prompt / images / systemPrompt / systemPromptOptions`（`types.d.ts BeforeAgentStartEvent`）。官方示例：`PKG/examples/extensions/prompt-customizer.ts`。
2. **`input` 扩展事件**：`prompt()` 入口最早触发（`agent-session.js:816-817`），可返回 `{ action: "transform", text }` 改写当轮提示词文本（`runner.js:885-920`；`types.d.ts InputEvent/InputEventResult`）。【推断】这是标注"当前说话人"最直接的点：把 `"[群成员 张三(QQ:123)] 说：..."` 包装到用户文本头部。注意它会改变存入 session 的 user 消息原文（transform 后的文本入库）。
3. **`context` 事件**：每次 LLM 调用前可改消息列表（见 §2.5），【推断】也可用来在每轮 LLM 调用前临时注入/改写上下文（不入库）。
4. **`sendCustomMessage(..., { deliverAs: "nextTurn" })`**：排队的 custom 消息会随下一次 prompt 的 user 消息一起注入（`agent-session.js:881-886` `_pendingNextTurnMessages`；`agent-session.d.ts sendCustomMessage` 注释）。
5. `PromptOptions` 本身**没有**系统提示词/上下文注入字段，只有 `expandPromptTemplates/images/streamingBehavior/source/preflightResult`（`agent-session.d.ts PromptOptions`）。

### 3.3 扩展事件总线可用钩子全量（源码确证）

`ExtensionAPI.on` 支持 33 个事件（`PKG/dist/core/extensions/types.d.ts` `ExtensionAPI`）：
`project_trust`、`resources_discover`、`session_start`、`session_info_changed`、`session_before_switch`、`session_before_fork`、`session_before_compact`、`session_compact`、`session_shutdown`、`session_before_tree`、`session_tree`、`context`、`before_provider_request`、`before_provider_headers`、`after_provider_response`、`before_agent_start`、`agent_start`、`agent_end`、`agent_settled`、`turn_start`、`turn_end`、`message_start`、`message_update`、`message_end`、`tool_execution_start`、`tool_execution_update`、`tool_execution_end`、`model_select`、`thinking_level_select`、`tool_call`（可 block）、`tool_result`（可改写）、`user_bash`、`input`（可 transform）。

注册方式：自定义 `ResourceLoader`，或 `DefaultResourceLoader({ extensionFactories: [(pi) => { pi.on(...) } ] })`（`resource-loader.d.ts DefaultResourceLoaderOptions`）。

---

## 4. 工具门控

### 4.1 `defineTool` 完整能力（源码确证）

`defineTool` 是恒等函数，仅为保留类型推断（`PKG/dist/core/extensions/types.d.ts` `defineTool`）。`ToolDefinition` 字段全量（同文件）：

- `name / label / description`；`parameters`（TypeBox `TSchema`）
- `promptSnippet?`：入选系统提示词 "Available tools" 节的一行简介（缺省时自定义工具不出现在该节）
- `promptGuidelines?: string[]`：工具激活时追加到系统提示词 Guidelines 节
- `prepareArguments?`：schema 校验前的参数整形
- `executionMode?: "sequential" | "parallel"`：覆盖默认并发策略
- `renderShell?` / `renderCall?` / `renderResult?`：TUI 渲染（Stella 无 TUI 可忽略）
- **`execute(toolCallId, params, signal, onUpdate, ctx: ExtensionContext)`**：第 5 参 `ctx` 是**该会话的** ExtensionContext（`PKG/dist/core/extensions/wrapper.js:13` 用 `runner.createContext()`；`tools/tool-definition-wrapper.js:4-8` 透传）。`ctx` 含 `sessionManager`（只读，可 `getSessionId()`）、`model`、`cwd`、`isIdle()`、`signal`、`getContextUsage()` 等（`types.d.ts ExtensionContext`），**不含调用者身份**。

### 4.2 `tools` 白名单：创建时是硬上限，运行时可切换（源码确证）

- 创建时：`options.tools` 形成 `allowedToolNames`，`_refreshToolRegistry` 用它过滤注册表——**不在白名单的工具根本不进注册表**（`PKG/dist/core/agent-session.js:1941-1951`；`sdk.js:128-131`）。`excludeTools` 是其后再套的 denylist。
- 运行时：`session.setActiveToolsByName(names)`（公开方法，`agent-session.js:637-651`；注释 "Changes take effect on the next agent turn" 见 `agent-session.d.ts`）或扩展 `pi.setActiveTools(names)`（`agent-session.js:1885`），只能从注册表（即白名单子集）里启用，未知名被忽略；同时重建系统提示词。
- 运行时也可新增工具：扩展 `pi.registerTool(...)`（官方示例 `PKG/examples/extensions/dynamic-tools.ts`），但受 `allowedToolNames` 上限约束（`agent-session.js:1994-2000`：白名单存在时仅白名单内工具会被激活）。

### 4.3 按调用者身份动态门控的方案（推断，机制均为源码确证）

双层门控：

1. **会话创建时** `tools: [...全员可用白名单 + 主人专属工具...]` 定上限（防扩展/提示注入越权注册）。
2. **每轮/每人切换**：Stella 在派发 QQ 消息到 `session.prompt()` 前，按当前说话人调 `session.setActiveToolsByName(主人 ? 全量 : 仅白名单)`。说话人身份由 Stella 自己维护——SDK 无任何"调用者"概念；可在自定义工具 `execute` 内读闭包里的"当前说话人"变量（该变量由 Stella 在 prompt 前更新，或由 `input`/`before_agent_start` handler 更新）做执行期二次校验。
3. **执行级兜底**：扩展 `tool_call` 事件返回 `{ block: true, reason }`（`types.d.ts ToolCallEventResult`；官方示例 `PKG/examples/extensions/permission-gate.ts`），按当轮说话人做 per-call 拦截。`event.input` 还可原地修改参数。

【推断】方案 2（setActiveToolsByName）让非主人根本看不到专属工具的描述，最省 token 也最干净；方案 3 作为纵深防御。

---

## 5. 会话持久化

### 5.1 存储格式与路径规则（源码确证）

- **JSONL**：首行 `SessionHeader`：`{"type":"session","version":3,"id","timestamp","cwd","parentSession?"}`（`PKG/dist/core/session-manager.d.ts` `SessionHeader`；`CURRENT_SESSION_VERSION = 3`）。之后每行一个 `SessionEntry`，各含 `id/parentId/timestamp` 构成**树**（branch 靠移动 leaf 指针，历史不改写）。条目类型：`message / thinking_level_change / model_change / compaction / branch_summary / custom / custom_message / label / session_info`（`session-manager.d.ts SessionEntry`）。
- **文件命名**：`<sessionDir>/<fileTimestamp>_<sessionId>.jsonl`（`session-manager.js:606`）。
- **默认目录**：`<agentDir>/sessions/--<cwd 把 / \ : 全替换成 ->--/`，如 `~/.pi/agent/sessions/--D--Code-MyProject-stella--/`（`session-manager.js:242-247 getDefaultSessionDirPath`；Windows 盘符的 `:` 也被替换）。
- **写入语义**：append-only；**首个 assistant 消息出现前不落盘**（`session-manager.js:663-691 _persist`：无 assistant 时只在内存累积，首个 assistant 到达后一次性 `wx` 创建并全量写入，之后逐行 append）。
- 读取：整体加载 + 索引（`_buildIndex`，`session-manager.js:610-632`），leaf 默认为最后一条 entry。

### 5.2 create / open / continueRecent / list 语义（源码确证）

| 方法 | 语义 | 出处 |
|---|---|---|
| `SessionManager.create(cwd, sessionDir?, options?)` | 新建会话（立即生成 header；持久模式下首次 flush 时才建文件） | `session-manager.js:1113-1118` |
| `SessionManager.open(path, sessionDir?, cwdOverride?)` | 打开既有 jsonl；cwd 默认取 header.cwd，sessionDir 默认取文件父目录 | `session-manager.js:1123-1133` |
| `SessionManager.continueRecent(cwd, sessionDir?)` | 找目录里最新修改的会话打开，没有则新建 | `session-manager.js:1138-1146`、`findMostRecentSession:338` |
| `SessionManager.inMemory(cwd?)` | 纯内存不落盘 | `session-manager.js:1148-1150` |
| `SessionManager.list(cwd, sessionDir?, onProgress?)` | 列出该 cwd 默认目录全部会话，返回 `SessionInfo[]`（path/id/cwd/name/created/modified/messageCount/firstMessage/allMessagesText） | `session-manager.d.ts` |
| `SessionManager.listAll(...)` | 跨全部项目目录 | 同上 |
| `SessionManager.forkFrom(sourcePath, targetCwd, ...)` | 把别的项目的会话复制进当前项目 | `session-manager.js:1158+` |

### 5.3 进程重启后恢复一批会话的正确做法（源码确证 + 推断）

1. `const infos = await SessionManager.list(cwd, sessionDir?)` 拿到全部 `SessionInfo`（含 `path`）。Stella 需自己维护"QQ 会话 ↔ session 文件路径/会话 id"的映射（SessionInfo 里没有业务标识，可用 `appendSessionInfo(name)` 写入的 `name`，或 Stella 侧自建索引文件）。【推断】
2. 对每个要恢复的会话：`createAgentSession({ sessionManager: SessionManager.open(path), model, ... })`。SDK 自动 `buildSessionContext()` 恢复消息进 `agent.state.messages`（`PKG/dist/core/sdk.js:230-232`），并从 `model_change`/`thinking_level_change` 条目恢复模型与思考级别（`sdk.js:88-119`；模型恢复失败时返回 `modelFallbackMessage`）。
3. 约束：
   - **同一 jsonl 同时只能打开一次**（无文件锁，§1.4-4）。
   - `open` 恢复的 cwd 来自 header；Stella 多会话若共用 cwd 无影响，但跨机器/路径迁移时注意（可用 `cwdOverride`）。【推断】
   - 大批量恢复时每会话一个 ResourceLoader（§1.4-1），且惰性创建（收到首条消息再 open + createAgentSession）可省内存。【推断】

### 5.4 参考

- 文档 HTML "Session Management" 节给的 runtime 用法（`createAgentSessionRuntime` + `newSession/switchSession/fork`）是**单活跃会话**场景（交互式 /new、/resume），不适合 Stella "每 QQ 实体一条常驻会话"的多会话模型——Stella 应直接用 `createAgentSession` × N + 自建会话注册表。【推断】
- 会话树格式细节（分支、label、fork）在官方另有 "Session Format" 文档（HTML 导航中引用），本仓库未含该文件；`session-manager.d.ts` 的类型与注释已足够覆盖。

---

## 6. 对 Stella 设计的直接推论（全部为推断）

1. **多会话**：每 QQ 实体 = 一个 `createAgentSession`（独立 `SessionManager`、`SettingsManager.inMemory`、`DefaultResourceLoader`），共享一个 `ModelRuntime`（认证/模型目录）与可选 `AuthStorage`。自建 `Map<qqEntityId, AgentSession>` 注册表；`dispose()` 时从表移除。禁止共享 ResourceLoader。
2. **100K 硬丢弃**：`compaction.enabled=false` + 每会话注册一个 `context` 事件扩展：估算 token（可用导出的 `estimateContextTokens`/`estimateTokens`，`PKG/dist/index.d.ts:5` 有导出），超 100K 时沿 turn 边界裁掉最旧消息返回。session 文件保留全量历史，仅 LLM 视野被截断。同时把 `retry` 保持开启以吸收普通 API 错误（溢出错误此时已无自动恢复，需在 Stella 层 catch 后主动裁剪重试）。
3. **当前说话人标注**：`input` 事件 transform 在入库前改写文本（最省事），或 `before_agent_start` 返回 `systemPrompt`/`message`（不入库、每轮自动还原）。两者都可读会话级闭包里的"当前说话人"。
4. **工具门控**：创建时 `tools` 定硬上限（含主人工具），prompt 前 `setActiveToolsByName` 按身份切换；自定义工具的 `execute` 第 5 参 `ctx` + 闭包状态做执行期校验；`tool_call` 事件 block 兜底。
5. **持久化恢复**：启动时 `SessionManager.list` → 映射 QQ 实体 → 惰性 `SessionManager.open` + `createAgentSession`；同文件单持。
