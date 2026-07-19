# Stella 设计 spec v1

> 本 spec 是实施会话的动工依据：架构、模块划分与全部关键决策（含理由与被否掉的备选）。
> 决策来源：绘图会话头条决策 + 决策票 01–09、11（`.scratch/stella-spec/issues/`，各票 Resolution 节为细节一手来源）。
> 协议与 SDK 事实来源：研究票 01–03（`.scratch/stella-spec/research/`）。
> 术语以仓库根 `CONTEXT.md` 为准。

## 1. 架构总览

Stella 是单进程 Bun 应用，内嵌两个平台接入与一套会话运行时：

```
        QQ (NapCat)                        HTTP 客户端 / Web 管理端
            │                                      │
  反向 WS（NapCat 反连，Bearer 握手）      HTTP + SSE（Bearer 鉴权）
            │                                      │
   ┌────────┴─────────┐                  ┌─────────┴────────┐
   │ QQ 适配器         │                  │ API 层 (Elysia)  │
   │ ·事件解析/触发判定 │                  │ ·对话/会话/记忆端点 │
   │ ·注入编排/输出解析 │                  │ ·OpenAPI 自动文档  │
   └────────┬─────────┘                  └─────────┬────────┘
            │            消息派发（串行）            │
            ▼                                      ▼
   ┌──────────────────────────────────────────────────────┐
   │ 会话运行时                                            │
   │ ·注册表（内存 Map + sqlite chat_sessions）             │
   │ ·惰性唤起 / 常驻不回收 / 重启透明恢复                   │
   │ ·身份解析（平台身份 → 用户）                           │
   │ ·上下文裁剪扩展（10K token + 3 天 TTL，context 事件）  │
   │ ·每会话独立 AgentSession（SessionManager /            │
   │   SettingsManager.inMemory / ResourceLoader），        │
   │   全进程共享一个 ModelRuntime                          │
   └───────┬──────────────────────────────┬───────────────┘
           │ 工具面                        │ 数据面
           ▼                              ▼
   ┌───────────────┐            ┌──────────────────┐
   │ 权限门控       │            │ memory.db (sqlite)│
   │ ·创建时 tools  │            │ ·memories         │
   │  白名单上限     │            │ ·users/identities │
   │ ·按 role 切换   │            │ ·chat_sessions    │
   │  激活集 + 兜底  │            └──────────────────┘
   └───────────────┘
   工具集：主人 = SDK 内置 7 件全开；全员白名单 = 4 个记忆工具
```

模块划分：**平台适配层**（QQ 适配器、API 层）、**会话运行时**、**权限门控**、**记忆系统**、**配置**。
平台差异只存在于适配层；运行时行为（门控、记忆作用域、裁剪、compaction 关闭）全平台一致。

## 2. 头条决策（绘图会话敲定，理由在此记录）

### 2.1 能力边界与权限模型

**决策**：Stella 是带工具的干活 Agent；白名单工具全员可用（私聊/群聊同权），非白名单工具仅主人；主人 = 配置登记的 QQ 号 + API token。
**理由**：白名单按"构造就安全"划界（副作用封闭在调用者自身数据域内），安全边界不依赖提示词约束；主人单点高权限，管理面最小。
**被否备选**：逐平台分权（群聊权限 < 私聊）——同权更简单且群聊本就需白名单工具；多角色分级——v1 只需 admin/guest 两级。

### 2.2 会话边界

**决策**：QQ 私聊按用户一条会话；QQ 群聊按群一条（全群共享上下文）；API 由调用方以会话 id 指定，缺省新建。
**理由**：与 IM 产品的天然上下文边界一致——群成员在群里本就能看到彼此发言，共享上下文是特性不是泄露；私聊天然隔离。API 侧会话 id 交给调用方，与未来 Web 管理端的多标签对话天然契合。

### 2.3 触发与倾听

**决策**：私聊逐条必回；群聊仅被 @ 时回应；未触发的群消息仍写入会话历史（倾听）。
**理由**：群聊逐条回复会刷屏且成本爆炸，@ 是群里约定俗成的"叫bot"信号；倾听让 Stella 被 @ 时握有上文，回答能接得上话。

### 2.4 长期记忆机制

**决策**：纯工具化——存取全走记忆工具，系统零提取、零注入；作用域限当前说话人；v1 无群记忆。
**理由**：零提取/零注入意味着上下文里没有任何模型不可见的魔法，行为完全可预测、可调试；自动提取类方案（后台总结用户画像）在 v1 记忆量下收益低且引入不可控写路径。群记忆涉及归属权与泄露面（A 存的群记忆 B 可见？），v1 不碰。

