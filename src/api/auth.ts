import type { IdentityResolver } from "../identity";
import type { UserRow } from "./types";

/** 创建带 HTTP 状态码的错误对象。 */
export function statusError(status: number, message: string): Error {
  const err = new Error(message) as Error & { status: number };
  err.status = status;
  return err;
}

/** 从 Authorization header 提取 Bearer token 并解析用户，失败抛 401。 */
export function authenticate(
  bearerHeader: string | undefined,
  identity: IdentityResolver,
): UserRow {
  if (!bearerHeader || !bearerHeader.startsWith("Bearer ")) {
    throw statusError(401, "Unauthorized");
  }
  const token = bearerHeader.slice("Bearer ".length);
  if (!token) {
    throw statusError(401, "Unauthorized");
  }
  return identity.resolve("api", token);
}

/** 断言当前用户为 admin，否则抛 403。 */
export function requireAdmin(user: UserRow): void {
  if (user.role !== "admin") {
    throw statusError(403, "Forbidden: admin only");
  }
}

/** 断言当前用户是资源所有者或 admin，否则抛 403。 */
export function requireOwnerOrAdmin(user: UserRow, resourceOwnerId: number): void {
  if (user.role !== "admin" && user.id !== resourceOwnerId) {
    throw statusError(403, "Forbidden: not your resource");
  }
}
