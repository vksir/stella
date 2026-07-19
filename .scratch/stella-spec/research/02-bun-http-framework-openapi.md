# Bun HTTP 框架与自动 OpenAPI 文档选型事实

- 票：`.scratch/stella-spec/issues/02-bun-http-framework-openapi.md`
- 检索日期：2026-07-19；本机 Bun 1.3.14
- 来源纪律：只引用一手来源（elysiajs.com、hono.dev、bun.sh/docs、官方 GitHub 仓库源码与 issue）。每条事实标注来源；标注 **[推断]** 的条目为基于这些事实的推理，非来源原文。
- 检索时版本（npm latest）：elysia 1.4.29、@elysia/openapi 1.4.15、hono 4.12.31、@hono/zod-openapi 1.5.1、@hono/swagger-ui 0.6.1。

---

## 0. 结论速览

| 维度 | Elysia + @elysia/openapi | Hono + @hono/zod-openapi | Bun.serve 原生 |
|---|---|---|---|
| Schema 校验 | 内置 TypeBox（`Elysia.t`），运行时+编译时+OpenAPI 同源；亦支持 Standard Schema（Zod 等） | 核心无校验；zod-openapi 用 Zod 校验 | 无内置，需自行集成 |
| OpenAPI 生成 | 全自动：从路由 schema 推断，一行 `.use(openapi())` | 半自动：按 OpenAPI 结构手写 createRoute（responses/description 必填） | 无 |
| 文档 UI | 内置挂载（默认 Scalar，可换 SwaggerUI），spec 在 `/openapi/json` | 需另挂 @hono/swagger-ui 中间件；spec 由 `app.doc()` 输出 | 无 |
| 类型安全 | 最强：schema→handler 类型推断 + Eden 端到端（类 tRPC） | 强：`c.req.valid()` 推断 + RPC 模式（hc） | 仅 TS 手写类型 |
| Bun 亲和 | 为 Bun 设计（亦可跑 Node/Deno/CF） | 多运行时，官方支持 Bun | Bun 独占 API |
| SSE/流式 | generator + `sse()` 工具 | `hono/streaming`：stream/streamText/streamSSE | `Response(async generator)`，注意 idleTimeout |
| WebSocket | `.ws()` 内置，消息可 schema 校验 | `hono/bun` upgradeWebSocket helper | 原生 websocket handler + pub/sub |
| compile 打包坑 | 有 open issue（minify 破坏 AOT、static 插件不内嵌、macro 解析）；fromTypes 需预生成 .d.ts | 未检索到不兼容 issue；官方文档缺 build/deploy 章节 | 官方文档完整覆盖；静态资产须 `with { type: "file" }` |
| 文档 UI 资源 | 默认 CDN（jsdelivr/unpkg），可配自托管 | 默认 CDN（jsdelivr），可配 baseUrl | — |

---

## 1. 候选框架对比

### 1.1 Elysia

**定位与 Bun 亲和**
- "Elysia is an ergonomic web framework for building backend servers with Bun… optimized for Bun." 来源：https://elysiajs.com/at-glance.html
- WinterTC 兼容，同一应用可部署到 Bun / Node.js / Deno / Cloudflare Worker / Vercel 等。来源：同上（"Platform Agnostic" 一节）。
- 官方自测 benchmark（2023-08-06，Bun 0.7.2，厂商数据仅供参考）：elysia(bun) 255,574 req/s vs hono(bun) 203,937 req/s。来源：同上（Performance 表）。

**Schema 校验**
- `Elysia.t` 是基于 TypeBox 的 schema builder："provides type-safety at runtime, compile-time, and OpenAPI schema generation from a single source of truth." 来源：https://elysiajs.com/essential/validation.html
- 同时支持 Standard Schema，可直接在路由上使用 Zod / Valibot / ArkType / Effect Schema / Yup / Joi 等，类型自动推断。来源：同上（"Standard Schema" 一节）。
- schema 类型（body/query/params/headers/cookie/response）作为路由第三参声明；"When a schema is provided, the type will be inferred from the schema automatically and an OpenAPI type will be generated for API documentation"。来源：同上。