### 2.5 API 使命

**决策**：API 是面向对话与未来 Web 管理端的完整后端：对话、会话管理、记忆管理全暴露；自动文档。
**理由**：API 不是 QQ 的附属品而是对等平台；管理端尚未开建，先把后端面定全，前端随时可接。

## 3. 平台适配层

### 3.1 QQ 适配器（napcat / OneBot v11）

**接入形态**（票 01 §6）：NapCat 配一个 `websocketClients` 指向 Stella（如 `ws://127.0.0.1:8082/onebot`），Stella 内嵌 WS 服务端；握手校验 `Authorization: Bearer <token>`、记录 `X-Self-ID`；`messagePostFormat: "array"`（结构化段数组，免 CQ 码转义歧义）。重连由 NapCat 侧按 `reconnectInterval`（默认 5000ms）无限重试，Stella 只处理接受连接与状态置位。连接建立即收 `meta_event.lifecycle/connect`，之后按 heartInterval 收心跳，驱动连接存活状态机。

**消息入口流水线**：按 `post_type` 分流——`message` 进消息管线；`meta_event`（lifecycle/heartbeat）驱动连接状态机；`message_sent`/`notice`/`request` v1 记录不处理。

**触发判定**（票 01 §3.3、票 05）：私聊逐条必回；群聊触发 = 存在 `at` 段且 `String(seg.data.qq) === String(event.self_id)`；`qq: "all"`（@全体成员）**不触发**，仅倾听入库。

**倾听与注入编排**（票 05）：

- 每条群消息（被动 + 触发）写入会话历史，标注格式：
  `[#消息id 群名片(QQ号) HH:MM] 内容`
  - 群名片优先于昵称；@Stella → 行内 `@Stella`；@别人 → `@昵称(QQ号)`；@全体 → `@全体成员`；图片/表情/语音等 → `[图片]`/`[表情]`/`[语音]` 占位；引用 → `[引用#id]`。
  - 机制：被动消息经 `sendCustomMessage` 入库（custom_message 条目，对模型呈现为 user 消息，随会话持久化与恢复）；触发消息走 `input` 事件 transform 或 prompt 文本包装入库。
- **触发注记**（不入库）：`【本轮触发】群名片(QQ号) 在 #消息id @ 了你（Stella）。你的回复会自动 @ 对方；行首写 [reply:#消息id] 可引用某条消息，正文里写 @QQ号 可提及他人。`——经 `before_agent_start` 当轮注入（当轮结束 SDK 自动还原）。
- **私聊不标注说话人**（会话按用户划分、身份写进系统提示词），历史即纯文本。
- 窗口代价：标注 ≈7 token/条，相对 10K 上限可忽略。

**输出解析与发送**（票 05）：

- 模型输出约定：行首 `[reply:#消息id]` → reply 段（主动引用）；正文 `@QQ号`（5+ 位数字）→ at 段（主动提及）；其余为 text 段。
- 群聊回复自动在开头 at 当前说话人（at 段 + 空格开头的 text 段，QQ 客户端惯例）；私聊不加。
- 经 `send_group_msg` / `send_private_msg` 段数组发送；WS 调用以 UUID `echo` 关联请求-响应。
- 降级：标记解析失败 → 纯文本发送；引用过期 message_id（NapCat LRU 短 ID 约 5000 条过期）报错 → 去掉 reply 段重发。

**断连韧性**（票 01 §6）：监听 WS close 置连接状态；断连窗口的消息空洞（NapCat 不缓存补发）在重连后用 `get_group_msg_history` / `get_friend_msg_history` 补偿拉取（注意 message_id LRU 时效；事件扩展字段 `message_seq`/`real_seq` 可作备用键）。

### 3.2 API 层（Elysia）

**框架**：Elysia + `@elysia/openapi`（票 11）。schema 即文档零注解出 UI 与 spec；SSE 用 generator + `sse()`（自带取消语义；注意 Bun 层 `idleTimeout` 默认 10s，长静默流需 `server.timeout(req, 0)` 或保活帧）。

**写法纪律**（规避 compile open issue）：不开 `--minify`（#1711）；不用 `@elysiajs/static`，静态资产走 `with { type: "file" }` + 手工路由（#1713）；不用 macro（#1280）；不用 `fromTypes`（或按官方指引预生成 `.d.ts`）。文档 UI（Scalar）自托管：内嵌资产 + `scalar.cdn` 指本地路径。

