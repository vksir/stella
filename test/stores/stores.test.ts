import { describe, expect, it, beforeAll, afterAll } from "bun:test";
import { resolve } from "node:path";
import { unlinkSync, existsSync, mkdirSync } from "node:fs";
import { Database } from "bun:sqlite";
import { runMigrations } from "../../src/stores/schema";
import { createUserStore, type UserStore } from "../../src/stores/user";
import { createIdentityStore, type IdentityStore } from "../../src/stores/identity";
import { createSessionStore, type SessionStore } from "../../src/stores/session";
import { createMemoryStore, type MemoryStore, MemoryLimitError } from "../../src/stores/memory";

const tmpDir = resolve(import.meta.dir, "..", "..", ".test", "stores");
const dbPath = resolve(tmpDir, "test-stores.db");

let db_sql: Database;
let users: UserStore;
let identities: IdentityStore;
let sessions: SessionStore;
let memories: MemoryStore;

beforeAll(() => {
  if (!existsSync(tmpDir)) mkdirSync(tmpDir, { recursive: true });
  if (existsSync(dbPath)) unlinkSync(dbPath);
  db_sql = new Database(dbPath);
  runMigrations(db_sql);
  users = createUserStore(db_sql);
  identities = createIdentityStore(db_sql);
  sessions = createSessionStore(db_sql);
  memories = createMemoryStore(db_sql, 10);
});

afterAll(() => {
  db_sql.close();
  try { unlinkSync(dbPath); } catch { /* ok */ }
});

describe("schema", () => {
  it("创建 users 表", () => {
    const row = db_sql.query("SELECT name FROM sqlite_master WHERE type='table' AND name='users'").get() as { name: string } | undefined;
    expect(row).not.toBeUndefined();
  });

  it("创建 user_identities 表", () => {
    const row = db_sql.query("SELECT name FROM sqlite_master WHERE type='table' AND name='user_identities'").get() as { name: string } | undefined;
    expect(row).not.toBeUndefined();
  });

  it("创建 chat_sessions 表", () => {
    const row = db_sql.query("SELECT name FROM sqlite_master WHERE type='table' AND name='chat_sessions'").get() as { name: string } | undefined;
    expect(row).not.toBeUndefined();
  });

  it("重复 runMigrations 不报错（幂等）", () => {
    expect(() => runMigrations(db_sql)).not.toThrow();
  });
});

describe("UserStore", () => {
  it("创建用户并返回 id", () => {
    const id = users.create("Alice", "guest");
    expect(typeof id).toBe("number");
    expect(id).toBeGreaterThan(0);
  });

  it("按 id 获取用户", () => {
    const id = users.create("Bob", "guest");
    const user = users.get(id);
    expect(user).not.toBeNull();
    expect(user!.display_name).toBe("Bob");
    expect(user!.role).toBe("guest");
  });

  it("获取不存在的用户返回 null", () => {
    expect(users.get(99999)).toBeNull();
  });

  it("listAll 返回所有用户", () => {
    users.create("Eve", "admin");
    const all = users.listAll();
    expect(all.length).toBeGreaterThanOrEqual(1);
  });

  it("updateRole 修改用户角色", () => {
    const id = users.create("Frank", "guest");
    users.updateRole(id, "admin");
    expect(users.get(id)!.role).toBe("admin");
  });

  it("delete_ 删除用户", () => {
    const id = users.create("Grace", "guest");
    users.delete_(id);
    expect(users.get(id)).toBeNull();
  });
});

describe("IdentityStore", () => {
  it("link 创建平台身份关联", () => {
    const userId = users.create("Charlie", "guest");
    identities.link(userId, "qq", "12345");

    const user = identities.resolve("qq", "12345");
    expect(user).not.toBeNull();
    expect(user!.display_name).toBe("Charlie");
  });

  it("重复 link 不报错（INSERT OR IGNORE）", () => {
    const userId = users.create("Dave", "guest");
    identities.link(userId, "qq", "99999");
    expect(() => identities.link(userId, "qq", "99999")).not.toThrow();
  });

  it("resolve 按 (platform, platform_user_id) 返回 user", () => {
    const id = users.create("Eve", "guest");
    identities.link(id, "api", "token-xyz");
    const user = identities.resolve("api", "token-xyz");
    expect(user).not.toBeNull();
    expect(user!.display_name).toBe("Eve");
  });

  it("未匹配身份返回 null", () => {
    expect(identities.resolve("qq", "nonexistent")).toBeNull();
  });

  it("reassignUser 转移身份", () => {
    const srcId = users.create("Source", "guest");
    const tgtId = users.create("Target", "guest");
    identities.link(srcId, "qq", "move-me");

    identities.reassignUser(srcId, tgtId);

    // 源用户身份不再可解析
    expect(identities.resolve("qq", "move-me")!.id).toBe(tgtId);
  });
});

