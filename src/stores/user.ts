import type { Database } from "bun:sqlite";

export interface UserRow {
  id: number;
  display_name: string;
  role: "admin" | "guest";
  created_at: number;
}

export interface UserStore {
  create(displayName: string, role: "admin" | "guest"): number;
  get(id: number): UserRow | null;
  listAll(): UserRow[];
  updateRole(id: number, role: "admin" | "guest"): void;
  delete_(id: number): void;
}

export function createUserStore(db: Database): UserStore {
  const now = () => Math.floor(Date.now() / 1000);

  return {
    create(displayName: string, role: "admin" | "guest"): number {
      const stmt = db.prepare(
        "INSERT INTO users (display_name, role, created_at) VALUES (?, ?, ?)",
      );
      const result = stmt.run(displayName, role, now());
      return Number(result.lastInsertRowid);
    },

    get(id: number): UserRow | null {
      return db
        .query("SELECT * FROM users WHERE id = ?")
        .get(id) as UserRow | null;
    },

    listAll(): UserRow[] {
      return db
        .query("SELECT id, display_name, role, created_at FROM users")
        .all() as UserRow[];
    },

    updateRole(id: number, role: "admin" | "guest"): void {
      db.prepare("UPDATE users SET role = ? WHERE id = ?").run(role, id);
    },

    delete_(id: number): void {
      db.prepare("DELETE FROM users WHERE id = ?").run(id);
    },
  };
}
