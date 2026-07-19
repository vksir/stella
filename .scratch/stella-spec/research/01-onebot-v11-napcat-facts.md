# OneBot v11 / NapCat 协议事实

> 研究票 `issues/01-onebot-v11-napcat-facts.md` 的成果文件。为 Stella 平台适配层（napcat 以 OneBot v11 接入 QQ）提供协议事实。
>
> 标注约定：
> - 【规范】= OneBot v11 官方规范（https://11.onebot.dev/ ，源码仓库 https://github.com/botuniverse/onebot-11，本文引用 master 分支的具体 md 文件）
> - 【NapCat文档】= NapCat 官方文档（https://napneko.github.io/ ，源码仓库 https://github.com/NapNeko/NapCatDocs）
> - 【NapCat源码】= NapCatQQ 主仓库 main 分支源码（https://github.com/NapNeko/NapCatQQ）
> - 【推断】= 本文作者基于上述事实的推断/建议，非来源明示

## 0. 来源清单

| 来源 | URL |
| --- | --- |
| OneBot v11 规范站点 | https://11.onebot.dev/ |
| OneBot v11 规范仓库（与站点同内容） | https://github.com/botuniverse/onebot-11 |
| NapCat 官方文档站 | https://napneko.github.io/ |
| NapCat 文档源码仓库 | https://github.com/NapNeko/NapCatDocs |
| NapCatQQ 源码仓库 | https://github.com/NapNeko/NapCatQQ |
| NapCat 全量 API 用例（官方维护） | https://napcat.apifox.cn （见 https://napneko.github.io/onebot/api 页首指引） |

注：`napcat.wiki` 域名目前不可达，官方文档以 napneko.github.io 为准。【推断：napcat.wiki 可能是社区镜像或已停用】

---

## 1. 连接模式

### 1.1 OneBot v11 规范定义的四种通信方式

【规范】规范定义 4 种通信方式：HTTP、HTTP POST、正向 WebSocket、反向 WebSocket。来源：https://github.com/botuniverse/onebot-11/blob/master/communication/README.md

| 方式 | 拓扑（谁是服务端） | 方向 | 鉴权 | 重连语义 |
| --- | --- | --- | --- | --- |
| HTTP | OneBot 是 HTTP 服务端，监听 IP:端口，接受 `GET/POST /:action` | 仅 API 调用（用户→OneBot），不推事件 | `Authorization: Bearer <token>` 头或 `?access_token=` query | 无连接概念 |
| HTTP POST | 用户是 HTTP 服务端；OneBot 作为客户端向配置的上报 URL POST 事件 | 仅事件推送（OneBot→用户）；响应体可带"快速操作" | 配 `secret` 时加 `X-Signature: sha1=<HMAC-SHA1(secret, body)>` 头；另有 `X-Self-ID` 头 | 上报失败/超时由实现自行处理（规范未定重推） |
| 正向 WS | OneBot 是 WS 服务端，接受路径 `/api`、`/event`、`/`（`/` 为两者合一） | 一条连接上 API 调用 + 事件推送（全双工） | 连接建立时校验 access token（头或 query），鉴权失败直接断开 | 无内建重连（客户端自理） |
| 反向 WS | 用户是 WS 服务端；OneBot 作为客户端主动连配置的 URL | 一条连接上 API 调用 + 事件推送（Universal 客户端） | OneBot 连接请求带 `Authorization: Bearer <token>`、`X-Self-ID`、`X-Client-Role: API/Event/Universal` 头 | 断线后按配置间隔（示例默认 `ws_reverse.reconnect_interval=3000`ms）不断重连直至成功 |

来源：
- HTTP：https://github.com/botuniverse/onebot-11/blob/master/communication/http.md
- HTTP POST：https://github.com/botuniverse/onebot-11/blob/master/communication/http-post.md
- 正向 WS：https://github.com/botuniverse/onebot-11/blob/master/communication/ws.md
- 反向 WS：https://github.com/botuniverse/onebot-11/blob/master/communication/ws-reverse.md
- 鉴权：https://github.com/botuniverse/onebot-11/blob/master/communication/authorization.md

