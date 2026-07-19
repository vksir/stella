import type { Database } from "bun:sqlite";

export interface MemoryRow {
  id: number;
  user_id: number;
  content: string;
  created_at: number;
  updated_at: number;
  source_session: string | null;
}

export interface MemoryStore {
  /**
   * 创建一条长期记忆。
   * @throws MemoryLimitError 当用户记忆条数已达上限。
   */
  create(userId: number, content: string, sourceSession?: string): number;
  get(id: number): MemoryRow | null;
  search(userId: number, keyword: string): MemoryRow[];
  list(userId: number, offset?: number, limit?: number): MemoryRow[];
  update(id: number, content: string): void;
  delete_(id: number): void;
  count(userId: number): number;
  /** 将 source 用户的所有记忆转移给 target 用户（用于用户合并）。 */
  reassignUser(sourceUserId: number, targetUserId: number): void;
}

export class MemoryLimitError extends Error {
  constructor(
    public readonly current: number,
    public readonly max: number,
  ) {
    super(`记忆已满（${current}/${max} 条），请先删除旧记忆后再保存新的。`);
    this.name = "MemoryLimitError";
  }
}

export function createMemoryStore(db: Database, maxEntriesPerUser: number = 200): MemoryStore {
  const now = () => Math.floor(Date.now() / 1000);

  return {
    create(userId: number, content: string, sourceSession?: string): number {
      // 检查条数上限
      const row = db
        .query("SELECT COUNT(*) as cnt FROM memories WHERE user_id = ?")
        .get(userId) as { cnt: number } | undefined;
      const current = row?.cnt ?? 0;
      if (current >= maxEntriesPerUser) {
        throw new MemoryLimitError(current, maxEntriesPerUser);
      }

      const ts = now();
      const stmt = db.prepare(
        "INSERT INTO memories (user_id, content, created_at, updated_at, source_session) VALUES (?, ?, ?, ?, ?)",
      );
      const result = stmt.run(userId, content, ts, ts, sourceSession ?? null);
      return Number(result.lastInsertRowid);
    },

    get(id: number): MemoryRow | null {
      return db
        .query("SELECT * FROM memories WHERE id = ?")
        .get(id) as MemoryRow | null;
    },

    search(userId: number, keyword: string): MemoryRow[] {
      return db
        .query("SELECT * FROM memories WHERE user_id = ? AND content LIKE ? ORDER BY updated_at DESC")
        .all(userId, `%${keyword}%`) as MemoryRow[];
    },

    list(userId: number, offset?: number, limit?: number): MemoryRow[] {
      return db
        .query("SELECT * FROM memories WHERE user_id = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?")
        .all(userId, limit ?? 200, offset ?? 0) as MemoryRow[];
    },

    update(id: number, content: string): void {
      db
        .prepare("UPDATE memories SET content = ?, updated_at = ? WHERE id = ?")
        .run(content, now(), id);
    },

    delete_(id: number): void {
      db.prepare("DELETE FROM memories WHERE id = ?").run(id);
    },

    count(userId: number): number {
      const row = db
        .query("SELECT COUNT(*) as cnt FROM memories WHERE user_id = ?")
        .get(userId) as { cnt: number } | undefined;
      return row?.cnt ?? 0;
    },

    reassignUser(sourceUserId: number, targetUserId: number): void {
      db.prepare(
        "UPDATE memories SET user_id = ? WHERE user_id = ?",
      ).run(targetUserId, sourceUserId);
    },
  };
}
