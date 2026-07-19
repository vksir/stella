import { describe, test, expect, beforeEach } from "bun:test";
import { Database } from "bun:sqlite";
import { runMigrations } from "../../src/stores/schema";
import { createUserStore } from "../../src/stores/user";
import { createMemoryStore, type MemoryStore, MemoryLimitError } from "../../src/stores/memory";
import { createMemoryTools } from "../../src/tools/memory";
import type { UserRow } from "../../src/stores/user";
import { mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";

function setup(overrides?: { maxEntriesPerUser?: number; maxContentChars?: number }) {
  const tmpDir = mkdtempSync(join(tmpdir(), "stella-memory-test-"));
  const dbPath = join(tmpDir, "test.db");

  const db_sql = new Database(dbPath);
  runMigrations(db_sql);

  const users = createUserStore(db_sql);
  const maxEntries = overrides?.maxEntriesPerUser ?? 4;
  const memories = createMemoryStore(db_sql, maxEntries);
  const maxContentChars = overrides?.maxContentChars ?? 2000;

  // 创建两个测试用户
  const aliceId = users.create("Alice", "guest");
  const bobId = users.create("Bob", "guest");

  let currentUser: UserRow | null = null;
  const getUserForSession = (_sessionId: string): UserRow | null => currentUser;

  const tools = createMemoryTools(memories, maxContentChars, getUserForSession);
  const toolMap = new Map(tools.map((t) => [t.name, t]));

  return {
    db_sql,
    users,
    memories,
    aliceId,
    bobId,
    tmpDir,
    currentUser: {
      set: (u: UserRow | null) => { currentUser = u; },
    },
    tools,
    toolMap,
    execute: async (name: string, params: Record<string, unknown>) => {
      const tool = toolMap.get(name);
      if (!tool) throw new Error(`工具 ${name} 未找到`);
      return tool.execute("test-call-id", params as any, undefined, undefined, {
        sessionManager: { getSessionId: () => "test-session" },
      } as any);
    },
    cleanup: () => {
      db_sql.close();
      try { rmSync(tmpDir, { recursive: true, force: true }); } catch {}
    },
  };
}

function alice(): UserRow {
  return { id: 1, display_name: "Alice", role: "guest", created_at: 1 };
}

function bob(): UserRow {
  return { id: 2, display_name: "Bob", role: "guest", created_at: 1 };
}

describe("memory_tools", () => {
  describe("memory_save", () => {
    test("新建记忆", async () => {
      const { execute, currentUser, cleanup } = setup();
      currentUser.set(alice());

      const result = await execute("memory_save", { content: "Alice 喜欢猫" });

      expect(result.content).toEqual([{ type: "text", text: expect.stringContaining("已保存") }]);
      expect(result.details).toHaveProperty("id");
      expect(result.details).toHaveProperty("content", "Alice 喜欢猫");
      cleanup();
    });

    test("带 id 覆盖更新", async () => {
      const { execute, currentUser, cleanup } = setup();
      currentUser.set(alice());

      const r1 = await execute("memory_save", { content: "Alice 喜欢狗" });
      const id = (r1.details as any).id as number;

      const r2 = await execute("memory_save", { content: "Alice 喜欢猫", id });

      expect(r2.details).toHaveProperty("id", id);
      expect(r2.details).toHaveProperty("content", "Alice 喜欢猫");
      expect((r2 as any).content[0].text).toContain("已更新");
      cleanup();
    });

    test("更新不存在的 id 报错", async () => {
      const { execute, currentUser, cleanup } = setup();
      currentUser.set(alice());

      const result = await execute("memory_save", { content: "test", id: 999 });

      expect(result.content[0]).toHaveProperty("type", "text");
      expect(String((result as any).content[0].text)).toMatch(/找不到|不存在/);
      expect(result.details).toHaveProperty("error");
      cleanup();
    });

    test("超限拒存", async () => {
      const { execute, currentUser, cleanup } = setup({ maxEntriesPerUser: 2, maxContentChars: 2000 });
      currentUser.set(alice());

      await execute("memory_save", { content: "记忆 1" });
      await execute("memory_save", { content: "记忆 2" });

      // 第 3 条超限 → MemoryLimitError 被捕获转为友好文本
      const result = await execute("memory_save", { content: "记忆 3" });

      expect(String((result as any).content[0].text)).toMatch(/已满|上限|先删/);
      expect(result.details).toHaveProperty("error");
      cleanup();
    });

    test("超长内容拒存", async () => {
      const { execute, currentUser, cleanup } = setup({ maxContentChars: 5 });
      currentUser.set(alice());

      const result = await execute("memory_save", { content: "123456" });

      expect(String((result as any).content[0].text)).toMatch(/超长|上限|字符/);
      expect(result.details).toHaveProperty("error");
      cleanup();
    });

    test("带 id 覆盖时不检查条目上限", async () => {
      const { execute, currentUser, cleanup } = setup({ maxEntriesPerUser: 2, maxContentChars: 2000 });
      currentUser.set(alice());

      await execute("memory_save", { content: "记忆 1" });
      await execute("memory_save", { content: "记忆 2" });

      const result = await execute("memory_save", { content: "更新记忆 1", id: 1 });

      expect((result as any).content[0].text).toContain("已更新");
      expect(result.details).toHaveProperty("id", 1);
      cleanup();
    });
  });

  describe("memory_search", () => {
    test("关键词匹配", async () => {
      const { execute, currentUser, cleanup } = setup();
      currentUser.set(alice());

      await execute("memory_save", { content: "Alice 喜欢猫" });
      await execute("memory_save", { content: "Alice 的生日是 3 月 15 日" });
      await execute("memory_save", { content: "Alice 住在北京" });

      const result = await execute("memory_search", { keyword: "猫" });
      const results = result.details as any;

      expect(results).toHaveProperty("results");
      expect(results.results).toBeArray();
      expect(results.results.length).toBe(1);
      expect(results.results[0]).toHaveProperty("content", "Alice 喜欢猫");
      cleanup();
    });

    test("无匹配返回空列表", async () => {
      const { execute, currentUser, cleanup } = setup();
      currentUser.set(alice());

      await execute("memory_save", { content: "Alice 喜欢猫" });

      const result = await execute("memory_search", { keyword: "狗" });
      const results = result.details as any;

      expect(results.results).toBeArray();
      expect(results.results.length).toBe(0);
      cleanup();
    });
  });

  describe("memory_list", () => {
    test("列出全部记忆", async () => {
      const { execute, currentUser, cleanup } = setup();
      currentUser.set(alice());

      await execute("memory_save", { content: "记忆 A" });
      await execute("memory_save", { content: "记忆 B" });
      await execute("memory_save", { content: "记忆 C" });

      const result = await execute("memory_list", {});
      const details = result.details as any;

      expect(details.results).toBeArray();
      expect(details.results.length).toBe(3);
      cleanup();
    });

    test("分页 offset/limit", async () => {
      const { execute, currentUser, cleanup } = setup();
      currentUser.set(alice());

      for (let i = 0; i < 5; i++) {
        await execute("memory_save", { content: `记忆 ${i}` });
      }

      const result = await execute("memory_list", { offset: 1, limit: 2 });
      const details = result.details as any;

      expect(details.results).toBeArray();
      expect(details.results.length).toBe(2);
      cleanup();
    });
  });

  describe("memory_delete", () => {
    test("删除后 search 不可见", async () => {
      const { execute, currentUser, cleanup } = setup();
      currentUser.set(alice());

      const saved = await execute("memory_save", { content: "待删除的记忆" });
      const id = (saved.details as any).id as number;

      await execute("memory_delete", { id });

      const searchResult = await execute("memory_search", { keyword: "待删除" });
      const results = (searchResult.details as any).results;
      expect(results.length).toBe(0);
      cleanup();
    });

    test("删除不存在的 id 报错", async () => {
      const { execute, currentUser, cleanup } = setup();
      currentUser.set(alice());

      const result = await execute("memory_delete", { id: 999 });

      expect(String((result as any).content[0].text)).toMatch(/找不到|不存在/);
      expect(result.details).toHaveProperty("error");
      cleanup();
    });
  });

  describe("作用域隔离", () => {
    test("两个不同用户的记忆互不可见", async () => {
      const { execute, currentUser, cleanup } = setup();
      currentUser.set(alice());
      await execute("memory_save", { content: "Alice 的秘密" });

      currentUser.set(bob());
      await execute("memory_save", { content: "Bob 的秘密" });

      currentUser.set(alice());
      const aliceResult = await execute("memory_search", { keyword: "秘密" });
      const aliceResults = (aliceResult.details as any).results;
      expect(aliceResults.length).toBe(1);
      expect(aliceResults[0].content).toBe("Alice 的秘密");

      currentUser.set(bob());
      const bobResult = await execute("memory_search", { keyword: "秘密" });
      const bobResults = (bobResult.details as any).results;
      expect(bobResults.length).toBe(1);
      expect(bobResults[0].content).toBe("Bob 的秘密");
      cleanup();
    });

    test("Alice 不能删除 Bob 的记忆", async () => {
      const { execute, currentUser, cleanup } = setup();
      currentUser.set(bob());
      const saved = await execute("memory_save", { content: "Bob 的记忆" });
      const bobMemoryId = (saved.details as any).id as number;

      currentUser.set(alice());
      const result = await execute("memory_delete", { id: bobMemoryId });

      expect(String((result as any).content[0].text)).toMatch(/找不到|不存在|无权/);
      expect(result.details).toHaveProperty("error");

      currentUser.set(bob());
      const searchResult = await execute("memory_search", { keyword: "Bob 的记忆" });
      const results = (searchResult.details as any).results;
      expect(results.length).toBe(1);
      cleanup();
    });
  });
});