【规范】补充事实：
- 正向 WS 的 `/event` 推送不做签名、不处理响应数据；正向 WS 与 HTTP POST 可同时启用，事件会先 HTTP POST 上报，再推给所有 `/event` WS 客户端。（ws.md）
- WS 上 API 调用格式为 `{"action": "...", "params": {...}, "echo": "任意值"}`；响应含 `status`/`retcode`/`data`/`echo`，HTTP 状态码错误映射为 retcode 1400/1401/1403/1404（1401/1403 实际不会出现在 WS 上，因为鉴权失败在连接阶段就断开了）。（ws.md）
- HTTP 响应：`status` 为 `ok`/`async`/`failed`；`retcode` 0=成功、1=异步；HTTP 状态码 401（未提供 token）/403（token 不符）/406/400/404（API 不存在）/其余一律 200。（http.md）

### 1.2 NapCat 的网络配置模型与实现事实

【NapCat文档】NapCat 按"网络类型"配置 4 类适配器，每类可配多个实例（配置文件 `./config/onebot11_<QQ号>.json`，v4.5.3 起支持 `./config/onebot11.json` 作默认配置，亦可 WebUI 配置）：

| 类型 | 描述 |
| --- | --- |
| HTTP 服务端 `httpServers` | NapCat 作为 HTTP 请求接受方，接收 API 调用并回应（单工） |
| HTTP 客户端 `httpClients` | NapCat 作为 HTTP 请求发起方，将事件 POST 推送至应用（单工） |
| WebSocket 服务端 `websocketServers` | 即正向 WS，NapCat 为服务端（双工） |
| WebSocket 客户端 `websocketClients` | 即反向 WS，NapCat 为客户端（双工） |

来源：https://napneko.github.io/config/basic

【NapCat源码】各适配器配置字段与默认值（`packages/napcat-onebot/config/config.ts`，https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/config/config.ts）：

- 公共：`name`、`enable`、`messagePostFormat`（**默认 `array`**，可选 `string`）、`token`（默认空=不鉴权）、`debug`
- `httpServers`：`host`（默认 127.0.0.1）、`port`（3000）、`enableCors`、`enableWebsocket`
- `httpClients`：`url`、`reportSelfMessage`（默认 false）
- `websocketServers`：`host`、`port`（3001）、`reportSelfMessage`、`enableForcePushEvent`、`heartInterval`（默认 30000ms）
- `websocketClients`：`url`、`reportSelfMessage`、`reconnectInterval`（默认 5000ms）、`heartInterval`（默认 30000ms）、`verifyCertificate`
- 另有 `httpSseServers`（HTTP SSE 适配器，见 §5）与 `plugins`

【NapCat源码】反向 WS 客户端行为（`packages/napcat-onebot/network/websocket-client.ts`）：
- 连接请求头固定为：`X-Self-ID: <机器人uin>`、`Authorization: Bearer <token>`、`x-client-role: Universal`（注释说明为兼容 koishi 适配器）、`User-Agent: OneBot/11`；握手超时 2s，maxPayload 50MB。
- 即 NapCat 反向 WS 只有 Universal 一种角色：一条连接同时承载 API 调用与事件推送。
- 连接成功（open）后立即发送一个 `meta_event.lifecycle`、`sub_type: connect` 的元事件。
- `heartInterval > 0` 时每 `heartInterval` 毫秒沿连接发送心跳元事件（`meta_event.heartbeat`，`status: {online, good}`，`interval` 为心跳间隔）。
- 断线（close 或 error）后，只要适配器仍启用，每 `reconnectInterval` 毫秒无限重试重连（`setTimeout(tryConnect, reconnectInterval)`），日志提示重连倒计时。**断连期间产生的事件不会缓存补发**（源码中 onEvent 仅在连接 OPEN 时 send，无队列）。【推断：Stella 若要弥补断连窗口的消息空洞，需借助消息历史 API 主动拉取，见 §5 扩展 API】

来源：https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/network/websocket-client.ts

