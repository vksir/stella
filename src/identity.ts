import type { UserStore, UserRow } from "./stores/user";
import type { IdentityStore } from "./stores/identity";
import type { StellaConfig } from "./config";

export interface IdentityResolver {
  /**
   * 按 (platform, platform_user_id) 解析用户身份。
   * 首次接触自动建档（role=guest）。
   * 管理员的 QQ 号与 API token 绑定到同一 admin 用户。
   */
  resolve(platform: string, platformUserId: string): UserRow;
}

export function createIdentityResolver(
  users: UserStore,
  identities: IdentityStore,
  config: StellaConfig,
): IdentityResolver {
  let ownerEnsured = false;

  function ensureOwner(): void {
    if (ownerEnsured) return;
    ownerEnsured = true;

    const qqOwner = identities.resolve("qq", config.owner.qq);
    const apiOwner = identities.resolve("api", config.owner.api_token);

    if (qqOwner && apiOwner) {
      if (qqOwner.id !== apiOwner.id) {
        // 以 QQ 身份为准
        users.updateRole(qqOwner.id, "admin");
      } else {
        users.updateRole(qqOwner.id, "admin");
      }
    } else if (qqOwner) {
      users.updateRole(qqOwner.id, "admin");
      identities.link(qqOwner.id, "api", config.owner.api_token);
    } else if (apiOwner) {
      users.updateRole(apiOwner.id, "admin");
      identities.link(apiOwner.id, "qq", config.owner.qq);
    } else {
      const ownerId = users.create("管理员", "admin");
      identities.link(ownerId, "qq", config.owner.qq);
      identities.link(ownerId, "api", config.owner.api_token);
    }
  }

  return {
    resolve(platform: string, platformUserId: string): UserRow {
      ensureOwner();

      const existing = identities.resolve(platform, platformUserId);
      if (existing) return existing;

      const displayName = `${platform}:${platformUserId}`;
      const userId = users.create(displayName, "guest");
      identities.link(userId, platform, platformUserId);
      return users.get(userId)!;
    },
  };
}
