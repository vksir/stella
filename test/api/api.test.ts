import { describe, expect, it, beforeAll, afterAll } from "bun:test";
import { resolve } from "node:path";
import { existsSync, mkdirSync, rmSync, unlinkSync } from "node:fs";
import { Database } from "bun:sqlite";
import { runMigrations } from "../../src/stores/schema";
import { transaction } from "../../src/stores/transaction";
import { createUserStore, type UserStore, type UserRow } from "../../src/stores/user";
import { createIdentityStore, type IdentityStore } from "../../src/stores/identity";
import { createSessionStore, type SessionStore } from "../../src/stores/session";
import { createMemoryStore, type MemoryStore } from "../../src/stores/memory";
import { createIdentityResolver } from "../../src/identity";
import { SessionRegistry, type ISessionFactory, type SessionCreateResult } from "../../src/sessions-registry";
import { createMemoryTools } from "../../src/tools/memory";
import { startApiServer } from "../../src/api/api";
import type { AppContext } from "../../src/index";
import type { StellaConfig } from "../../src/config";
import type { AgentSession, AgentSessionEvent } from "@earendil-works/pi-coding-agent";

const tmpDir = resolve(import.meta.dir, "..", "..", ".test", "api-server");
const dbPath = resolve(tmpDir, "test-api.db");
const dataDir = resolve(tmpDir, "data");
const sessionsDir = resolve(dataDir, "sessions");

let db_sql: Database;
let users: UserStore;
let identityStore: IdentityStore;
let sessionStore: SessionStore;
let memoryStore: MemoryStore;
let testConfig: StellaConfig;
let appCtx: AppContext;
let serverUrl: string;
let server: { stop: () => Promise<void> } | null = null;

const OWNER_TOKEN = "master-token-test";
const GUEST_TOKEN = "guest-token-test";

// ---- 会话模拟 ----
interface MockSession {
  session: AgentSession;
  dispose: () => void;
  sessionPath: string;
}

function makeMockSession(): MockSession {
  const listeners: Array<(event: AgentSessionEvent) => void> = [];
  let _streaming = false;
  const sessionMessages: unknown[] = [];
  const sessionId = `mock-${Math.random().toString(36).slice(2, 10)}`;

  const session = {
    sessionId,
    sessionFile: `${sessionsDir}/${sessionId}.jsonl`,
    get messages() {
      return sessionMessages;
    },
    get isStreaming() {
      return _streaming;
    },
    set isStreaming(v: boolean) {
      _streaming = v;
    },
    subscribe(listener: (event: AgentSessionEvent) => void): () => void {
      listeners.push(listener);
      return () => {
        const idx = listeners.indexOf(listener);
        if (idx >= 0) listeners.splice(idx, 1);
      };
    },
    async prompt(text: string): Promise<void> {
      _streaming = true;

      const chars = `[回复] ${text}`;
      const msgId = `msg-${Date.now()}`;

      for (let i = 0; i < chars.length; i++) {
        await new Promise((r) => setTimeout(r, 5));
        for (const l of listeners) {
          l({
            type: "message_update",
            assistantMessageEvent: {
              type: "text_delta",
              delta: chars[i]!,
              messageId: msgId,
            },
          } as unknown as AgentSessionEvent);
        }
      }

      for (const l of listeners) {
        l({
          type: "agent_end",
          messages: [],
          willRetry: false,
        } as AgentSessionEvent);

        l({
          type: "agent_settled",
        } as AgentSessionEvent);
      }

      _streaming = false;

      sessionMessages.push(
        { role: "user", content: text },
        { role: "assistant", content: `[回复] ${text}` },
      );
    },
    dispose: () => {
      listeners.length = 0;
    },
  };

  return { session: session as unknown as AgentSession, dispose: () => session.dispose(), sessionPath: session.sessionFile };
}

const mockFactory: ISessionFactory = {
  async create(_platform: string, _chatKey: string): Promise<SessionCreateResult> {
    const mock = makeMockSession();
    return { session: mock.session, dispose: mock.dispose, sessionPath: mock.sessionPath };
  },
};

function authHeaders(token: string): Record<string, string> {
  return { Authorization: `Bearer ${token}` };
}