【NapCat源码】正向 WS 服务端行为（`packages/napcat-onebot/network/websocket-server.ts`）：
- 在配置 host:port 起 `WebSocketServer`。token 非空时鉴权：优先取 `?access_token=` query，其次 `Authorization: Bearer` 头；失败则回发 `{"status":"failed","retcode":1403,...}` 后关闭连接。
- 路径语义：仅 `/api`（或 `/api/`）是"纯 API"连接（不推事件、不发 lifecycle）；**其余任意路径**（含 `/`、`/event`）都会收到事件推送，并在连接建立时收到 lifecycle connect 元事件。【推断：与规范的三路径模型意图一致，但实现上是"`/api` 特殊、其余皆事件"的判定，Stella 直接连 `/` 或任意非 `/api` 路径即可】
- 事件推送给所有已连接的事件客户端；心跳元事件按 `heartInterval` 发送。

来源：https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/network/websocket-server.ts

【NapCat源码】HTTP 服务端行为（`packages/napcat-onebot/network/http-server.ts`）：
- express 服务；`GET /:action`（query 传参）或 `POST /:action`（body 以 **JSON5** 解析，同时兼容 urlencoded）；根路径 `/` 返回 "NapCat4 Is Running"（可作存活探测）。
- token 校验失败一律返回 HTTP 403 `{message: 'token verify failed!'}`（规范区分 401/403，NapCat 统一 403——差异点）。
- API 不存在时返回 HTTP 200 + `{"status":"failed","retcode":200,"message":"不支持的Api ..."}`（规范为 404——差异点）；WS 上对应 retcode 1404。
- `enableWebsocket: true` 时同一端口上同时挂 WS 服务（行为同正向 WS）。【NapCat文档】标注该配置"暂时没有作用"（https://napneko.github.io/config/basic）——以源码为准它是生效的，但属非主推用法【推断：不要依赖】。

来源：https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/network/http-server.ts

【NapCat源码】HTTP 客户端（HTTP POST 上报）行为（`packages/napcat-onebot/network/http-client.ts`）：
- 向配置 `url` POST 事件 JSON，请求头：`Content-Type: application/json`、`x-self-id: <uin>`；token 非空时加 `x-signature: sha1=<HMAC-SHA1(token, body)>`（规范中该密钥叫 `secret`，NapCat 复用 `token` 字段——语义对应）。
- 上报响应体按 JSON5 解析为"快速操作"（quick operation）并执行（如快速回复）。

来源：https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/network/http-client.ts

【NapCat文档】自 v4.4.14 起 NapCat 用 JSON5 标准解析 WS/HTTP 请求（允许尾随逗号与注释）。来源：https://napneko.github.io/develop/msg

### 1.3 对"Bun 实现的 Stella 进程"最自然的接法

【推断】推荐 **反向 WebSocket（NapCat `websocketClients` → Stella 内嵌 WS 服务端）**，理由：
1. 拓扑自然：Bun 的 `Bun.serve()` 原生提供高性能 WebSocket 服务端，Stella 一个进程内即可同时持有事件流与 API 通道（NapCat 反向 WS 固定为 Universal 单连接，见 §1.2），无需维护两条连接。
2. 重连语义省心：规范与 NapCat 源码都明确反向 WS 断线后由 OneBot 侧（NapCat）按 `reconnectInterval` 无限重连；Stella 作为服务端只需处理"接受连接、校验 `Authorization: Bearer` 头、读 `X-Self-ID` 头"。
3. 鉴权闭环：token 配在 NapCat 侧，Stella 校验握手头即可；NapCat 官方安全复盘明确要求公网部署务必启用 token（https://napneko.github.io/develop/security）。
4. 生命周期信号清晰：连接建立即收 `lifecycle/connect` 元事件，之后按 `heartInterval` 收心跳元事件，可直接驱动 Stella 的连接存活状态机。

备选：**正向 WS**（Stella 作客户端连 NapCat）同样可行，差别仅在谁发起连接与谁负责重连——正向模式下重连逻辑落在 Stella 自己手里。HTTP+HTTP POST 组合需要两条单向通道且事件依赖回调可达性，【推断】对单进程 Agent 属次选。

