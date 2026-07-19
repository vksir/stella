import { Elysia } from "elysia";
import { openapi } from "@elysia/openapi";
import type { AppContext } from "../index";
import { sessionRoutes } from "./sessions";
import { memoryRoutes } from "./memories";
import { userRoutes } from "./users";

/**
 * API 服务器返回句柄：包含停止方法与监听信息。
 */
export interface ApiServerHandle {
  stop(): Promise<void>;
  server: { port: number; hostname: string };
}

/**
 * 启动 Elysia HTTP API 服务。
 *
 * 端点：
 * - POST /sessions           — 新建会话
 * - POST /sessions/:id/messages — 发消息（SSE 流式返回）
 * - GET  /sessions           — 列出全部 API 会话
 * - GET  /sessions/:id       — 会话详情（含消息历史）
 * - DELETE /sessions/:id     — 删除会话
 * - GET  /users/:id/memories — 列出某用户的记忆（鉴权）
 * - PUT  /memories/:id       — 更新记忆内容（鉴权）
 * - DELETE /memories/:id     — 删除记忆（鉴权）
 * - GET  /users              — 用户列表
 * - POST /users/:id/merge    — 合并用户（admin）
 * - /openapi                 — Scalar 文档 UI
 * - /openapi/json            — OpenAPI spec JSON
 */
export async function startApiServer(ctx: AppContext): Promise<ApiServerHandle> {
  const { config, sessionStore, memoryStore, users, identityStore, identity, sessions, setSessionUser, transaction } = ctx;

  const app = new Elysia()
    // OpenAPI 插件
    .use(
      openapi({
        path: "/openapi",
        provider: "scalar",
      }),
    )

    // 路由模块（Elysia 跨模块泛型不兼容，用 any 桥接）
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    .use(sessionRoutes(sessionStore, sessions, identity, setSessionUser) as any)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    .use(memoryRoutes(memoryStore, identity) as any)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    .use(userRoutes(users, identityStore, memoryStore, identity, transaction) as any)

    // 全局错误处理：将 statusError 转为 HTTP 响应
    .onError(({ error }) => {
      const err = error as Error & { status?: number };
      if (err?.status) {
        return new Response(JSON.stringify({ error: err.message }), {
          status: err.status,
          headers: { "Content-Type": "application/json" },
        });
      }
      console.error("[API] 未处理错误:", error);
      return new Response(JSON.stringify({ error: "Internal server error" }), {
        status: 500,
        headers: { "Content-Type": "application/json" },
      });
    });

  // 解析监听地址
  const [hostname, portStr] = config.api.listen.split(":");
  const port = portStr ? parseInt(portStr, 10) : 3000;

  app.listen({
    hostname: hostname ?? "127.0.0.1",
    port,
  });

  const actualHostname = app.server?.hostname ?? hostname ?? "127.0.0.1";
  const actualPort = app.server?.port ?? port;

  console.log(`[Stella] API 服务已启动: http://${actualHostname}:${actualPort}`);

  return {
    stop: async () => {
      app.stop();
    },
    server: {
      port: actualPort,
      hostname: actualHostname,
    },
  };
}
