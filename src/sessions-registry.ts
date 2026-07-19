import type { AgentSession } from "@earendil-works/pi-coding-agent";
import type { SessionStore } from "./stores/session";

// ---- 接口定义 ----

export interface SessionCreateResult {
  session: AgentSession;
  dispose: () => void;
  sessionPath: string;
}

/**
 * 会话工厂接口：给定平台与 chatKey，创建 AgentSession。
 * 具体实现（SessionFactory 类）封装 SDK 依赖。
 */
export interface ISessionFactory {
  create(platform: string, chatKey: string): Promise<SessionCreateResult>;
}

// ---- 会话注册表 ----

interface SessionEntry {
  session: AgentSession;
  dispose: () => void;
}

/**
 * 会话注册表：管理 AgentSession 的创建、缓存和串行化访问。
 *
 * - 每个 (platform, chatKey) 对应一个会话
 * - 同一 chatKey 的操作串行化（通过 enqueue 实现）
 * - 会话惰性创建，首次访问时通过 ISessionFactory 创建
 */
export class SessionRegistry {
  private sessions = new Map<string, SessionEntry>();
  private locks = new Map<string, Promise<void>>();
  private store: SessionStore;
  private factory: ISessionFactory;

  constructor(store: SessionStore, factory: ISessionFactory) {
    this.store = store;
    this.factory = factory;
  }

  /** 获取或惰性创建会话（同一 chatKey 串行化）。 */
  async getOrCreate(platform: string, chatKey: string): Promise<AgentSession> {
    const k = this.fullKey(platform, chatKey);
    return this.enqueue(k, () => this.createOne(platform, chatKey));
  }

  /** 检查是否已有活跃会话。 */
  has(platform: string, chatKey: string): boolean {
    return this.sessions.has(this.fullKey(platform, chatKey));
  }

  /**
   * 串行化派发：同一 chatKey 的任务顺序执行，不同 chatKey 可并行。
   */
  async dispatch<T>(chatKey: string, fn: () => Promise<T>): Promise<T> {
    return this.enqueue(chatKey, fn);
  }

  /** 销毁所有会话。 */
  disposeAll(): void {
    for (const [, entry] of this.sessions) {
      try { entry.dispose(); } catch { /* ok */ }
    }
    this.sessions.clear();
  }

  // ---- 私有方法 ----

  private fullKey(platform: string, chatKey: string): string {
    return `${platform}:${chatKey}`;
  }

  private enqueue<T>(k: string, fn: () => Promise<T>): Promise<T> {
    const prev = this.locks.get(k) ?? Promise.resolve();
    let resolve: () => void;
    const next = new Promise<void>((r) => { resolve = r; });
    this.locks.set(k, prev.then(() => next));

    return prev.then(async () => {
      try {
        return await fn();
      } finally {
        resolve!();
      }
    });
  }

  private async createOne(platform: string, chatKey: string): Promise<AgentSession> {
    const k = this.fullKey(platform, chatKey);
    const existing = this.sessions.get(k);
    if (existing) return existing.session;

    const { session, dispose, sessionPath } = await this.factory.create(platform, chatKey);
    this.sessions.set(k, { session, dispose });
    this.store.upsert(platform, chatKey, sessionPath);
    return session;
  }
}