---

## 2. 事件模型

### 2.1 事件公共结构

【规范】所有事件含 `time`（int64 秒级时间戳）、`self_id`（收到事件的机器人 QQ 号，int64）、`post_type`；`post_type` 取值：`message`（消息）、`notice`（通知）、`request`（请求）、`meta_event`（元事件）。来源：https://github.com/botuniverse/onebot-11/blob/master/event/README.md

【NapCat源码】NapCat 扩展第 5 个取值 `message_sent`（机器人自己发出的消息，仅当适配器 `reportSelfMessage: true` 时才上报，默认不上报）。来源：https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/api/msg.ts（`post_type: this.core.selfInfo.uin === msg.senderUin ? EventType.MESSAGE_SENT : EventType.MESSAGE`）与 https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/index.ts（self 消息按适配器配置过滤）

### 2.2 私聊消息事件

【规范】字段表（https://github.com/botuniverse/onebot-11/blob/master/event/message.md）：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `time` | number(int64) | 时间戳 |
| `self_id` | number(int64) | 机器人 QQ 号 |
| `post_type` | string | `message` |
| `message_type` | string | `private` |
| `sub_type` | string | `friend`（好友）/ `group`（群临时会话）/ `other` |
| `message_id` | number(int32) | 消息 ID |
| `user_id` | number(int64) | 发送者 QQ 号 |
| `message` | message | 消息内容（字符串或消息段数组，取决于消息格式配置） |
| `raw_message` | string | 原始消息内容（CQ 码字符串） |
| `font` | number(int32) | 字体 |
| `sender` | object | 发送人信息：`user_id`、`nickname`、`sex`(male/female/unknown)、`age`；尽最大努力提供，不保证字段齐全与准确 |

【NapCat源码】NapCat 侧私聊事件补充事实：
- 临时会话（群成员私聊）`sub_type: 'group'`，并带 `group_id`（来源群号）与 `temp_source`（来源：https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/api/msg.ts `handleTempGroupMessage`）。
- 额外字段：`message_seq`（消息序列号）、`real_id`、`real_seq`（真实 QQ msgSeq）、`message_format`（`array`/`string`）、`message_sent_type`（`self`）、`target_id` 等（https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/types/message.ts `OB11MessageSchema`）。

### 2.3 群消息事件

【规范】字段表（https://github.com/botuniverse/onebot-11/blob/master/event/message.md）：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `time` / `self_id` / `post_type` | — | 同私聊 |
| `message_type` | string | `group` |
| `sub_type` | string | `normal`（正常）/ `anonymous`（匿名）/ `notice`（系统提示） |
| `message_id` | number(int32) | 消息 ID |
| `group_id` | number(int64) | 群号 |
| `user_id` | number(int64) | 发送者 QQ 号 |
| `anonymous` | object \| null | 匿名信息（`id`/`name`/`flag`），非匿名为 null |
| `message` / `raw_message` / `font` | — | 同私聊 |
| `sender` | object | `user_id`、`nickname`、`card`（群名片）、`sex`、`age`、`area`、`level`、`role`（owner/admin/member）、`title`（专属头衔）；匿名消息中无参考价值 |

【NapCat源码】NapCat 群事件补充：`sender` 实现提供 `user_id`、`nickname`、`card`、`role`（https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/api/msg.ts `handleGroupMessage`，文档所列 `title`/`level` 在当前源码构造中未填充【推断：按规范"尽最大努力"语义处理，字段可缺省】）；额外有 `group_name` 字段（types/message.ts）。

### 2.4 `message` 字段的两种格式

【规范】事件中 `message` 的实际类型由消息格式配置决定：字符串格式（CQ 码）或消息段数组格式；规范示例配置 `event.message_format` 默认 `string`（https://github.com/botuniverse/onebot-11/blob/master/event/README.md）。