describe("SessionStore", () => {
  it("upsert 写入会话记录", () => {
    sessions.upsert("qq", "private:111", "/data/sessions/1.jsonl");
    const row = sessions.get("qq", "private:111");
    expect(row).not.toBeNull();
    expect(row!.session_path).toBe("/data/sessions/1.jsonl");
  });

  it("upsert 更新已有记录", () => {
    sessions.upsert("qq", "private:111", "/data/sessions/1-updated.jsonl");
    const row = sessions.get("qq", "private:111");
    expect(row!.session_path).toBe("/data/sessions/1-updated.jsonl");
  });

  it("获取不存在的会话返回 null", () => {
    expect(sessions.get("qq", "group:999")).toBeNull();
  });

  it("listByPlatform 列出平台会话", () => {
    sessions.upsert("api", "api:test1", "/fake/1.jsonl");
    const list = sessions.listByPlatform("api");
    expect(list.length).toBeGreaterThanOrEqual(1);
  });

  it("delete_ 删除会话", () => {
    sessions.upsert("api", "api:del-test", "/fake/del.jsonl");
    sessions.delete_("api", "api:del-test");
    expect(sessions.get("api", "api:del-test")).toBeNull();
  });
});

describe("MemoryStore", () => {
  it("create 创建记忆", () => {
    const userId = users.create("MemUser", "guest");
    const id = memories.create(userId, "喜欢咖啡");
    expect(id).toBeGreaterThan(0);
  });

  it("get 获取记忆", () => {
    const userId = users.create("MemUser2", "guest");
    const id = memories.create(userId, "住在上海");
    const mem = memories.get(id);
    expect(mem).not.toBeNull();
    expect(mem!.content).toBe("住在上海");
  });

  it("search 关键词匹配", () => {
    const userId = users.create("MemUser3", "guest");
    memories.create(userId, "喜欢游泳");
    memories.create(userId, "喜欢跑步");

    const results = memories.search(userId, "游泳");
    expect(results.length).toBe(1);
    expect(results[0]!.content).toBe("喜欢游泳");
  });

  it("list 列出记忆", () => {
    const userId = users.create("MemUser4", "guest");
    memories.create(userId, "A");
    memories.create(userId, "B");

    const results = memories.list(userId);
    expect(results.length).toBeGreaterThanOrEqual(2);
  });

  it("update 更新记忆", () => {
    const userId = users.create("MemUser5", "guest");
    const id = memories.create(userId, "旧");
    memories.update(id, "新");
    expect(memories.get(id)!.content).toBe("新");
  });

  it("delete_ 删除记忆", () => {
    const userId = users.create("MemUser6", "guest");
    const id = memories.create(userId, "待删除");
    memories.delete_(id);
    expect(memories.get(id)).toBeNull();
  });

  it("count 返回记忆条数", () => {
    const userId = users.create("MemUser7", "guest");
    expect(memories.count(userId)).toBe(0);
    memories.create(userId, "X");
    expect(memories.count(userId)).toBe(1);
  });

  it("reassignUser 转移记忆", () => {
    const srcId = users.create("MemSrc", "guest");
    const tgtId = users.create("MemTgt", "guest");
    memories.create(srcId, "源用户的记忆");

    memories.reassignUser(srcId, tgtId);

    const tgtMems = memories.search(tgtId, "源用户");
    expect(tgtMems.length).toBe(1);
    const srcMems = memories.search(srcId, "源用户");
    expect(srcMems.length).toBe(0);
  });

  it("create 超限抛出 MemoryLimitError", () => {
    const limited = createMemoryStore(db_sql, 2);
    const userId = users.create("FullUser", "guest");

    limited.create(userId, "1");
    limited.create(userId, "2");

    expect(() => limited.create(userId, "3")).toThrow(MemoryLimitError);
  });
});