**端点面**（v1，细节随实施定形）：

| 面 | 端点（草） |
|---|---|
| 对话 | `POST /sessions`（新建，返回会话 id）；`POST /sessions/:id/messages`（发消息，SSE 流式回 token） |
| 会话管理 | `GET /sessions`、`GET /sessions/:id`（含历史）、`DELETE /sessions/:id` |
| 记忆管理 | `GET /users/:id/memories`、`PUT /memories/:id`、`DELETE /memories/:id` |
| 用户管理 | `GET /users`、用户合并 `POST /users/:id/merge`（源 user 的身份与记忆转移到目标后删源） |
| 文档 | `/openapi`（Scalar UI）、`/openapi/json` |

**鉴权**：`Authorization: Bearer <token>`；token ↔ 平台身份 `api` ↔ 用户（票 07 用户模型）；主人 token 即 admin。每条 API 请求天然是一次触发，会话 id 缺省新建。

## 4. 会话运行时（票 09、04）

**多会话模型**（票 03 §1、§6）：每个聊天实体 = 一个 `createAgentSession`，每会话独立 `SessionManager`、`SettingsManager.inMemory(...)`、`DefaultResourceLoader`（含各自 extensionFactories——**严禁跨会话共享 ResourceLoader**，扩展闭包会串会话）；全进程共享一个 `ModelRuntime`（认证/模型目录）。自建 `Map<chatKey, AgentSession>` 内存注册表；同一 jsonl 会话文件同时只打开一次（SDK 无文件锁）。

**唤起与常驻**：惰性唤起——消息到达才 `SessionManager.open(path)` + `createAgentSession`；唤起后常驻不主动回收（v1 会话规模小，内存可承受；演进路径 = LRU 驱逐）。消息派发按会话**串行化**（保证"当前说话人"闭包与轮次严格对应；SDK 对流式中的 prompt 本就走排队）。

**注册表与重启恢复**：映射存 sqlite 表 `chat_sessions(platform, chat_key, session_path, created_at, last_active)`，`chat_key` = `qq:private:<uin>` / `qq:group:<gid>` / `api:<sessionId>`。进程启动**不批量恢复**——消息驱动惰性 open，重启对会话透明。兜底：创建会话时把 `chat_key` 经 `appendSessionInfo(name)` 写入会话文件，注册表损毁可 `SessionManager.list` 扫描 `data/sessions/` 按 name 重建。

**短期记忆（上下文窗口）**（票 04）：

- **关闭 SDK compaction**（`SettingsManager.inMemory({ compaction: { enabled: false } })`）；注意关闭后 overflow 自动恢复也随之消失，Stella 层需 catch 溢出错误后主动裁剪重试。
- **双重淘汰**：每会话注册 `context` 事件扩展，每次 LLM 调用前（含工具循环每个续轮）幂等裁剪——token 口径用 SDK `getContextUsage()`（usage 优先、chars/4 兜底，只计会话历史，系统提示词与当轮注入不占额），超 `short_term_memory.max_tokens`（默认 10K）沿 **user 轮边界**丢最旧消息；消息年龄超 `max_age_days`（默认 3 天）同样淘汰。被动倾听消息与正式轮一视同仁按时间处理。
- **会话文件始终保留全量历史**（裁剪只动 LLM 视野），为未来检索式召回留素材。
- 下限约束：max_tokens 必须 > 最大单轮体量（含工具结果的轮可达 5–10K），配置校验时警告过低值。

**每轮注入挂载点**（票 03 §3）：触发注记走 `before_agent_start`（当轮注入、自动还原、不入库）；入库标注走 `input` transform / custom message；系统提示词为会话创建级（`systemPromptOverride`，按 chat 类型生成：群聊说明群信息与消息格式，私聊说明对方身份）。

**人格与系统提示词**：v1 系统提示词 = 人格段（从简，占位待细化）+ 平台/会话上下文段 + 记忆引导一句（"你拥有长期记忆工具，用于记住用户的重要信息"）+ SDK 自动追加的工具 Guidelines。存取规则不写正文，写工具侧（见 §5 记忆工具）。

## 5. 权限门控与工具集（票 06、03 §4）

**双层门控**：

1. **创建时上限**：`tools: [...白名单 + 主人专属...]` 定硬上限——不在名单的工具根本不进注册表，防提示注入越权注册。
2. **按身份切换**：派发消息到 `session.prompt()` 前，按当前说话人 role 调 `session.setActiveToolsByName(admin ? 全量 : 仅白名单)`——guest 根本看不到专属工具的描述，最省 token 也最干净。
3. **执行级兜底**：扩展 `tool_call` 事件按当轮说话人 block 越权调用（纵深防御）。