**OpenAPI 文档生成**
- 现任官方插件是 **@elysia/openapi**（`bun add @elysia/openapi`），`.use(openapi())` 一行接入；访问 `/openapi` 得到文档 UI（默认 Scalar），原始 spec 在 `/openapi/json`。来源：https://elysiajs.com/plugins/openapi.html ；插件 README https://github.com/elysiajs/elysia-openapi
- **旧插件 @elysia/swagger 已废弃**，官方文档顶部警告："Swagger plugin is deprecated and is no longer maintained. Please use OpenAPI plugin instead." 来源：https://elysiajs.com/plugins/swagger.html
- 生成的文档遵循 OpenAPI 3.0.3（README `documentation` 字段 @see https://spec.openapis.org/oas/v3.0.3.html）。来源：https://github.com/elysiajs/elysia-openapi
- `detail` 字段扩展 OpenAPI Operation Object（summary/description/tags/hide/deprecated），documentation 字段可配 info、tags、securitySchemes（Bearer 等）。来源：https://elysiajs.com/plugins/openapi.html ；https://elysiajs.com/patterns/openapi.html
- `provider` 配置：`'scalar'`（默认）/ `'swagger-ui'` / `null`（只要 spec 不要 UI）。来源：https://elysiajs.com/plugins/openapi.html
- **OpenAPI from types**：`openapi({ references: fromTypes() })` 可直接从导出的 app 实例的 TS 类型生成 OpenAPI references，"This is equivalent to FastAPI's automatic OpenAPI generation from types but in TypeScript"；与 runtime schema 共存且 runtime schema 优先。来源：https://elysiajs.com/at-glance.html ；https://elysiajs.com/patterns/openapi.html
- 对 Standard Schema：Elysia 尝试用各 schema 库原生方法转 OpenAPI；Zod 需经 `mapJsonSchema` 提供映射（Zod 4 用 `z.toJSONSchema`，Zod 3 用 `zod-to-json-schema`），Valibot 用 `@valibot/to-json-schema`。来源：https://elysiajs.com/patterns/openapi.html ；https://elysiajs.com/plugins/openapi.html

**类型安全**
- handler context（params/query/body…）类型从 schema 自动推断，无需手写 TS。来源：https://elysiajs.com/at-glance.html（"TypeScript"/"Type Integrity"）
- Eden（`@elysiajs/eden` 的 treaty）提供端到端类型安全：客户端直接消费 server 的 `typeof app` 类型，无需代码生成，覆盖错误分支（type soundness）。来源：https://elysiajs.com/at-glance.html（"End-to-end Type Safety"/"Type Soundness"）
- `status()` 函数返回带类型窄化的状态码，配合 response schema 做编译时校验。来源：https://elysiajs.com/essential/handler.html

### 1.2 Hono

**定位与 Bun 亲和**
- 多运行时框架，官方 Bun 上手文档：`bun add hono`，`export default app` 即由 Bun 直接运行（Bun 识别含 fetch 的默认导出）。来源：https://hono.dev/docs/getting-started/bun

**Schema 校验与 OpenAPI**
- Hono 核心不带 OpenAPI；官方路线是 **@hono/zod-openapi**："an extended Hono class that supports OpenAPI. With it, you can validate values and types using Zod and generate OpenAPI Swagger documentation"，底层基于 zod-to-openapi（asteasolutions）。来源：https://github.com/honojs/middleware/tree/main/packages/zod-openapi（README）
- 写法：`createRoute({ method, path, request, responses })` 按 OpenAPI 结构显式声明（每个 response 需 content+description），`app.openapi(route, handler)` 注册，`app.doc('/doc', { openapi: '3.0.0', info })` 暴露 spec JSON。来源：同上
- OpenAPI 3.1：`app.doc31('/docs', …)` / `getOpenAPI31Document()`。来源：同上（"OpenAPI v3.1"）
- Swagger UI 由独立包 **@hono/swagger-ui** 提供：`app.get('/ui', swaggerUI({ url: '/doc' }))`。来源：https://github.com/honojs/middleware/tree/main/packages/swagger-ui（README）
- **[对比推断]** zod-openapi 是"OpenAPI 文档优先"的路由声明方式：description、responses 结构都要手写，自动推断成分少于 Elysia（Elysia 从普通路由的 schema 直接产出文档，还可从纯 TS 类型生成 references）。