【NapCat源码】NapCat 每个网络适配器独立配置 `messagePostFormat`，**默认 `array`**；事件中带 `message_format` 字段标明本次格式；`raw_message` 恒为 CQ 码字符串（由消息段 encode 后拼接 trim 得到）。来源：https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/api/msg.ts（`convertArrayToStringMessage` 等）、https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/index.ts（按适配器格式分发 `stringMsg`/`arrayMsg`）

【推断】Stella 应显式将接入适配器配为 `messagePostFormat: "array"`：数组格式免 CQ 码转义歧义、@ 检测可靠（见 §3.3），且 NapCat 默认即是 array。

### 2.5 元事件（lifecycle / heartbeat）

【规范】（https://github.com/botuniverse/onebot-11/blob/master/event/meta.md）：
- 生命周期：`meta_event_type: 'lifecycle'`，`sub_type: enable/disable/connect`；**只有 HTTP POST 能收到 enable/disable，只有正向/反向 WS 能收到 connect**。
- 心跳：`meta_event_type: 'heartbeat'`，`status`（同 `get_status` 快速操作的结构）、`interval`（ms）。

【NapCat源码】NapCat 心跳 `status: {online: boolean, good: boolean}`，`interval` = 适配器 `heartInterval`；WS 连接建立即发 lifecycle connect（§1.2）。来源：https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/event/meta/OB11HeartbeatEvent.ts 及 websocket-client.ts

---

## 3. 消息段（message segment）体系

### 3.1 基本结构

【规范】数组格式中消息 = 消息段对象数组；段结构 `{"type": "<功能名>", "data": {...}}`，`type` 对应 CQ 码功能名，`data` 对应参数（可为 null）。**除合并转发相关段外，几乎所有消息段参数值类型均为字符串**（为与 CQ 码互转）。数组格式不需要转义。来源：https://github.com/botuniverse/onebot-11/blob/master/message/array.md

【规范】字符串格式即 CQ 码：`[CQ:face,id=178]`；纯文本转义 `&`→`&amp;`、`[`→`&#91;`、`]`→`&#93;`，CQ 码参数值额外转义 `,`→`&#44;`。来源：https://github.com/botuniverse/onebot-11/blob/master/message/string.md

### 3.2 关键段的 JSON 表示

【规范】（https://github.com/botuniverse/onebot-11/blob/master/message/segment.md）：

纯文本：
```json
{"type": "text", "data": {"text": "纯文本内容"}}
```

@某人：
```json
{"type": "at", "data": {"qq": "10001000"}}
```
- `qq`：被 @ 的 QQ 号（字符串），或 `"all"` 表示 @全体成员。收、发同构。

回复：
```json
{"type": "reply", "data": {"id": "123456"}}
```
- `id`：被引用消息的 `message_id`。收、发同构。

图片：
```json
{"type": "image", "data": {"file": "http://baidu.com/1.jpg"}}
```
- 发送时 `file` 支持：收到的文件名、`file:///` 绝对路径 URI、网络 URL、`base64://...`；接收时另有 `url`（图片 URL）、`type`（`flash` 为闪照）。

其他常用：`face`（QQ 表情，`id`）、`record`（语音）、`video`、`json`（卡片）、`forward`（合并转发，收）、`node`（转发节点，发）、`share`、`contact`、`location`、`music`（仅发）、`poke`、`dice`、`rps`、`shake`（仅发）。

【NapCat源码】NapCat 段定义（TypeBox schema，https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/types/message.ts）与规范一致，并有扩展：
- `at` 段：`data.qq: string`（"QQ号或all"），可选 `name`。
- `reply` 段：`data.id?: string`（消息 ID 短 ID 映射），**扩展可选 `seq?: number`（真实消息序列号，标注"优先使用"）**。
- `image` 段接收时扩展：`file_id`、`path`、`file_size`、`file_unique`、`summary`（描述）、`sub_type`；发送可选 `name`、`summary`。
- `face` 段接收扩展：`raw`、`resultId`、`chainCount`；`dice`/`rps` 接收带 `result`。
- 扩展段类型：`mface`（商城表情，接收时以 `image` 上报、子类型区分；发送可用 `mface` 或 `image`）、`markdown`、`lightapp`（小程序卡片，发送走扩展接口 `get_mini_app_ark`）、`file`（文件段）。

