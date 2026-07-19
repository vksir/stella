# OneBot v11 / napcat 协议事实

Type: research
Status: resolved

## Question

Stella 经 napcat 接入 QQ（OneBot v11 协议），平台层设计需要一组确切的协议事实：

1. 连接模式：napcat 支持哪些接入方式（正向 WebSocket、反向 WebSocket、HTTP）？各自的拓扑（谁是服务端）与重连语义？对"用 Bun 写的 Stella 进程"最自然的接法是哪种？
2. 事件模型：私聊与群聊消息事件的结构（message_id、user_id、group_id、message、raw_message、sender 等字段）。
3. 消息段（message segment）体系：纯文本、at、reply 等段的 JSON 表示；收到群消息时如何识别"是否 @ 了机器人"；构造回复时如何正确 @ 目标用户。
4. 发送消息 API：send_msg / send_private_msg / send_group_msg 的请求与回执。
5. napcat 相对 OneBot v11 标准的特有差异或扩展（若有）。

事实须引用一手来源（OneBot v11 官方规范、napcat 官方文档/源码）。

## Answer

- 连接：规范四种方式（HTTP / HTTP POST / 正向 WS / 反向 WS）；NapCat 反向 WS 固定 Universal 单连接、断线由 NapCat 按 `reconnectInterval`（默认 5s）无限重连——Bun 版 Stella 最自然的接法是内嵌 WS 服务端等 NapCat 反连，校验 `Authorization: Bearer`。
- 事件：私聊/群聊消息事件字段齐全（`message_id/user_id/group_id/message/raw_message/sender` 等）；NapCat 默认 `messagePostFormat: "array"`，并扩展 `message_sent`、`message_seq`、`group_name` 等字段。
- 消息段：`{type, data}` 结构、参数值均为字符串；@ 机器人判定 = `at` 段 `qq === String(self_id)`（`qq: "all"` 为 @全体，需另行决策）；回复用 `[at段, text段]` 数组，可加 `reply` 段引用 `message_id`。
- 发送 API：`send_msg/send_private_msg/send_group_msg` 均返回 `message_id`；`message` 参数接受 CQ 字符串 / 段数组 / 单段对象；NapCat 响应扩展 `wording/stream` 字段，message_id 为 LRU 短 ID（约 5000 条过期）。
- NapCat 差异：JSON5 解析、HTTP 鉴权失败统一 403、未知 action 回 HTTP 200、HTTP-SSE 通道、`get_group_msg_history` 等扩展 API（全量见 napcat.apifox.cn）。

详见 [../research/01-onebot-v11-napcat-facts.md](../research/01-onebot-v11-napcat-facts.md)