**类型安全**
- handler 内 `c.req.valid('param'|'json'|…)` 获得校验后类型。来源：zod-openapi README
- 支持 RPC 模式：`hc<typeof appRoutes>()` 得到类型化客户端。来源：同上（"RPC Mode"）
- README 明示的坑：
  - 请求 Content-Type 不对时 `c.req.valid('json')` 返回 `{}`（除非 `request.body.required = true`）。
  - header key 必须小写。
  - OpenAPIHono 与普通 Hono 混用有限制；`.route()` 挂载子 app 时路径参数须用 Hono 的 `:param` 语法而非 `{param}`。
  来源：zod-openapi README（Limitations 等节）。

### 1.3 Bun.serve 原生

- `Bun.serve()` 是 Bun 内置高性能 HTTP server；v1.2.3+ 支持 `routes` 声明式路由（静态路由直接映射 `Response`/`Bun.file`，动态路由支持 `:param` 与通配），`fetch` 作 fallback。来源：https://bun.sh/docs/api/http
- 官方 HTTP 文档全文未提及 OpenAPI / Swagger（对 https://bun.sh/docs/api/http 全文检索无命中）。**[推断]** 走原生路线时，schema 校验（如 Zod/TypeBox）与 OpenAPI 文档（如手写 spec + 自托管 Swagger UI）均需自行集成，无框架辅助。
- HTML imports 可构建 full-stack 应用（`import index from "./index.html"` 配合 routes）。来源：https://bun.sh/docs/api/http（"HTML imports"）

---

## 2. 流式响应（SSE / chunked）

### 2.1 Elysia
- handler 写 generator function（`function*` + `yield`）即为流式响应。来源：https://elysiajs.com/essential/handler.html（"Stream"）
- 内置 `sse()` 工具：yield 被 `sse()` 包裹的值时，Elysia 自动设置 `text/event-stream` 并格式化 SSE event（支持 event/data 字段）。来源：同上（"Server Sent Events (SSE)"）
- headers 只能在第一个 chunk yield 之前设置；之后修改无效。来源：同上
- 客户端中断请求时 Elysia 自动停止 generator（automatic cancellation）。来源：同上
- 条件流：无 yield 而直接 return 时自动转为普通响应。来源：同上
- Eden 客户端把流式响应解释为 `AsyncGenerator`，可 `for await` 消费。来源：同上

### 2.2 Hono
- `hono/streaming` 提供三个 helper：
  - `stream(c, cb)`：通用流式 Response，支持 `stream.onAbort()`、`write(Uint8Array)`、`pipe(ReadableStream)`。
  - `streamText`：`text/plain` + `Transfer-Encoding: chunked`，支持 `writeln`/`sleep`。
  - `streamSSE`：`stream.writeSSE({ data, event, id })` 发 SSE。
  来源：https://hono.dev/docs/helpers/streaming
- 第三参可传错误处理器；文档警告 stream 回调抛错不会触发 Hono 的 onError（响应已开始无法覆写）。来源：同上

### 2.3 Bun.serve 原生
- `Response` 可直接接收 async generator 作 body 实现流式下发，官方示例即为 SSE：
  `new Response(async function* () { yield "data: hello\n\n" }, { headers: { "Content-Type": "text/event-stream" } })`。来源：https://bun.sh/docs/api/http（"server.timeout(Request, seconds)" 一节）
- **重要坑**：`idleTimeout` 默认 10 秒，流静默超时会被 Bun 断连；长寿命流（SSE）需 `server.timeout(req, 0)` 对该请求关闭超时（max 255s，0 为禁用）。来源：https://bun.sh/docs/api/http（"idleTimeout" 与 "Streaming & Server-Sent Events" 提示框）

---

## 3. 与 `bun build --compile` 的兼容性

### 3.1 Bun 官方机制（文档事实）
来源均为 https://bun.sh/docs/bundler/executables ，除单独标注外。

