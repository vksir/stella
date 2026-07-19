import { describe, expect, it, beforeAll, afterAll } from "bun:test";
import { resolve } from "node:path";
import { existsSync, mkdirSync, rmSync } from "node:fs";
import { Database } from "bun:sqlite";
import { runMigrations } from "../src/stores/schema";
import { createSessionStore, type SessionStore } from "../src/stores/session";
import { SessionRegistry, type ISessionFactory, type SessionCreateResult } from "../src/sessions-registry";
import type { AgentSession } from "@earendil-works/pi-coding-agent";

const tmpDir = resolve(import.meta.dir, "..", ".test", "sessions");
const dbPath = resolve(tmpDir, "test-sessions.db");

let db_sql: Database;
let sessionStore: SessionStore;
let registry: SessionRegistry;
let sessionCounter = 0;

function makeMockSession(chatKey?: string): SessionCreateResult {
  const id = `mock-session-${++sessionCounter}`;
  const path = chatKey ? `/fake/sessions/${chatKey}.jsonl` : `/fake/sessions/${id}.jsonl`;
  return {
    session: {
      sessionId: id,
      sessionFile: path,
      isStreaming: false,
      dispose: () => {},
      prompt: async () => {},
      subscribe: () => () => {},
    } as unknown as AgentSession,
    dispose: () => {},
    sessionPath: path,
  };
}

const mockFactory: ISessionFactory = {
  async create(_platform: string, chatKey: string): Promise<SessionCreateResult> {
    return makeMockSession(chatKey);
  },
};

beforeAll(async () => {
  if (!existsSync(tmpDir)) mkdirSync(tmpDir, { recursive: true });
  db_sql = new Database(dbPath);
  runMigrations(db_sql);
  sessionStore = createSessionStore(db_sql);
  registry = new SessionRegistry(sessionStore, mockFactory);
});

afterAll(() => {
  registry.disposeAll();
  db_sql.close();
  try { rmSync(tmpDir, { recursive: true }); } catch { /* ok */ }
});

describe("SessionRegistry", () => {
  it("getOrCreate 首次调用创建新会话", async () => {
    const session = await registry.getOrCreate("api", "api:test-1");
    expect(session).not.toBeNull();
    expect(session.sessionId).toBeTruthy();
  });

  it("再次调用同一 chatKey 返回相同会话", async () => {
    const s1 = await registry.getOrCreate("api", "api:test-2");
    const s2 = await registry.getOrCreate("api", "api:test-2");
    expect(s2.sessionId).toBe(s1.sessionId);
  });

  it("不同 chatKey 返回不同会话", async () => {
    const s1 = await registry.getOrCreate("api", "api:test-3a");
    const s2 = await registry.getOrCreate("api", "api:test-3b");
    expect(s2.sessionId).not.toBe(s1.sessionId);
  });

  it("会话创建后写入 sqlite 注册表", () => {
    const row = sessionStore.get("api", "api:test-1");
    expect(row).not.toBeNull();
    expect(row!.session_path).toContain("test-1");
  });

  it("串行化派发：同一 chatKey 的任务序列化执行", async () => {
    const chatKey = "api:serial-test";
    const order: number[] = [];

    const p1 = registry.dispatch(chatKey, async () => {
      order.push(1);
      await new Promise((r) => setTimeout(r, 10));
      order.push(2);
    });
    const p2 = registry.dispatch(chatKey, async () => {
      order.push(3);
    });

    await Promise.all([p1, p2]);
    expect(order).toEqual([1, 2, 3]);
  });

  it("不同 chatKey 的 dispatch 可并行", async () => {
    const order: number[] = [];

    const p1 = registry.dispatch("api:par-a", async () => {
      order.push(1);
      await new Promise((r) => setTimeout(r, 20));
      order.push(3);
    });
    await new Promise((r) => setTimeout(r, 5));
    const p2 = registry.dispatch("api:par-b", async () => {
      order.push(2);
    });

    await Promise.all([p1, p2]);
    expect(order).toEqual([1, 2, 3]);
  });

  it("has 正确反映会话存在性", async () => {
    expect(registry.has("api", "api:nonexist")).toBe(false);
    await registry.getOrCreate("api", "api:has-test");
    expect(registry.has("api", "api:has-test")).toBe(true);
  });
});
