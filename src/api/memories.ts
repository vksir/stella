import { Elysia, t } from "elysia";
import type { MemoryStore } from "../stores/memory";
import type { IdentityResolver } from "../identity";
import { authenticate, requireOwnerOrAdmin, statusError } from "./auth";

export function memoryRoutes(
  memoryStore: MemoryStore,
  identity: IdentityResolver,
) {
  const app = new Elysia()

    // GET /users/:id/memories — 列出某用户的记忆
    .get(
      "/users/:id/memories",
      ({ headers, params: { id } }) => {
        const user = authenticate(headers["authorization"], identity);
        const targetUserId = Number(id);

        if (isNaN(targetUserId)) {
          throw statusError(400, "Invalid user id");
        }

        requireOwnerOrAdmin(user, targetUserId);

        const memories = memoryStore.list(targetUserId);
        return memories.map((m) => ({
          id: m.id,
          content: m.content,
          created_at: m.created_at,
          updated_at: m.updated_at,
        }));
      },
      {
        detail: {
          summary: "列出用户记忆",
          description: "列出指定用户的所有长期记忆。需为 admin 或记忆所属用户本人。",
        },
      },
    )

    // PUT /memories/:id — 更新记忆内容
    .put(
      "/memories/:id",
      ({ headers, params: { id }, body }) => {
        const user = authenticate(headers["authorization"], identity);
        const memoryId = Number(id);
        const { content } = body as { content: string };

        if (isNaN(memoryId)) {
          throw statusError(400, "Invalid memory id");
        }
        if (!content || typeof content !== "string") {
          throw statusError(400, "content is required");
        }

        const memory = memoryStore.get(memoryId);
        if (!memory) {
          throw statusError(404, "Memory not found");
        }

        requireOwnerOrAdmin(user, memory.user_id);

        memoryStore.update(memoryId, content);

        return { id: memoryId, content };
      },
      {
        body: t.Object({
          content: t.String({ description: "新的记忆内容" }),
        }),
        detail: {
          summary: "更新记忆",
          description: "更新指定记忆的内容。需为记忆所有者或 admin。",
        },
      },
    )

    // DELETE /memories/:id — 删除记忆
    .delete(
      "/memories/:id",
      ({ headers, params: { id }, set }) => {
        const user = authenticate(headers["authorization"], identity);
        const memoryId = Number(id);

        if (isNaN(memoryId)) {
          throw statusError(400, "Invalid memory id");
        }

        const memory = memoryStore.get(memoryId);
        if (!memory) {
          throw statusError(404, "Memory not found");
        }

        requireOwnerOrAdmin(user, memory.user_id);

        memoryStore.delete_(memoryId);

        set.status = 204;
        return "";
      },
      {
        detail: {
          summary: "删除记忆",
          description: "删除指定记忆。需为记忆所有者或 admin。",
        },
      },
    );

  return app;
}