- `bun build --compile` 把入口及全部 import 的文件、npm 包连同 Bun 运行时打成单文件；"All built-in Bun and Node.js APIs are supported."
- 交叉编译：`--target=bun-windows-x64` / `bun-linux-x64`（含 baseline/modern/musl/arm64 变体）等；Windows 目标自动补 `.exe`。
- **静态资产内嵌**：须用 import attribute `import icon from "./icon.png" with { type: "file" }`；构建期嵌入二进制并改写为 `/$bunfs/` 内部路径，运行时用 `Bun.file(path)` 或 `node:fs` 读取；`Bun.embeddedFiles` 可枚举；`--asset-naming="[name].[ext]"` 去 hash。目录内嵌需把 glob 展开后列入 entrypoints（官方称其为 workaround）。
- **HTML import / full-stack executable**（v1.2.17+）：server import HTML 时前端资产一并打包，Bun.serve 直接服务。
- **动态导入**：`--compile --splitting` 支持 code splitting，懒加载 chunk 打入二进制。
- **Worker**：worker 文件必须显式列为 entrypoint；"If you use a relative path to a file not included in the standalone executable, Bun loads that path from disk relative to the process's current working directory, and errors if it doesn't exist."
- 生产推荐：`bun build --compile --minify --sourcemap`（可选 `--bytecode`）。
- SQLite：`bun:sqlite` 可用，默认相对 CWD 解析 db 文件；`with { type: "sqlite", embed: "true" }` 可内嵌（内存态，退出即失）。
- Windows 专属 flags（icon、hide-console、元数据）交叉编译时不可用。

### 3.2 Elysia：官方说明与已知 issue

**官方文档对单文件编译的直接说明**
- Patterns: OpenAPI 文档设 "Production" 一节：*"In production environment, it's likely that you might compile Elysia to a single executable with Bun or bundle into a single JavaScript file. It's recommended that you should pre-generate the declaration file (.d.ts) to provide type declaration to the generator."* 即使用 `fromTypes()` 时，编译部署须预生成 `.d.ts` 并传入（示例按 NODE_ENV 切换 `dist/index.d.ts` / `src/index.ts`）。来源：https://elysiajs.com/patterns/openapi.html
- Elysia 内置 JIT "compiler"（`aot` 选项，默认 `true` 启动前预编译全部路由；`false` 关闭 JIT 进入 dynamic mode，启动更快）。来源：https://elysiajs.com/patterns/configuration.html（"aot" 一节）

**官方仓库 open issue（检索于 2026-07-19，均为 open）**
- elysiajs/elysia#1713 「staticPlugin cannot embed static files to singlefile」：@elysiajs/static 在 `Bun.build({ compile })` 产物中 `Bun.embeddedFiles` 为空、换路径/换机 ENOENT；复现于 Bun 1.3.8 + Elysia 1.4.22。https://github.com/elysiajs/elysia/issues/1713
- elysiajs/elysia#1711 「`bun build --minify` breaks Elysia app in some conditions」：`bun build --compile --minify` 后运行即报错（AOT 生成代码被压缩破坏）；复现于 Bun 1.3.8 + Elysia 1.4.1。https://github.com/elysiajs/elysia/issues/1711
- elysiajs/elysia#1280 「Sucrose can't parse my macro correctly when using bun build」：含复杂 `macro` 时 bun build 下 Sucrose 静态分析失败。https://github.com/elysiajs/elysia/issues/1280
- **[推断]** Elysia 的 AOT 在运行时做代码生成（new Function 式 JIT），编译产物内仍是完整 Bun/JSC 运行时，机制本身与单文件不冲突；但上述 issue 表明 `--minify` 与 macro 等场景在当前版本有实际风险——若走 Elysia 路线，compile 时应先不开 `--minify` 验证，或跟踪 #1711/#1280。

