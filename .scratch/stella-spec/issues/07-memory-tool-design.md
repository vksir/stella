# 记忆工具设计

Type: grilling
Status: closed (2026-07-19)
Blocked by: 03

## Question

长期记忆纯工具化已定，本票定记忆工具的具体设计：

- 工具面：save / search / list / update / delete 切到什么程度？一个工具还是多个？
- 存储介质：`bun:sqlite` 还是 JSON 文件？（单文件二进制打包下的考量。）
- 记忆条目数据模型：内容、所属用户、创建/更新时间、来源会话要不要记？
- "作用域 = 当前说话人"在工具实现上如何强制（工具执行时注入当前用户上下文）？
- 模型何时该存记忆的引导：靠系统提示词还是工具描述？

依赖 [PI SDK 深度行为事实] 中 defineTool 能力与动态上下文挂载点。

## Resolution

1. **工具面 = 4 个独立工具**：`memory_save`（可选 `id`，带 id 即覆盖更新、不带即新建）/ `memory_search`（关键词）/ `memory_list` / `memory_delete`。否掉单工具多 action（union 参数是模型误用重灾区）与独立 update。
2. **存储 = `bun:sqlite`**（内建零依赖；过滤/搜索/上限均一行 SQL；事务写入；compile 兼容）。否掉 JSON 文件（整文件重写怕崩、并发需自行串行化，且记忆管理走 API 层不需要肉眼直读）。
3. **数据模型**：记忆条目 = `id` / **`user_id`**（→ users）/ `content`（≤2000 字符）/ `created_at` / `updated_at` / `source_session`（可空，管理追溯用）。护栏：每用户 200 条上限，超限拒存提示先删；两阈值进配置。**用户模型**（应用户要求为多云台同一人识别而设）：`users(id, display_name, role: admin|guest, created_at)` + `user_identities(user_id, platform, platform_user_id)`，`(platform, platform_user_id)` 唯一；v1 仅一个 admin（配置登记的主人）。身份解析：消息到达按 `(platform, platform_user_id)` 查身份，未命中自动建档；首次启动把配置的主人 QQ + API token 关联到同一 admin。v1 API 层提供**用户合并端点**（源 user 的身份与记忆转移到目标后删源）。
4. **作用域强制**：记忆工具 schema 无 user 参数（模型无处越权）；user_id 只来自会话级闭包（派发前身份解析写入，工具 execute 读闭包过滤）；同会话消息串行派发保证闭包与轮次严格对应。
5. **存记忆引导放工具侧**（`description` + `promptGuidelines`），系统提示词正文仅一句原则。规则：该存 = 明确要求/稳定偏好/长期事实；不该存 = 一次性请求与闲聊/重复内容（先 search 再 save）/**敏感凭据**（密码密钥，要求也不存）；该更新 = 带 id 覆盖不新增重复。
