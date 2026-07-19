# 配置与部署形态

Type: grilling
Status: closed (2026-07-19)

## Question

- 配置文件 schema：主人 QQ 号、API token、napcat 连接参数、工具白名单、模型与 API key、记忆存储路径——单文件还是分层？格式（JSON/YAML/TOML）？
- 密钥管理：模型 API key 走 SDK 的 AuthStorage 还是 Stella 自己的配置？
- `bun build --compile` 双平台（Linux/Windows）产物形态：单文件二进制 + 外挂配置/数据目录的运行布局。
- napcat 由用户自备的边界：部署文档要承诺到什么程度？

## Resolution

1. **配置 = 单文件 TOML**（Bun 原生 `Bun.TOML.parse` 零依赖；支持注释适合人维护；二十来个配置项不分层）。配置项：主人 QQ/token、napcat 连接参数、API 端口、provider/模型名、记忆阈值（200 条/2000 字符）、短期记忆（maxTokens 默认 10K、TTL 3 天）、工具白名单（默认仅记忆工具）。
2. **模型 API key 归 SDK**（`agentDir/auth.json` 经 `ModelRuntime.create({authPath})` 原生加载）；否掉 TOML 桥接（双写不一致）。Stella 自有概念（napcat token、主人 API token）留 TOML。
3. **运行布局**：部署目录内启动（相对路径相对 CWD），`stella(.exe)` 单文件二进制（`bun build --compile`，windows/linux 双产物）+ 外挂 `config.toml` + `data/`（= agentDir：auth.json、models.json、sessions/、memory.db）；`--config`/环境变量可覆盖配置路径；打包 flags 待票 11 框架定后拍死。
4. **NapCat 文档承诺到接口契约**：Stella 侧配置说明 + NapCat `websocketClients` 配置片段（url/token/`messagePostFormat:"array"`/reconnectInterval）+ 联调检查清单；兼容面承诺 OneBot v11 协议不承诺 NapCat 具体版本；安装/登录/保活/风控链接官方文档不兜底。