**Swagger UI / Scalar 前端资源不内嵌，默认走 CDN**
- @elysia/openapi 源码：Scalar bundle 默认 `https://cdn.jsdelivr.net/npm/@scalar/api-reference@${version ?? 'latest'}/dist/browser/standalone.min.js`；SwaggerUI provider 默认 `https://unpkg.com/swagger-ui-dist@4.18.2/swagger-ui-bundle.js`（css 同源）。来源：https://github.com/elysiajs/elysia-openapi/blob/main/src/index.ts ；https://github.com/elysiajs/elysia-openapi/blob/main/src/swagger/index.ts ；类型注释见 https://github.com/elysiajs/elysia-openapi/blob/main/src/types.ts
- 官方文档给出自托管方式："Self-hosted Scalar bundle"——`openapi({ scalar: { cdn: "/public/scalar-standalone.min.js", withDefaultFonts: false } })`，cdn 选项可覆盖 bundle URI。来源：https://elysiajs.com/plugins/openapi.html
- spec JSON 由服务端在 `specPath`（默认 `/openapi/json`）动态生成；Scalar 渲染时 spec 以 `content` 内嵌进页面配置。来源：https://github.com/elysiajs/elysia-openapi/blob/main/src/scalar/index.ts
- **[推断]** 单文件二进制离线/内网运行时，文档 UI 的 JS/CSS 需自托管：把 scalar/swagger-ui-dist 的 bundle 用 `with { type: "file" }` 内嵌并经静态路由吐出（Elysia 侧注意 #1713，可用 Bun.serve static routes 或手工路由替代 @elysiajs/static），再把 `scalar.cdn` 指向该本地路径；仅暴露 spec JSON（`/openapi/json`）则完全不受 CDN 影响。

### 3.3 Hono：已知情况

- 检索 honojs/hono（"bun build compile" 全文 23 条、"compile in:title" 1 条）与 oven-sh/bun（"hono in:title" 28 条），**未发现** Hono 与 `bun build --compile` 不兼容的 open issue（检索日 2026-07-19；未发现≠不存在）。
- honojs/hono#2010（open）"Add build/deploy section for node and bun in docs"：官方 getting-started 文档缺 build/deploy（含生产打包）章节。https://github.com/honojs/hono/issues/2010
- @hono/swagger-ui 的前端资源同样是远端加载：源码 `remoteAssets()` 默认 `https://cdn.jsdelivr.net/npm/swagger-ui-dist`，可用 `baseUrl`（如内网镜像）与 `version` 覆盖，或 `manuallySwaggerUIHtml` 完全自定义 HTML。来源：https://github.com/honojs/middleware/blob/main/packages/swagger-ui/src/swagger/resource.ts ；README
- **[推断]** Hono 无运行时代码生成机制（文档无 AOT/JIT 概念，路由为常规查表），对 `--minify` 类压缩破坏的敏感度应低于 Elysia；但这属推理，应以实测为准。

### 3.4 Bun.serve 原生
- 无框架层，官方 compile 文档即全部契约（见 3.1）；静态文件官方推荐路径就是 `with { type: "file" }` + `Bun.file()` + `Bun.serve({ static })`。来源：https://bun.sh/docs/bundler/executables（"Embed assets & files"、"Serving static assets in an HTTP server"）
- **[推断]** 若自研薄层（路由+校验+OpenAPI 生成手写），compile 风险面最小，但 OpenAPI 生成与 UI 需完全自建。

---

## 4. WebSocket 服务端能力（备用）

### 4.1 Elysia
- `.ws('/ws', { open/message/close/drain })` 内置支持；"Elysia uses uWebSocket which Bun uses under the hood with the same API." 来源：https://elysiajs.com/patterns/websocket.html
- 消息可用与 HTTP 路由相同的 schema 校验（body=消息本体、query、params、headers、cookie、response），默认把 JSON 字符串消息解析为对象再校验。来源：同上
- 配置直通 Bun WebSocket（perMessageDeflate 默认关、maxPayloadLength、idleTimeout 默认 120s、backpressureLimit 默认 16MB、closeOnBackpressureLimit）。来源：同上

### 4.2 Hono
- Bun 适配：`import { upgradeWebSocket, websocket } from 'hono/bun'`，`export default { fetch: app.fetch, websocket }`；事件 onOpen/onMessage/onClose/onError。来源：https://hono.dev/docs/helpers/websocket
- 支持 RPC 模式（`hc` 客户端 `client.ws.$ws()`）。来源：同上
- 文档警告：在 WS 路由上使用会改 headers 的中间件（如 CORS）会报 immutable headers 错。来源：同上

