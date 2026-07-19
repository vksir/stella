# NapCat 联调检查清单

## 前提

- NapCat 已安装并登录（参考 https://napneko.github.io/ ）
- Stella 已编译或通过 `bun run dev` 运行
- `config.toml` 已配置 `napcat.token` 和 `owner.qq`

## 1. NapCat 配置

在 NapCat 的 `websocketClients` 中添加一项（WebUI 或配置文件 `./config/onebot11_<QQ号>.json`）：

```json
{
  "url": "ws://127.0.0.1:8082/onebot",
  "token": "<与 config.toml napcat.token 一致>",
  "messagePostFormat": "array",
  "reconnectInterval": 5000
}
```

注意：
- `messagePostFormat` 必须为 `"array"`（提供结构化段数组，确保 @ 判定可靠）
- `token` 必须与 Stella `config.toml` 中 `napcat.token` 完全一致

## 2. 启动 Stella

```bash
# 开发模式
bun run dev

# 或编译产物
bun run build
bun run start
```

## 3. 连接检查

启动后观察 Stella 日志：
- `[Stella] 配置加载完成: config.toml`
- `[Stella] 数据目录: ./data`
- `[Stella] 模型: anthropic/claude-sonnet-4-5`
- `[QQ] WebSocket 服务启动: 127.0.0.1:8082`

NapCat 连接后 Stella 日志应出现：
- `[QQ] 收到 lifecycle 事件: connect`

## 4. 功能检查

### 4.1 群聊 @ 回复

1. 在群里发送 `@Stella 你好`
2. 验证：Stella 回复消息，且自动 @ 当前说话人
3. 发送 `@Stella [reply:#消息id]` → 验证引用回复

### 4.2 群聊 @全体 不触发

1. 在群里发送 `@全体成员 测试`
2. 验证：Stella **不回复**（仅倾听入库）

### 4.3 私聊对话

1. 给机器人发私聊消息 "你好"
2. 验证：Stella 逐条回复

### 4.4 群聊上下文

1. 在群里先后发：`@Stella 我叫小明`、`@Stella 我叫什么名字`
2. 验证：Stella 能记住上文（记忆工具可用时）

## 5. 韧性检查

### 5.1 断连恢复

1. 断开 NapCat（停止 NapCat 进程或断网）
2. Stella 日志出现 `[QQ] 连接关闭`
3. 重新启动 NapCat / 恢复网络
4. Stella 日志出现 `[QQ] 收到 lifecycle 事件: connect`
5. 验证：断连期间群里的消息被补偿拉取（通过 `get_group_msg_history` 入库）

### 5.2 上下文裁剪

1. 在群里发送大量消息（>10K token 量）
2. 验证：Stella 继续正常回复，不报 token 溢出错误

### 5.3 SSE 长连接

1. 通过 API `POST /sessions/:id/messages` 发送需长时间思考的问题
2. 验证：连接超过 10s 不中断

## 6. API 功能检查

```bash
# 新建会话
curl -H "Authorization: Bearer <api_token>" -X POST http://127.0.0.1:3000/sessions

# 发消息（SSE 流式）
curl -H "Authorization: Bearer <api_token>" \
     -H "Content-Type: application/json" \
     -X POST http://127.0.0.1:3000/sessions/<id>/messages \
     -d '{"content": "你好"}'

# 浏览 API 文档
浏览器打开: http://127.0.0.1:3000/openapi
```

## 常见问题排查

| 现象 | 排查方向 |
|------|----------|
| NapCat 连不上 | 检查 `url` 端口与 Stella `napcat.listen` 一致；检查防火墙 |
| 连接后立即断开 | 检查 `token` 一致；Stella 日志中 `token` 校验失败信息 |
| @机器人无回复 | (1) 检查 token → (2) 检查 at 判定（`array` 格式 + `qq === self_id`）→ (3) 检查模型 API key |
| 模型报错 | 检查 `data/auth.json` 是否存在且有有效凭据 |
| 群消息乱码 | 检查 `messagePostFormat: "array"` |
