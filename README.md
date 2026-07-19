# Stella

基于 [PI SDK](https://www.npmjs.com/package/@earendil-works/pi-coding-agent) 的 QQ 机器人，通过 NapCat 反向 WebSocket 接入。

## 快速开始

```bash
# 安装依赖
bun install

# 配置
cp config.example.toml config.toml
# 编辑 config.toml：填写 QQ 号、API token、模型等

# 配置模型凭据
mkdir -p data
# 编辑 data/auth.json，填入 API key

# 启动
bun run dev
```

## 目录结构

```
src/
├── adapters/qq/     # QQ 协议适配（NapCat 反向 WS）
├── api/             # HTTP API（Elysia + OpenAPI）
├── stores/          # 持久化存储（SQLite）
├── tools/           # 自定义工具（defineTool）
├── index.ts         # 入口 + bootstrap
├── session-factory.ts    # AgentSession 工厂（SDK 适配层）
├── sessions-registry.ts  # 会话注册表
├── config.ts        # 配置加载（TOML）
├── context-trimmer.ts    # 上下文裁剪扩展
└── identity.ts      # 身份解析
test/                # 测试（镜像 src 结构）
scripts/build.ts     # 打包脚本（Bun compile）
```

## 脚本

| 命令 | 说明 |
|------|------|
| `bun run dev` | 开发运行 |
| `bun test` | 运行单元测试 |
| `bun run typecheck` | TypeScript 类型检查 |
| `bun run e2e` | E2E 端到端测试（自动拉起服务） |
| `bun run build` | 构建 Windows 单文件（dist/stella.exe） |
| `bun run build:linux` | 构建 Linux 单文件（dist/stella） |

## 部署

```bash
# 构建
bun run build

# 部署目录
mkdir my-stella && cd my-stella
cp ../dist/stella.exe .
cp ../config.example.toml config.toml
# 编辑 config.toml
mkdir -p data
# 编辑 data/auth.json
./stella.exe
```

## 配置参考

见 `config.example.toml`，关键字段：

- `owner.qq` — 管理员 QQ 号
- `model.provider` / `model.name` — 模型选择
- `model.thinking_level` — 思考深度（默认 `high`）
- `memory.max_entries_per_user` — 每用户记忆上限
- `short_term_memory.max_tokens` — 上下文窗口大小