**工具集**：

| 集合 | 内容 |
|---|---|
| 主人（admin） | SDK 内置 7 件全开：`read` `bash` `edit` `write` `grep` `find` `ls` + 记忆工具 |
| 白名单（全员） | 仅 4 个记忆工具：`memory_save`（可选 id，带 id 覆盖更新）/ `memory_search` / `memory_list` / `memory_delete` |

**被否备选**：白名单放内置只读工具（read/grep/find/ls 暴露主人整机文件系统含密钥，不满足"按构造就安全"）；v1 引入网页搜索（外部依赖 + 计费 + 滥用面，演进时单独开研究票选型）；单工具多 action 的记忆工具（union 参数是模型误用重灾区）；独立 `memory_update`（save 带 id 已覆盖）。

## 6. 记忆系统与用户模型（票 07）

**存储**：`bun:sqlite`，库文件 `data/memory.db`（compile 兼容，事务写入）。被否备选：JSON 文件（整文件重写怕崩、并发需自行串行化，且管理走 API 层不需要肉眼直读）。

**表结构**：

```sql
-- 用户模型（跨平台同一人识别）
users(id INTEGER PRIMARY KEY, display_name TEXT, role TEXT NOT NULL,  -- admin | guest
      created_at INTEGER NOT NULL)
user_identities(user_id INTEGER NOT NULL REFERENCES users(id),
                platform TEXT NOT NULL,            -- 'qq' | 'api'（未来扩展）
                platform_user_id TEXT NOT NULL,    -- QQ 号 / token 名
                created_at INTEGER NOT NULL,
                UNIQUE(platform, platform_user_id))

-- 长期记忆
memories(id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id),
         content TEXT NOT NULL,                    -- ≤2000 字符
         created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
         source_session TEXT)                      -- 可空，管理追溯用

-- 会话注册表
chat_sessions(platform TEXT NOT NULL, chat_key TEXT NOT NULL,
              session_path TEXT NOT NULL, created_at INTEGER NOT NULL,
              last_active INTEGER NOT NULL, PRIMARY KEY(platform, chat_key))
```

**护栏**（均进配置）：每用户记忆 ≤200 条（超限拒存并提示先删）；单条 ≤2000 字符（超限拒绝）。

**身份解析**：消息到达 → 按 `(platform, platform_user_id)` 查 `user_identities` → 未命中**自动建档**（新建 user + identity，role=guest）。首次启动把配置的主人 QQ + API token 关联到同一 admin 用户。v1 仅一个 admin（配置登记的主人）；role 字段为多管理员留形。**用户合并**：v1 API 提供合并端点（管理操作）。

**作用域强制**：记忆工具 schema **无 user 参数**（模型无处越权）；user_id 只来自会话级闭包（派发前身份解析写入，工具 `execute` 读闭包过滤）；同会话消息串行派发保证闭包与轮次对应。

**存取引导**（写工具侧 `description` + `promptGuidelines`，系统提示词正文仅一句原则）：

- 该存：用户明确要求；稳定偏好（称呼、口味、作息）；长期事实（在做的项目、生日、宠物名）
- 不该存：一次性请求与闲聊；重复内容（先 `memory_search` 再 save）；**敏感凭据**（密码/密钥/token，用户要求也不存并说明原因）
- 该更新：信息变化时 `memory_save` 带 id 覆盖，不新增重复条目

## 7. 配置与部署（票 08）

**配置**：单文件 TOML（Bun 原生 `Bun.TOML.parse`，零依赖，支持注释）。

```toml
[owner]
qq = "123456789"              # 主人 QQ 号
api_token = "change-me"       # 主人 API token（Bearer）

[napcat]
listen = "127.0.0.1:8082"     # 反向 WS 监听（Stella 为服务端）
token = "change-me"           # 校验 NapCat 握手 Authorization: Bearer

[api]
listen = "127.0.0.1:3000"

[model]
provider = "anthropic"
name = "claude-sonnet-4-5"
# 模型 API key 不在此文件：data/auth.json（SDK ModelRuntime 机制）

[paths]
data_dir = "./data"           # = SDK agentDir

[memory]
max_entries_per_user = 200
max_content_chars = 2000

[short_term_memory]
max_tokens = 10000
max_age_days = 3

[tools]
whitelist = ["memory_save", "memory_search", "memory_list", "memory_delete"]
```