### 4.3 Bun.serve 原生
- 原生服务端 WebSocket：`server.upgrade(req)` 升级，`websocket: { open, message, close, drain }` 每服单份 handler（性能设计）；`ServerWebSocket.send` 支持 string/ArrayBuffer 等；内置 pub/sub（`ws.subscribe`/`server.publish`）、per-message deflate 压缩、背压（`ws.data` 缓冲与 drain）。来源：https://bun.sh/docs/api/websockets
- 基于 uWebSockets；官方 benchmark 称约 7 倍于 Node.js + ws（Bun v0.2.1 数据）。来源：同上

---

## 5. 对 Stella 的启示（推断，非来源事实）

1. **需求契合度**："自动 swag 风格文档 + 少手写" 与 Elysia 的设计完全同构（schema 即文档，`@elysia/openapi` 零注解出 UI + spec）。Hono 路线可行但文档声明成本更高（createRoute 全量手写）。Bun.serve 原生需自建全部 OpenAPI 设施，与"自动生成"目标相悖。
2. **流式对话接口**：三家都能做 SSE；Elysia 的 generator + `sse()` 写法最简且自带取消语义，与 LLM token 流直推契合。注意 Bun 层 idleTimeout 语义对三家都适用（底层都是 Bun.serve），长静默流要保活或 `server.timeout(req, 0)`。
3. **compile 风险排序**（推断）：Elysia 当前存在 minify/macro/static 三个 open issue，打包需验证矩阵（关 minify、避开 @elysiajs/static、不用或慎用 macro/fromTypes，或按官方指引预生成 .d.ts）；Hono 未见不兼容报告但官方无打包文档，需自行验证；原生最稳但自建成本最高。
4. **文档 UI 离线化**：无论选谁，Scalar/SwaggerUI 前端 bundle 默认 CDN，单文件分发场景要规划自托管内嵌（`with { type: "file" }` + 静态路由 + 覆盖 cdn/baseUrl）。
5. **WebSocket 备用**：三家皆可用；Elysia `.ws()` 自带消息 schema 校验，与现有校验体系同源。

---

## 6. 来源清单

官方文档：
- Elysia At a Glance：https://elysiajs.com/at-glance.html
- Elysia Validation：https://elysiajs.com/essential/validation.html
- Elysia Handler（Stream/SSE）：https://elysiajs.com/essential/handler.html
- Elysia OpenAPI Plugin：https://elysiajs.com/plugins/openapi.html
- Elysia Swagger Plugin（deprecated 声明）：https://elysiajs.com/plugins/swagger.html
- Elysia Patterns: OpenAPI（fromTypes / Production 一节）：https://elysiajs.com/patterns/openapi.html
- Elysia Patterns: Configuration（aot）：https://elysiajs.com/patterns/configuration.html
- Elysia Patterns: WebSocket：https://elysiajs.com/patterns/websocket.html
- Hono Getting Started: Bun：https://hono.dev/docs/getting-started/bun
- Hono Streaming Helper：https://hono.dev/docs/helpers/streaming
- Hono WebSocket Helper：https://hono.dev/docs/helpers/websocket
- Hono Examples: Zod OpenAPI：https://hono.dev/examples/zod-openapi
- Bun Single-file executable：https://bun.sh/docs/bundler/executables
- Bun HTTP server：https://bun.sh/docs/api/http
- Bun WebSockets：https://bun.sh/docs/api/websockets

官方仓库（源码/README）：
- elysiajs/elysia-openapi（@elysia/openapi）：https://github.com/elysiajs/elysia-openapi （src/index.ts、src/scalar/index.ts、src/swagger/index.ts、src/types.ts）
- honojs/middleware → @hono/zod-openapi：https://github.com/honojs/middleware/tree/main/packages/zod-openapi
- honojs/middleware → @hono/swagger-ui：https://github.com/honojs/middleware/tree/main/packages/swagger-ui （src/swagger/resource.ts、src/swagger/renderer.ts）

官方仓库 issue：
- https://github.com/elysiajs/elysia/issues/1713 （staticPlugin 与单文件，open）
- https://github.com/elysiajs/elysia/issues/1711 （--minify 破坏 Elysia，open）
- https://github.com/elysiajs/elysia/issues/1280 （Sucrose macro 与 bun build，open）
- https://github.com/honojs/hono/issues/2010 （官方文档缺 build/deploy 章节，open）
