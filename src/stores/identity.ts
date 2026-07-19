import type { Database } from "bun:sqlite";
import type { UserRow } from "./user";

export interface IdentityStore {
  /** 绑定平台身份到用户。重复绑定静默忽略。 */
  link(userId: number, platform: string, platformUserId: string): void;
  /** 通过平台身份查找用户，未绑定时返回 null。 */
  resolve(platform: string, platformUserId: string): UserRow | null;
  /** 将 source 用户的所有平台身份转移给 target 用户。 */
  reassignUser(sourceUserId: number, targetUserId: number): void;
}

export function createIdentityStore(db: Database): IdentityStore {
  const now = () => Math.floor(Date.now() / 1000);

  return {
    link(userId: number, platform: string, platformUserId: string): void {
      // INSERT OR IGNORE 处理重复绑定
      db.prepare(
        `INSERT OR IGNORE INTO user_identities (user_id, platform, platform_user_id, created_at)
         VALUES (?, ?, ?, ?)`,
      ).run(userId, platform, platformUserId, now());
    },

    resolve(platform: string, platformUserId: string): UserRow | null {
      return db
        .query(
          `SELECT u.* FROM users u
           JOIN user_identities i ON u.id = i.user_id
           WHERE i.platform = ? AND i.platform_user_id = ?`,
        )
        .get(platform, platformUserId) as UserRow | null;
    },

    reassignUser(sourceUserId: number, targetUserId: number): void {
      db.prepare(
        "UPDATE user_identities SET user_id = ? WHERE user_id = ?",
      ).run(targetUserId, sourceUserId);
    },
  };
}