**运行布局**：部署目录内启动（相对路径相对 CWD；`--config` 或环境变量可覆盖配置路径）：

```
stella/
├── stella(.exe)     # bun build --compile 双产物（bun-windows-x64 / bun-linux-x64）
├── config.toml
└── data/            # = agentDir
    ├── auth.json    # 模型凭据（SDK）
    ├── models.json  # 模型目录（SDK）
    ├── sessions/    # 会话 JSONL（SDK SessionManager）
    └── memory.db    # 记忆 + 用户模型 + 会话注册表
```

**NapCat 联调**（文档承诺到接口契约，安装/登录/保活/风控链接 NapCat 官方文档）：

- NapCat `websocketClients` 加一项：`{ "url": "ws://<stella-host>:8082/onebot", "token": "<同 config.toml>", "messagePostFormat": "array", "reconnectInterval": 5000 }`
- 检查清单：连上后 Stella 日志应见 `lifecycle/connect` 与心跳；@无响应排查顺序：token → at 判定（`qq === self_id`）→ 群触发规则。
- 兼容面承诺 OneBot v11 协议，不承诺 NapCat 具体版本。

## 8. 决策票汇总（结论 → 票文件）

| 票 | 结论 | 一手来源 |
|---|---|---|
| 01 | 反向 WS 接入、@ 判定、段数组收发、message_id LRU 时效 | `issues/01-onebot-v11-napcat-facts.md` + `research/01` |
| 02 | Elysia 文档全自动；compile 三 issue；UI 资产 CDN 需自托管 | `issues/02` + `research/02` |
| 03 | 多会话可行（禁共享 ResourceLoader）；裁剪挂载点；工具门控机制 | `issues/03` + `research/03` |
| 04 | 硬截断（关 compaction，context 事件）；10K 可配 + 3 天 TTL；一视同仁 | `issues/04` |
| 05 | 注入标注格式；输出标记约定；@全体不触发 | `issues/05` + 原型分支 `proto/05-group-format` |
| 06 | 主人 7 件全开；白名单仅记忆工具；不引入网页搜索 | `issues/06` |
| 07 | 4 记忆工具；sqlite；用户模型 + 跨平台身份；schema 无 user 参数强制作用域 | `issues/07` |
| 08 | TOML 单文件；key 归 SDK；部署目录布局；NapCat 承诺接口契约 | `issues/08` |
| 09 | 惰性唤起 + 常驻不回收；sqlite 注册表；API/QQ 同运行时 | `issues/09` |
| 11 | Elysia + 四条写法纪律 + compile 实测任务 | `issues/11` |

## 9. 演进路径（v1 明确不做，但留了位置）

- **检索式记忆召回**（embedding 等）：v1 记忆量小用不上；会话文件全量保留、记忆表结构可扩，届时叠加。
- **多模型切换与负载均衡**：v1 配置单一模型。
- **网页搜索工具**：真实需求冒头后单独开研究票选型（Tavily/Brave/必应/DuckDuckGo），进白名单需重新过"按构造就安全"。
- **会话 LRU 驱逐**：会话数/内存涨起来后加，机制（dispose + 惰性再唤起）已就位。
- **群记忆**：归属权与可见性需专门设计。
- **多 admin 与管理端权限细分**：role 字段已留形。
- **新平台接入**：平台身份模型与适配层结构已按可扩展设计（platform 枚举 + 新适配器）。
- **主动发话/定时任务**：v1 Stella 纯响应式。

## 10. 实施任务与未细化项

**实施期必须验收的任务**：

1. `bun build --compile`（关 minify、守四条写法纪律）双平台打包 + 运行实测（含 `/openapi` 文档 UI 自托管资产）。
2. napcat 断连补偿：重连后 `get_group_msg_history`/`get_friend_msg_history` 拉取空洞消息（注意 message_id LRU 时效）。
3. 上下文裁剪扩展：溢出错误 catch 后主动裁剪重试（compaction 关闭后无 SDK 自动恢复）。
4. SSE 长连接：`server.timeout(req, 0)` 或保活帧防 Bun idleTimeout 断流。

**v1 未细化（实施中成形后补记）**：

- API 错误模型（统一错误响应结构、SSE 中途失败语义）与限流具体策略（防 token 滥用）。
- Stella 人格与系统提示词正文（结构已定，文案待调）。
- 消息编解码的边角段类型（合并转发、卡片等 v1 按占位处理）。
