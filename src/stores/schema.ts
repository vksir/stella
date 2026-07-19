import type { Database } from "bun:sqlite";

const DDL = `
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY,
  display_name TEXT NOT NULL,
  role TEXT NOT NULL CHECK(role IN ('admin', 'guest')),
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS user_identities (
  user_id INTEGER NOT NULL REFERENCES users(id),
  platform TEXT NOT NULL,
  platform_user_id TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  UNIQUE(platform, platform_user_id)
);

CREATE TABLE IF NOT EXISTS chat_sessions (
  platform TEXT NOT NULL,
  chat_key TEXT NOT NULL,
  session_path TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  last_active INTEGER NOT NULL,
  PRIMARY KEY(platform, chat_key)
);

CREATE TABLE IF NOT EXISTS memories (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id),
  content TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  source_session TEXT
);
`;

/** 初始化数据库 schema（幂等）。同时开启 WAL + 外键约束。 */
export function runMigrations(db: Database): void {
  db.exec("PRAGMA journal_mode=WAL");
  db.exec("PRAGMA foreign_keys=ON");
  db.exec(DDL);
}