【NapCat文档】段兼容表（https://napneko.github.io/develop/msg）：`shake` 只收不发；`share`/`location` 只收（以 `json` 类型上报）；`music` 接收时转为 `json` 类型；`poke` 群聊戳一戳"事件上报与接口调用，不通过消息"。

### 3.3 识别"是否 @ 了机器人"

【规范】事实基础：群消息中 @ 某人表示为 at 段，`qq` 为被 @ 者 QQ 号；事件带 `self_id`（机器人自身 QQ 号）。（segment.md、event/message.md）

【NapCat源码】NapCat 构造入站 at 段时：@全体 → `qq: "all"`；@个人 → 目标用户 uin 的字符串（https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/api/msg.ts `textElement` 转换器，atType 分支）。

【推断】可靠判定逻辑（数组格式下）：
```text
isAtBot = message.some(seg => seg.type === 'at' && String(seg.data.qq) === String(event.self_id))
```
注意三点：(1) `qq` 是字符串、`self_id` 是 number，比较前统一转字符串；(2) `qq: "all"` 是 @全体，**不等于** @机器人——是否把 @全体 视为触发属产品决策，协议层面可区分；(3) 不要用 `raw_message` 正则匹配 `[CQ:at,qq=...]`，字符串格式有转义与格式变体风险，数组格式才是结构化事实。

### 3.4 构造回复时 @ 目标用户

【规范】发送消息时 `message` 可传消息段数组，at 段与 text 段按序拼接发送（segment.md、api/public.md）。

【推断】群聊回复某人的标准构造（沿用 NapCat 段结构）：
```json
[
  {"type": "at", "data": {"qq": "<目标user_id字符串>"}},
  {"type": "text", "data": {"text": " 回复内容"}}
]
```
（at 段后接一个以空格开头的 text 段，QQ 客户端显示更自然——这是惯例做法，非协议强制。）如需引用原消息，在最前加 `{"type": "reply", "data": {"id": "<原消息message_id>"}}`；NapCat 的 reply 段还支持 `seq`【NapCat源码】。

---

## 4. 发送消息 API

### 4.1 规范定义

【规范】（https://github.com/botuniverse/onebot-11/blob/master/api/public.md）：

