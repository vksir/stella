import { describe, expect, it, beforeAll, afterAll } from "bun:test";
import { resolve } from "node:path";
import { unlinkSync, existsSync, mkdirSync } from "node:fs";
import { Database } from "bun:sqlite";
import { runMigrations } from "../src/stores/schema";
import { createUserStore, type UserStore } from "../src/stores/user";
import { createIdentityStore, type IdentityStore } from "../src/stores/identity";
import { createIdentityResolver, type IdentityResolver } from "../src/identity";
import type { StellaConfig } from "../src/config";

const tmpDir = resolve(import.meta.dir, "..", ".test", "identity");
const dbPath = resolve(tmpDir, "test-identity.db");

let db_sql: Database;
let users: UserStore;
let identities: IdentityStore;
let resolver: IdentityResolver;

const testConfig: StellaConfig = {
  owner: { qq: "99999", api_token: "master-token" },
  napcat: { listen: "0.0.0.0:8082", token: "x" },
  api: { listen: "0.0.0.0:3000" },
  model: { provider: "x", name: "y" },
  paths: { data_dir: tmpDir },
  memory: { max_entries_per_user: 200, max_content_chars: 2000 },
  short_term_memory: { max_tokens: 10000, max_age_days: 3 },
  tools: { whitelist: [] },
};

beforeAll(() => {
  if (!existsSync(tmpDir)) mkdirSync(tmpDir, { recursive: true });
  if (existsSync(dbPath)) unlinkSync(dbPath);
  db_sql = new Database(dbPath);
  runMigrations(db_sql);
  users = createUserStore(db_sql);
  identities = createIdentityStore(db_sql);
  resolver = createIdentityResolver(users, identities, testConfig);
});

afterAll(() => {
  db_sql.close();
  try { unlinkSync(dbPath); } catch { /* ok */ }
});

describe("createIdentityResolver", () => {
  it("首次解析管理员 QQ 身份 → 返回 admin 用户", () => {
    const user = resolver.resolve("qq", "99999");
    expect(user).not.toBeNull();
    expect(user!.role).toBe("admin");
    expect(user!.display_name).toBe("管理员");
  });

  it("首次解析管理员 API token → 返回同一 admin 用户", () => {
    const user = resolver.resolve("api", "master-token");
    expect(user).not.toBeNull();
    expect(user!.role).toBe("admin");
    const qqUser = resolver.resolve("qq", "99999");
    expect(user!.id).toBe(qqUser!.id);
  });

  it("首次解析陌生人 → 自动建档 guest", () => {
    const user = resolver.resolve("qq", "11111");
    expect(user).not.toBeNull();
    expect(user!.role).toBe("guest");
    expect(user!.display_name).toBe("qq:11111");
  });

  it("再次解析同一陌生人 → 返回已有 guest", () => {
    const first = resolver.resolve("qq", "22222");
    const second = resolver.resolve("qq", "22222");
    expect(second!.id).toBe(first!.id);
    expect(second!.role).toBe("guest");
  });

  it("不同平台的陌生人各自独立建档", () => {
    const qqUser = resolver.resolve("qq", "33333");
    const apiUser = resolver.resolve("api", "token-new");
    expect(qqUser!.id).not.toBe(apiUser!.id);
  });
});
