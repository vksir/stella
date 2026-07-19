import { Elysia, t } from "elysia";
import type { UserStore } from "../stores/user";
import type { IdentityStore } from "../stores/identity";
import type { MemoryStore } from "../stores/memory";
import type { IdentityResolver } from "../identity";
import { authenticate, requireAdmin, statusError } from "./auth";

export function userRoutes(
  users: UserStore,
  identityStore: IdentityStore,
  memoryStore: MemoryStore,
  identity: IdentityResolver,
  transaction: <T>(fn: () => T) => T,
) {
  const app = new Elysia()

    // GET /users — 用户列表
    .get(
      "/users",
      ({ headers }) => {
        authenticate(headers["authorization"], identity);

        return users.listAll();
      },
      {
        detail: {
          summary: "用户列表",
          description: "返回所有注册用户。",
        },
      },
    )

    // POST /users/:id/merge — 合并用户（仅 admin）
    .post(
      "/users/:id/merge",
      ({ headers, params: { id }, body }) => {
        const user = authenticate(headers["authorization"], identity);
        requireAdmin(user);

        const sourceUserId = Number(id);
        const { target_user_id: targetUserId } = body as { target_user_id: number };

        if (isNaN(sourceUserId)) {
          throw statusError(400, "Invalid source user id");
        }
        if (!targetUserId || typeof targetUserId !== "number") {
          throw statusError(400, "target_user_id is required");
        }

        const sourceUser = users.get(sourceUserId);
        if (!sourceUser) {
          throw statusError(404, "Source user not found");
        }

        const targetUser = users.get(targetUserId);
        if (!targetUser) {
          throw statusError(404, "Target user not found");
        }

        if (sourceUserId === targetUserId) {
          throw statusError(400, "Cannot merge a user into itself");
        }

        transaction(() => {
          identityStore.reassignUser(sourceUserId, targetUserId);
          memoryStore.reassignUser(sourceUserId, targetUserId);
          users.delete_(sourceUserId);
        });

        return { success: true, target_user_id: targetUserId };
      },
      {
        body: t.Object({
          target_user_id: t.Number({ description: "目标用户 ID" }),
        }),
        detail: {
          summary: "合并用户",
          description: "将源用户的 identity 和 memories 转移到目标用户后删除源用户。仅 admin 可操作。",
        },
      },
    );

  return app;
}
