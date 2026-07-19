import { Elysia } from "elysia";
import type { UserRow } from "./types";
import type { SessionStore } from "../stores/session";
import type { SessionRegistry } from "../sessions-registry";
import type { IdentityResolver } from "../identity";
import { authenticate } from "./auth";

export function sessionRoutes(
  sessionStore: SessionStore,
  sessions: SessionRegistry,
  identity: IdentityResolver,
  setSessionUser: (sessionId: string, user: UserRow) => void,
) {
  const app = new Elysia()

    // POST /sessions — 新建会话
    .post(
      "/sessions",
      async ({ headers }) => {
        authenticate(headers["authorization"], identity);

        const sessionId = crypto.randomUUID();
        const chatKey = `api:${sessionId}`;
        await sessions.getOrCreate("api", chatKey);

        return new Response(
          JSON.stringify({ session_id: sessionId }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        );
      },
      {
        detail: {
          summary: "新建会话",
          description: "创建一个新的 API 会话，返回唯一的 session_id。",
        },
      },
    )

    // GET /sessions — 列出全部 API 会话
    .get(
      "/sessions",
      ({ headers }) => {
        authenticate(headers["authorization"], identity);

        const rows = sessionStore.listByPlatform("api");
        return rows.map((r) => ({
          chat_key: r.chat_key,
          created_at: r.created_at,
          last_active: r.last_active,
        }));
      },
      {
        detail: {
          summary: "列出全部 API 会话",
          description: "返回 platform='api' 的所有会话记录。",
        },
      },
    )

    // GET /sessions/:id — 会话详情（含历史消息）
    .get(
      "/sessions/:id",
      async ({ headers, params: { id } }) => {
        authenticate(headers["authorization"], identity);

        const chatKey = `api:${id}`;
        const row = sessionStore.get("api", chatKey);
        if (!row) {
          const err = new Error("Session not found") as Error & { status: number };
          err.status = 404;
          throw err;
        }

        let messages: unknown[] = [];
        if (sessions.has("api", chatKey)) {
          const session = await sessions.getOrCreate("api", chatKey);
          messages = (session.messages as Array<{ role?: string; content?: unknown }>)
            .filter((m) => m.role === "user" || m.role === "assistant")
            .map((m) => ({
              role: m.role,
              content: typeof m.content === "string" ? m.content : JSON.stringify(m.content),
            }));
        }

        return {
          session_id: id,
          chat_key: chatKey,
          created_at: row.created_at,
          last_active: row.last_active,
          messages,
        };
      },
      {
        detail: {
          summary: "会话详情",
          description: "获取指定会话的基本信息及历史消息。",
        },
      },
    )

    // DELETE /sessions/:id — 删除会话
    .delete(
      "/sessions/:id",
      ({ headers, params: { id }, set }) => {
        authenticate(headers["authorization"], identity);

        sessionStore.delete_("api", `api:${id}`);

        set.status = 204;
        return "";
      },
      {
        detail: {
          summary: "删除会话",
          description: "删除指定的 API 会话记录。",
        },
      },
    )

    // POST /sessions/:id/messages — 发消息（SSE 流式返回）
    .post(
      "/sessions/:id/messages",
      async function* ({ headers, params: { id }, body, request, set }: any) {
        const user = authenticate(headers["authorization"], identity);
        const { content } = body as { content: string };
        if (!content || typeof content !== "string") {
          const err = new Error("content required") as Error & { status: number };
          err.status = 400;
          throw err;
        }

        const chatKey = `api:${id}`;
        const session = await sessions.getOrCreate("api", chatKey);
        setSessionUser(session.sessionId, user);

        const chunks: string[] = [];
        let error: string | null = null;
        const unsub = session.subscribe((event) => {
          if (event.type === "message_update") {
            const e = (event as unknown as { assistantMessageEvent?: { type?: string; delta?: string; text?: string } }).assistantMessageEvent;
            if (e?.delta && (e.type === "text_delta" || e.type === "thinking_delta")) chunks.push(e.delta);
          }
        });

        const promptPromise = session.prompt(content).catch((err) => { error = String(err); });

        const rawReq = request as unknown as { timeout?: (ms: number) => void };
        if (typeof rawReq?.timeout === "function") rawReq.timeout(0);

        set.headers["Content-Type"] = "text/event-stream";
        set.headers["Cache-Control"] = "no-cache";

        let keepAliveTimer: ReturnType<typeof setInterval> | null = null;
        try {
          keepAliveTimer = setInterval(() => { chunks.push("__ka__"); }, 5000);

          while (!session.isStreaming && chunks.length === 0) {
            await new Promise((r) => setTimeout(r, 10));
          }

          while (session.isStreaming || chunks.length > 0) {
            while (chunks.length > 0) {
              const c = chunks.shift()!;
              if (c === "__ka__") yield ":keepalive\n\n";
              else yield `data: ${JSON.stringify({ token: c })}\n\n`;
            }
            if (session.isStreaming) await new Promise((r) => setTimeout(r, 50));
          }
          await promptPromise;
          if (error) yield `data: ${JSON.stringify({ error })}\n\n`;
          yield "data: [DONE]\n\n";
        } finally {
          if (keepAliveTimer) clearInterval(keepAliveTimer);
          unsub();
        }
      },
      {
        detail: { summary: "发送消息", description: "SSE 流式返回 AI 回复" },
      },
    );

  return app;
}
