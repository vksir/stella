# Bun HTTP 框架与自动文档选型事实

Type: research
Status: resolved

## Question

Stella 的 API 平台跑在 Bun 上，要求自动生成 swag（OpenAPI/Swagger UI）文档，且最终用 `bun build --compile` 打成单文件二进制。选型需要事实：

1. 候选框架对比：Elysia（+ @elysiajs/swagger）、Hono（+ @hono/zod-openapi）、Bun.serve 原生。各自的 schema 校验与 OpenAPI 文档生成能力、类型安全程度。
2. 流式响应能力：SSE 或 chunked streaming 在各候选上的支持情况（对话接口需要流式下发回复）。
3. 与 `bun build --compile` 的兼容性：Swagger UI 静态资源、动态导入等是否有打包坑（有无已知 issue/官方说明）。
4. WebSocket 服务端能力（备用：若平台层需要）。

事实须引用一手来源（各框架官方文档、Bun 官方文档、官方仓库 issue）。

## Answer

- OpenAPI 自动程度：Elysia（@elysia/openapi，schema 即文档、零注解出 UI+spec，旧 @elysia/swagger 已废弃）> Hono（@hono/zod-openapi 需按 OpenAPI 结构手写 createRoute，UI 另挂 @hono/swagger-ui）> Bun.serve 原生（无校验/无文档设施，全自建）。
- 流式：三家皆可 SSE——Elysia 用 generator+sse()（自带取消语义），Hono 用 hono/streaming 的 streamSSE，原生用 Response(async generator)；底层 Bun idleTimeout 默认 10s，长流需 server.timeout(req, 0)。
- 类型安全：Elysia 最强（TypeBox 同源校验+Eden 端到端）；Hono 有 c.req.valid 推断与 RPC 模式。
- compile 兼容性：Bun 官方支持资产内嵌（with { type: "file" }）；Elysia 有 3 个 open issue（#1711 minify 破坏、#1713 static 插件不内嵌、#1280 macro 解析），fromTypes 需预生成 .d.ts；Hono 未见不兼容 issue 但官方无打包文档。
- 文档 UI 前端（Scalar/SwaggerUI）默认走 CDN 不内嵌，单文件离线分发须自托管 bundle 并覆盖 cdn/baseUrl。
- WebSocket：三家均可用，Elysia .ws() 带消息 schema 校验。

详见 [research/02-bun-http-framework-openapi.md](../research/02-bun-http-framework-openapi.md)