`send_private_msg` 发送私聊消息：
- 参数：`user_id`(number，对方 QQ 号）、`message`（message 类型）、`auto_escape`(boolean，默认 false；仅字符串形式时有效，为 true 则不解析 CQ 码）
- 响应数据：`message_id`(number int32)

`send_group_msg` 发送群消息：
- 参数：`group_id`(number，群号）、`message`、`auto_escape`
- 响应数据：`message_id`

`send_msg` 发送消息：
- 参数：`message_type`(`private`/`group`，不传则按 `*_id` 参数推断）、`user_id`、`group_id`、`message`、`auto_escape`
- 响应数据：`message_id`

【规范】`message` 类型参数允许三种形态：字符串（CQ 码）、消息段数组、单个消息段对象（https://github.com/botuniverse/onebot-11/blob/master/api/README.md）。

【规范】所有 API 可加 `_async` 后缀（异步，响应 `status: async`）与 `_rate_limited` 后缀（限速排队发送，防腾讯风控；排队间隔示例默认 500ms）（api/README.md）。

### 4.2 调用与回执形态

【规范】WS 调用（正向/反向一致）：
```json
// 请求
{"action": "send_group_msg", "params": {"group_id": 123456, "message": [{"type":"at","data":{"qq":"10001"}},{"type":"text","data":{"text":" 大家好！"}}]}, "echo": "req-1"}
// 响应
{"status": "ok", "retcode": 0, "data": {"message_id": 5678}, "echo": "req-1"}
```
（ws.md；响应结构与 HTTP 一致，外加原样返回的 `echo`。）

【规范】HTTP 调用：`POST /send_group_msg`，JSON body 即参数；响应 `{"status":"ok","retcode":0,"data":{...}}`（http.md）。

【NapCat源码】NapCat 响应对象扩展了两个字段：`wording`（= `message`）与 `stream`（`'normal-action'`/`'stream-action'`），完整形态 `{status, retcode, data, message, wording, echo, stream}`（https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/action/OneBotAction.ts）。

【NapCat源码】NapCat 侧错误码（WS 通道）：JSON 解析失败/参数校验失败 `1400`；API 不存在 `1404`；内部执行异常 `1200`；HTTP 通道参数校验失败 `400`、内部异常 `200`（OneBotAction.ts、websocket-server.ts）。

【NapCat文档】NapCat 的 `send_msg`/`send_private_msg`/`send_group_msg` 参数中 `user_id`/`group_id` 接受 number 或 string（https://napneko.github.io/onebot/api；源码 SendMsg.ts schema 为 string 可选 + 推断 message_type，https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/action/msg/SendMsg.ts）。

### 4.3 message_id 的时效性（NapCat 特有）

【NapCat文档】NapCat 无数据库，用 LRU 缓存管理消息与文件资源：**消息 ID 是基于哈希的正整数（短 ID 映射），约 5000 条后过期清理**；已撤回消息无法再获取。来源：https://napneko.github.io/onebot/napcat

【推断】Stella 会话历史里若要持久引用 QQ 消息（reply 引用、撤回），应在事件到达时立刻落库自存的 `message_id`↔上下文映射，并容忍 `get_msg`/`delete_msg` 对过期 ID 失败；NapCat 事件同时携带 `message_seq`/`real_seq`（真实序列号），可作为备用键。

---

## 5. NapCat 相对 OneBot v11 的差异与扩展汇总

以下均为【NapCat文档】/【NapCat源码】事实，与规范逐条对照：

### 5.1 协议行为差异

| 项 | OneBot v11 规范 | NapCat 实际 | 来源 |
| --- | --- | --- | --- |
| 默认消息格式 | `string`（CQ 码） | `array`（消息段数组），按适配器 `messagePostFormat` 配 | https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/config/config.ts |
| HTTP 鉴权失败 | 401（未提供）/403（不符） | 统一 403 | http-server.ts |
| HTTP API 不存在 | HTTP 404 | HTTP 200 + retcode 200 | http-server.ts |
| 请求解析 | JSON | **JSON5**（v4.4.14 起，允许注释/尾逗号） | https://napneko.github.io/develop/msg |
| 响应结构 | `status/retcode/data(/echo)` | 额外 `message`、`wording`、`stream` 字段 | OneBotAction.ts |
| HTTP POST 签名密钥 | 配置项叫 `secret` | 复用适配器 `token` 字段做 HMAC-SHA1 | http-client.ts |
| 正向 WS 路径 | `/api`、`/event`、`/` 三路 | 仅 `/api` 特殊（纯 API），其余路径全量双工 | websocket-server.ts |
| 反向 WS 角色 | API/Event/Universal 三种客户端可拆 | 固定 `x-client-role: Universal` 单连接 | websocket-client.ts |
| 反向 WS 重连间隔 | 示例默认 3000ms | 默认 5000ms（`reconnectInterval` 可配） | config.ts |
| WS 心跳 | 规范示例默认 15000ms、`heartbeat.enable` 默认关 | 默认 30000ms、`heartInterval>0` 即开 | config.ts |

### 5.2 事件与数据扩展

- `post_type: 'message_sent'`：机器人自身消息事件，适配器 `reportSelfMessage: true` 才上报（默认 false）。（NapCat 源码 api/msg.ts、index.ts；NapCat文档 https://napneko.github.io/onebot/basic_event 字段表亦列出 `'message' | 'message_sent'`）
- 消息事件扩展字段：`message_format`、`message_seq`、`real_id`、`real_seq`、`group_name`、`target_id`、`temp_source`、`message_sent_type`、`emoji_likes_list`。（types/message.ts）
- 扩展通知事件：`group_msg_emoji_like`（表情回应）、`input_status`（输入状态）、`profile_like`（资料点赞）、`bot_offline`（机器人离线）、`gray_tip`（群灰条）、`group_name`/`title`（群名/头衔变更，notice_type=`notify`）等。（https://napneko.github.io/onebot/event）
- 扩展消息段：`mface`、`markdown`、`lightapp`、`file`；`reply` 段扩展 `seq`；`image` 段扩展 `file_id/path/file_size/file_unique/summary/sub_type`。（§3.2 来源）

### 5.3 扩展 API（节选，全量见 https://napcat.apifox.cn 与 https://napneko.github.io/onebot/api）

对 Stella 平台层最相关者：
- 消息历史：**`get_group_msg_history`（group_id, count）、`get_friend_msg_history`（user_id, count）**——断连补偿的关键抓手【推断】
- `get_msg` / `delete_msg` / `get_forward_msg`（规范内，但受 §4.3 LRU 时效约束）
- `mark_group_msg_as_read` / `mark_private_msg_as_read` / `_mark_all_as_read`
- `set_msg_emoji_like`（表情回应）、`fetch_emoji_like`
- `group_poke` / `friend_poke` / `send_poke`（戳一戳）
- `get_group_member_list` / `get_group_member_info` / `get_group_list` / `get_friend_list`（规范内）
- `send_group_forward_msg` / `send_private_forward_msg` / `send_forward_msg`（合并转发）
- `ocr_image`（图片 OCR）、`.ocr_image`（增强）
- `set_input_status`（输入状态）、`set_online_status`、`bot_exit`
- 群文件系统系列（`upload_group_file`、`get_group_file_url` 等）、`download_file`
- `ArkSharePeer` / `ArkShareGroup` / `get_mini_app_ark`（卡片分享）

### 5.4 HTTP SSE（NapCat 独创通道）

【NapCat文档】NapCat 提供 HTTP-SSE 适配器（`httpSseServers`）：事件通过 `GET /_events` 的 SSE 长连接推送，API 仍走常规 HTTP `/:action`；协议端完全作 Server、应用端完全作 Client。来源：https://napneko.github.io/develop/http-sse 、源码 https://github.com/NapNeko/NapCatQQ/blob/main/packages/napcat-onebot/network/http-server-sse.ts

【推断】对 Bun/Stella 而言 SSE 需要客户端长连处理，与反向 WS 相比无额外收益（WS 本就全双工），仅备录。

---

## 6. 对 Stella 平台层的直接推论（全部为【推断】）

1. **接入形态**：NapCat 配一个 `websocketClients` 项指向 Stella（如 `ws://127.0.0.1:8082/onebot`），设 `token`、`messagePostFormat: "array"`；Stella 用 Bun 原生 WS 服务端接受连接、校验 `Authorization: Bearer`、记录 `X-Self-ID`。
2. **消息入口流水线**：按 `post_type` 分流——`message` 进消息管线；`meta_event`（lifecycle/heartbeat）驱动连接状态机；`message_sent`（若开启）视为机器人自己的回声；`notice`/`request` v1 可先记录不处理。
3. **触发判定**：群聊仅 `@机器人` 触发 = `at` 段 `qq === String(self_id)`（§3.3）；私聊逐条必回。判定前先把 `message` 归一为段数组（配置已保证 array）。
4. **回复构造**：`send_msg`（按 message_type 分发）或 `send_group_msg`/`send_private_msg`，段数组 `[at?, reply?, text...]`；用入站事件的 `message_id` 做 reply 引用；`echo` 用 UUID 关联请求-响应。
5. **韧性**：依赖 NapCat 侧重连（反向 WS 语义），Stella 监听 close 事件置状态；断连窗口的消息空洞用 `get_group_msg_history`/`get_friend_msg_history` 补偿拉取（§5.3），注意 message_id LRU 时效（§4.3）。
6. **安全**：token 必填（NapCat 安全复盘 https://napneko.github.io/develop/security）；监听地址按需绑 127.0.0.1。
