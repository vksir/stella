import { Database } from "bun:sqlite";
import { resolve } from "node:path";
import { loadConfig, type StellaConfig } from "./config";
import { initDataDir } from "./bootstrap";
import { runMigrations } from "./stores/schema";
import { transaction } from "./stores/transaction";
import { createUserStore, type UserStore, type UserRow } from "./stores/user";
import { createIdentityStore, type IdentityStore } from "./stores/identity";
import { createSessionStore, type SessionStore } from "./stores/session";
import { createMemoryStore, type MemoryStore } from "./stores/memory";
import { createIdentityResolver, type IdentityResolver } from "./identity";
import { SessionRegistry } from "./sessions-registry";
import { SessionFactory } from "./session-factory";
import { createMemoryTools } from "./tools/memory";
import { startApiServer, type ApiServerHandle } from "./api/api";
import { startQQAdapter, createTriggerNoteExtension, type QQAdapterHandle } from "./adapters/qq/qq";
import type { ToolDefinition } from "@earendil-works/pi-coding-agent";

/**
 * 应用级共享上下文：所有模块通过此获取依赖。
 */
export interface AppContext {
  config: StellaConfig;
  dataDir: string;

  users: UserStore;
  identityStore: IdentityStore;
  sessionStore: SessionStore;
  memoryStore: MemoryStore;

  identity: IdentityResolver;
  sessions: SessionRegistry;
  memoryTools: ToolDefinition[];

  transaction: <T>(fn: () => T) => T;
  setSessionUser: (sessionId: string, user: UserRow) => void;

  /** API 服务器句柄（用于关闭） */
  apiServer: ApiServerHandle;
  /** QQ 适配器句柄（用于关闭） */
  qqAdapter: QQAdapterHandle | null;
}

let _ctx: AppContext | null = null;

export function getAppContext(): AppContext {
  if (!_ctx) throw new Error("应用尚未启动，请先调用 bootstrap()");
  return _ctx;
}

function parseArgs(argv: string[]): { configPath: string } {
  let configPath = "config.toml";
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--config" && i + 1 < argv.length) {
      configPath = argv[i + 1]!;
      i++;
    } else if (argv[i]!.startsWith("--config=")) {
      configPath = argv[i]!.slice("--config=".length);
    }
  }
  if (process.env.STELLA_CONFIG) {
    configPath = process.env.STELLA_CONFIG;
  }
  return { configPath };
}

export async function bootstrap(argv: string[] = process.argv): Promise<AppContext> {
  const { configPath } = parseArgs(argv);

  // 1. 加载配置
  const config = loadConfig(configPath);

  // 2. 初始化目录结构
  const cwd = process.cwd();
  const dataDir = resolve(config.paths.data_dir);
  initDataDir(dataDir);
  const agentDir = resolve(dataDir, "agent");
  const sessionsDir = resolve(dataDir, "sessions");

  // 3. 初始化数据库 + schema
  const dbPath = resolve(dataDir, "memory.db");
  const db_sql = new Database(dbPath);
  runMigrations(db_sql);

  const tx = <T>(fn: () => T): T => transaction(db_sql, fn);

  // 4. 创建存储模块
  const users = createUserStore(db_sql);
  const identityStore = createIdentityStore(db_sql);
  const sessionStore = createSessionStore(db_sql);
  const memoryStore = createMemoryStore(db_sql, config.memory.max_entries_per_user);

  // 5. 身份解析器
  const identity = createIdentityResolver(users, identityStore, config);

  // 6. 会话 → 当前说话人映射
  const sessionUserMap = new Map<string, UserRow>();
  function getUserForSession(sessionId: string): UserRow | null {
    return sessionUserMap.get(sessionId) ?? null;
  }
  function setSessionUser(sessionId: string, user: UserRow): void {
    sessionUserMap.set(sessionId, user);
  }

  // 7. 记忆工具
  const memoryTools = createMemoryTools(memoryStore, config.memory.max_content_chars, getUserForSession);

  // 8. 会话工厂 + 注册表
  const triggerNoteExt = createTriggerNoteExtension();
  const sessionFactory = new SessionFactory({
    cwd,
    agentDir,
    dataDir,
    sessionsDir,
    config,
    memoryTools,
    extraExtensions: [triggerNoteExt],
  });
  const sessions = new SessionRegistry(sessionStore, sessionFactory);

  const ctxWithoutServer = {
    config, dataDir,
    users, identityStore, sessionStore, memoryStore,
    identity, sessions, memoryTools,
    transaction: tx,
    setSessionUser,
    apiServer: null!,
    qqAdapter: null,
  } as AppContext;

  // 9. 启动 API 服务器
  const apiServer = await startApiServer(ctxWithoutServer);
  ctxWithoutServer.apiServer = apiServer;
  _ctx = ctxWithoutServer;

  console.log(`[Stella] 配置加载完成: ${configPath}`);
  console.log(`[Stella] 数据目录: ${dataDir}`);
  console.log(`[Stella] SDK agentDir: ${agentDir}`);
  console.log(`[Stella] 模型: ${config.model.provider}/${config.model.name} (thinking: ${config.model.thinking_level ?? "high"})`);

  // 10. 启动 QQ 适配器
  let qqAdapter: QQAdapterHandle | null = null;
  try {
    qqAdapter = await startQQAdapter(_ctx);
  } catch (err) {
    console.error("[Stella] QQ 适配器启动失败:", err);
  }
  ctxWithoutServer.qqAdapter = qqAdapter;

  return _ctx;
}

if (import.meta.main) {
  await bootstrap();
}
