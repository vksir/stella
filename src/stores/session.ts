import type { Database } from "bun:sqlite";

export interface ChatSessionRow {
  platform: string;
  chat_key: string;
  session_path: string;
  created_at: number;
  last_active: number;
}

export interface SessionStore {
  /** 创建或更新会话记录。 */
  upsert(platform: string, chatKey: string, sessionPath: string): void;
  /** 按 (platform, chat_key) 查找会话。 */
  get(platform: string, chatKey: string): ChatSessionRow | null;
  /** 列出指定平台的所有会话记录。 */
  listByPlatform(platform: string): ChatSessionRow[];
  /** 删除指定会话记录。 */
  delete_(platform: string, chatKey: string): void;
}

export function createSessionStore(db: Database): SessionStore {
  const now = () => Math.floor(Date.now() / 1000);

  return {
    upsert(platform: string, chatKey: string, sessionPath: string): void {
      const ts = now();
      db.prepare(
        `INSERT INTO chat_sessions (platform, chat_key, session_path, created_at, last_active)
         VALUES (?, ?, ?, ?, ?)
         ON CONFLICT(platform, chat_key) DO UPDATE SET
           session_path = excluded.session_path,
           last_active = excluded.last_active`,
      ).run(platform, chatKey, sessionPath, ts, ts);
    },

    get(platform: string, chatKey: string): ChatSessionRow | null {
      return db
        .query("SELECT * FROM chat_sessions WHERE platform = ? AND chat_key = ?")
        .get(platform, chatKey) as ChatSessionRow | null;
    },

    listByPlatform(platform: string): ChatSessionRow[] {
      return db
        .query(
          "SELECT chat_key, created_at, last_active FROM chat_sessions WHERE platform = ? ORDER BY last_active DESC",
        )
        .all(platform) as ChatSessionRow[];
    },

    delete_(platform: string, chatKey: string): void {
      db.prepare(
        "DELETE FROM chat_sessions WHERE platform = ? AND chat_key = ?",
      ).run(platform, chatKey);
    },
  };
}