beforeAll(async () => {
  if (!existsSync(tmpDir)) mkdirSync(tmpDir, { recursive: true });
  if (existsSync(dbPath)) unlinkSync(dbPath);
  if (!existsSync(dataDir)) mkdirSync(dataDir, { recursive: true });
  if (!existsSync(sessionsDir)) mkdirSync(sessionsDir, { recursive: true });

  db_sql = new Database(dbPath);
  runMigrations(db_sql);

  users = createUserStore(db_sql);
  identityStore = createIdentityStore(db_sql);
  sessionStore = createSessionStore(db_sql);
  memoryStore = createMemoryStore(db_sql, 200);

  testConfig = {
    owner: { qq: "99999", api_token: OWNER_TOKEN },
    napcat: { listen: "0.0.0.0:8082", token: "x" },
    api: { listen: "127.0.0.1:0" },
    model: { provider: "test", name: "test" },
    paths: { data_dir: dataDir },
    memory: { max_entries_per_user: 200, max_content_chars: 2000 },
    short_term_memory: { max_tokens: 10000, max_age_days: 3 },
    tools: { whitelist: [] },
  };

  const identity = createIdentityResolver(users, identityStore, testConfig);

  // 预创建管理员和 guest 用户
  identity.resolve("api", OWNER_TOKEN);
  identity.resolve("api", GUEST_TOKEN);

  const sessionUserMap = new Map<string, UserRow>();
  function getUserForSession(s: string): UserRow | null {
    return sessionUserMap.get(s) ?? null;
  }
  function setSessionUser(s: string, u: UserRow): void {
    sessionUserMap.set(s, u);
  }

  const memoryTools = createMemoryTools(memoryStore, testConfig.memory.max_content_chars, getUserForSession);
  const sessions = new SessionRegistry(sessionStore, mockFactory);
  const tx = <T>(fn: () => T): T => transaction(db_sql, fn);

  appCtx = {
    config: testConfig,
    dataDir,
    users,
    identityStore,
    sessionStore,
    memoryStore,
    identity,
    sessions,
    memoryTools,
    transaction: tx,
    setSessionUser,
    apiServer: null!,
    qqAdapter: null,
  } as AppContext;

  const handle = await startApiServer(appCtx);
  appCtx.apiServer = handle;
  server = handle;
  serverUrl = `http://127.0.0.1:${handle.server.port}`;
});

afterAll(async () => {
  if (server) {
    try { await server.stop(); } catch { /* ok */ }
  }
  db_sql.close();
  try { rmSync(tmpDir, { recursive: true }); } catch { /* ok */ }
});

// ---- 辅助函数 ----
function createTestUser(name: string, token: string) {
  const id = users.create(name, "guest");
  identityStore.link(id, "api", token);
  return id;
}

function createTestMemory(userId: number, content: string) {
  return memoryStore.create(userId, content);
}

// ---- 测试 ----
describe("API 鉴权", () => {
  it("无 Authorization header → 401", async () => {
    const res = await fetch(`${serverUrl}/sessions`);
    expect(res.status).toBe(401);
  });

  it("非 Bearer 格式 → 401", async () => {
    const res = await fetch(`${serverUrl}/sessions`, {
      headers: { Authorization: "Basic xyz" },
    });
    expect(res.status).toBe(401);
  });

  it("未知 token 自动建档 guest → 200", async () => {
    const res = await fetch(`${serverUrl}/sessions`, {
      headers: authHeaders("unknown-new-token"),
    });
    expect(res.status).toBe(200);
  });

  it("有效 token → 200", async () => {
    const res = await fetch(`${serverUrl}/sessions`, {
      headers: authHeaders(OWNER_TOKEN),
    });
    expect(res.status).toBe(200);
  });
});

describe("POST /sessions", () => {
  it("创建会话返回 201 + session_id", async () => {
    const res = await fetch(`${serverUrl}/sessions`, {
      method: "POST",
      headers: authHeaders(OWNER_TOKEN),
    });
    expect(res.status).toBe(201);
    const body = await res.json() as { session_id: string };
    expect(body.session_id).toBeDefined();
    expect(typeof body.session_id).toBe("string");
  });
});

