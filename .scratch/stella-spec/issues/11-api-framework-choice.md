# API 框架选型

Type: grilling
Status: closed (2026-07-19)
Blocked by: 02

## Question

[Bun HTTP 框架与自动文档选型事实] 已给出事实，本票拍板 v1 的 API 框架：

- Elysia + @elysia/openapi：文档全自动（schema 即文档、零注解出 Swagger UI）、类型安全最强，但 `bun build --compile` 有 3 个 open issue（minify 破坏、static 插件不内嵌、macro 解析），需规避写法或绕路；文档 UI 前端默认走 CDN，单文件离线分发须自托管。
- Hono + @hono/zod-openapi：文档需按 OpenAPI 结构手写 createRoute，UI 另挂，但无 compile 不兼容报告。
- 取舍点：开发体验与类型安全 vs 单文件二进制的确定性。

## Resolution

**选 Elysia + @elysia/openapi**。理由：API 使命核心诉求 = swag 自动文档，Elysia schema 即文档零注解（Hono 手写 createRoute 会随端点漂移）；compile 三个 open issue 均有规避写法且 Stella API 面（纯 JSON + SSE）对其无感；SSE generator 写法与 LLM token 流最契合；决策可逆（API 层薄，踩坑降级 Hono 成本低）。

附**四条写法纪律**：不开 `--minify`（#1711）；不用 `@elysiajs/static`，静态资产走 `with { type: "file" }` + 手工路由（#1713）；不用 macro（#1280）；不用 `fromTypes`（或按官方指引预生成 .d.ts）。**实施任务**：`bun build --compile` + 关 minify 的打包含实测验收。文档 UI（Scalar）自托管：内嵌资产 + `scalar.cdn` 指本地路径。
