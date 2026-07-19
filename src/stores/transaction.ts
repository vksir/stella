import type { Database } from "bun:sqlite";

/**
 * 在事务中执行回调。回调抛错时自动 ROLLBACK，否则 COMMIT。
 * 事务期间数据库锁由 SQLite 自动管理。
 */
export function transaction<T>(db: Database, fn: () => T): T {
  db.run("BEGIN");
  try {
    const result = fn();
    db.run("COMMIT");
    return result;
  } catch (e) {
    db.run("ROLLBACK");
    throw e;
  }
}