describe("POST /sessions/:id/messages (SSE)", () => {
  it("流式返回 SSE Content-Type", async () => {
    const createRes = await fetch(`${serverUrl}/sessions`, {
      method: "POST",
      headers: authHeaders(OWNER_TOKEN),
    });
    const { session_id } = await createRes.json() as { session_id: string };

    const res = await fetch(`${serverUrl}/sessions/${session_id}/messages`, {
      method: "POST",
      headers: {
        ...authHeaders(OWNER_TOKEN),
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ content: "你好" }),
    });

    expect(res.status).toBe(200);
    expect(res.headers.get("Content-Type")).toContain("text/event-stream");
  });

  it("SSE 流中包含 token 数据", async () => {
    const createRes = await fetch(`${serverUrl}/sessions`, {
      method: "POST",
      headers: authHeaders(OWNER_TOKEN),
    });
    const { session_id } = await createRes.json() as { session_id: string };

    const res = await fetch(`${serverUrl}/sessions/${session_id}/messages`, {
      method: "POST",
      headers: {
        ...authHeaders(OWNER_TOKEN),
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ content: "hi" }),
    });

    const reader = res.body?.getReader();
    expect(reader).not.toBeNull();

    const decoder = new TextDecoder();
    let fullText = "";
    let done = false;

    while (!done) {
      const { value, done: streamDone } = await reader!.read();
      done = streamDone;
      if (value) fullText += decoder.decode(value, { stream: !streamDone });
    }

    expect(fullText).toContain("data:");

    const tokens: string[] = [];
    for (const line of fullText.split("\n")) {
      if (line.startsWith("data: ")) {
        const payload = line.slice("data: ".length);
        if (payload === "[DONE]") continue;
        try {
          const parsed = JSON.parse(payload) as { token: string };
          if (parsed.token) tokens.push(parsed.token);
        } catch { /* skip */ }
      }
    }
    const combined = tokens.join("");
    expect(combined).toContain("[回复]");
  });
});

describe("GET /sessions", () => {
  it("列出 API 会话", async () => {
    await fetch(`${serverUrl}/sessions`, {
      method: "POST",
      headers: authHeaders(OWNER_TOKEN),
    });

    const res = await fetch(`${serverUrl}/sessions`, {
      headers: authHeaders(OWNER_TOKEN),
    });
    expect(res.status).toBe(200);
    const list = await res.json() as unknown[];
    expect(Array.isArray(list)).toBe(true);
  });
});

describe("GET /sessions/:id", () => {
  it("存在的会话返回 200", async () => {
    const createRes = await fetch(`${serverUrl}/sessions`, {
      method: "POST",
      headers: authHeaders(OWNER_TOKEN),
    });
    const { session_id } = await createRes.json() as { session_id: string };

    const res = await fetch(`${serverUrl}/sessions/${session_id}`, {
      headers: authHeaders(OWNER_TOKEN),
    });
    expect(res.status).toBe(200);
    const body = await res.json() as Record<string, unknown>;
    expect(body.session_id).toBe(session_id);
  });

  it("不存在的会话返回 404", async () => {
    const res = await fetch(`${serverUrl}/sessions/nonexistent-id`, {
      headers: authHeaders(OWNER_TOKEN),
    });
    expect(res.status).toBe(404);
  });
});

describe("DELETE /sessions/:id", () => {
  it("删除会话返回 204", async () => {
    const createRes = await fetch(`${serverUrl}/sessions`, {
      method: "POST",
      headers: authHeaders(OWNER_TOKEN),
    });
    const { session_id } = await createRes.json() as { session_id: string };

    const deleteRes = await fetch(`${serverUrl}/sessions/${session_id}`, {
      method: "DELETE",
      headers: authHeaders(OWNER_TOKEN),
    });
    expect(deleteRes.status).toBe(204);

    const getRes = await fetch(`${serverUrl}/sessions/${session_id}`, {
      headers: authHeaders(OWNER_TOKEN),
    });
    expect(getRes.status).toBe(404);
  });
});

describe("GET /users/:id/memories", () => {
  it("admin 可查看任意用户记忆", async () => {
    const guestId = identityStore.resolve("api", GUEST_TOKEN)!.id;
    createTestMemory(guestId, "测试记忆内容");

    const res = await fetch(`${serverUrl}/users/${guestId}/memories`, {
      headers: authHeaders(OWNER_TOKEN),
    });
    expect(res.status).toBe(200);
  });

  it("guest 只能查看自己的记忆", async () => {
    const guestId = identityStore.resolve("api", GUEST_TOKEN)!.id;

    const res = await fetch(`${serverUrl}/users/${guestId}/memories`, {
      headers: authHeaders(GUEST_TOKEN),
    });
    expect(res.status).toBe(200);
  });

  it("guest 查看他人记忆被拒", async () => {
    const otherId = createTestUser("other", "other-token");

    const res = await fetch(`${serverUrl}/users/${otherId}/memories`, {
      headers: authHeaders(GUEST_TOKEN),
    });
    expect(res.status).toBe(403);
  });
});

describe("PUT /memories/:id", () => {
  it("更新记忆", async () => {
    const guestId = identityStore.resolve("api", GUEST_TOKEN)!.id;
    const memId = createTestMemory(guestId, "旧内容");

    const res = await fetch(`${serverUrl}/memories/${memId}`, {
      method: "PUT",
      headers: {
        ...authHeaders(GUEST_TOKEN),
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ content: "新内容" }),
    });
    expect(res.status).toBe(200);

    const mem = memoryStore.get(memId);
    expect(mem!.content).toBe("新内容");
  });

  it("更新他人记忆被拒", async () => {
    const otherId = createTestUser("other2", "other2-token");
    const memId = createTestMemory(otherId, "他人记忆");

    const res = await fetch(`${serverUrl}/memories/${memId}`, {
      method: "PUT",
      headers: {
        ...authHeaders(GUEST_TOKEN),
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ content: "hack" }),
    });
    expect(res.status).toBe(403);
  });
});

describe("DELETE /memories/:id", () => {
  it("删除记忆", async () => {
    const guestId = identityStore.resolve("api", GUEST_TOKEN)!.id;
    const memId = createTestMemory(guestId, "待删除内容");

    const res = await fetch(`${serverUrl}/memories/${memId}`, {
      method: "DELETE",
      headers: authHeaders(GUEST_TOKEN),
    });
    expect(res.status).toBe(204);
  });

  it("admin 可删除他人记忆", async () => {
    const guestId = identityStore.resolve("api", GUEST_TOKEN)!.id;
    const memId = createTestMemory(guestId, "admin 可删");

    const res = await fetch(`${serverUrl}/memories/${memId}`, {
      method: "DELETE",
      headers: authHeaders(OWNER_TOKEN),
    });
    expect(res.status).toBe(204);
  });
});

describe("GET /users", () => {
  it("返回用户列表", async () => {
    const res = await fetch(`${serverUrl}/users`, {
      headers: authHeaders(OWNER_TOKEN),
    });
    expect(res.status).toBe(200);
    const userList = await res.json() as unknown[];
    expect(Array.isArray(userList)).toBe(true);
    expect(userList.length).toBeGreaterThan(0);
  });
});

describe("POST /users/:id/merge", () => {
  it("合并用户（仅 admin）", async () => {
    const srcId = createTestUser("source", "merge-src-token");
    createTestMemory(srcId, "源用户记忆");
    const tgtId = createTestUser("target", "merge-tgt-token");

    // guest 尝试合并应被拒
    const deniedRes = await fetch(`${serverUrl}/users/${srcId}/merge`, {
      method: "POST",
      headers: {
        ...authHeaders(GUEST_TOKEN),
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ target_user_id: tgtId }),
    });
    expect(deniedRes.status).toBe(403);

    // admin 执行合并
    const res = await fetch(`${serverUrl}/users/${srcId}/merge`, {
      method: "POST",
      headers: {
        ...authHeaders(OWNER_TOKEN),
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ target_user_id: tgtId }),
    });
    expect(res.status).toBe(200);

    // 验证源用户已删除
    expect(users.get(srcId)).toBeNull();

    // 验证记忆已转移
    const tgtMemories = memoryStore.search(tgtId, "源用户");
    expect(tgtMemories.length).toBe(1);
  });
});

describe("OpenAPI 文档", () => {
  it("GET /openapi/json 返回 JSON", async () => {
    const res = await fetch(`${serverUrl}/openapi/json`);
    expect(res.status).toBe(200);
    const body = await res.json() as Record<string, unknown>;
    expect(body.openapi).toBeDefined();
  });
});
