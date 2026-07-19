import { defineTool } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";
import type { MemoryStore, MemoryRow } from "../stores/memory";
import { MemoryLimitError } from "../stores/memory";
import type { UserRow } from "../stores/user";
import type { ToolDefinition } from "@earendil-works/pi-coding-agent";

/**
 * "当前说话人"查询回调：给定会话 ID，返回该会话的当前用户。
 * 由上层（平台适配器）在消息到达时注入。
 */
export type GetUserForSession = (sessionId: string) => UserRow | null;

/**
 * 错误返回辅助函数
 */
function err(text: string) {
  return {
    content: [{ type: "text" as const, text }],
    details: { error: true },
  };
}

/**
 * 创建记忆系统自定义工具集合。
 *
 * @param memories  记忆存储实例
 * @param maxContentChars  单条记忆内容最大字符数
 * @param getUserForSession  根据 sessionId 获取当前说话人 UserRow 的回调
 * @returns 4 个 ToolDefinition 的数组
 */
export function createMemoryTools(
  memories: MemoryStore,
  maxContentChars: number,
  getUserForSession: GetUserForSession,
): ToolDefinition[] {

  // ---- memory_save ----

  const memory_save = defineTool({
    name: "memory_save",
    label: "保存记忆",
    description: "保存一条长期记忆。传入 content 创建新记忆；同时传入 id 则覆盖更新已有记忆。",
    parameters: Type.Object({
      content: Type.String({ description: "记忆内容" }),
      id: Type.Optional(Type.Number({ description: "记忆 ID（更新已有记忆时传入）" })),
    }),
    promptGuidelines: [
      "该存：用户明确要求记住的信息、稳定的个人偏好、长期事实（生日、地址等）。",
      "不该存：一次性请求、已存入的重复内容、密码/凭据等敏感信息。",
      "该更新：用户纠正之前的信息或偏好变化时，带 id 覆盖更新。",
    ],
    execute: async (_toolCallId, params, _signal, _onUpdate, ctx) => {
      const sessionId = ctx.sessionManager.getSessionId();
      const user = getUserForSession(sessionId);
      if (!user) {
        return err("无法确定当前用户，请先绑定身份。");
      }

      // 内容超长检查
      if (params.content.length > maxContentChars) {
        return err(`记忆内容过长（${params.content.length} 字符），上限为 ${maxContentChars} 字符。请精简后再存。`);
      }

      if (params.id != null) {
        // 更新已有记忆
        const existing = memories.get(params.id);
        if (!existing) {
          return err(`找不到 id=${params.id} 的记忆，可能已被删除。`);
        }
        if (existing.user_id !== user.id) {
          return err(`找不到 id=${params.id} 的记忆（无权访问）。`);
        }

        memories.update(params.id, params.content);

        return {
          content: [{ type: "text", text: `已更新记忆 #${params.id}` }],
          details: { id: params.id, content: params.content, user_id: user.id },
        };
      } else {
        // 新建记忆：条数上限已由 MemoryStore 检查，这里捕获溢出
        try {
          const id = memories.create(user.id, params.content);

          return {
            content: [{ type: "text", text: `已保存记忆 #${id}` }],
            details: { id, content: params.content, user_id: user.id },
          };
        } catch (e) {
          if (e instanceof MemoryLimitError) {
            return err(e.message);
          }
          throw e;
        }
      }
    },
  });

  // ---- memory_search ----

  const memory_search = defineTool({
    name: "memory_search",
    label: "搜索记忆",
    description: "在长期记忆中模糊搜索当前用户已存的内容，返回匹配的记忆列表。",
    parameters: Type.Object({
      keyword: Type.String({ description: "搜索关键词" }),
    }),
    promptGuidelines: [
      "在需要回忆用户之前提过的信息时使用此工具。",
      "关键词应尽量具体，避免过于宽泛的词。",
    ],
    execute: async (_toolCallId, params, _signal, _onUpdate, ctx) => {
      const sessionId = ctx.sessionManager.getSessionId();
      const user = getUserForSession(sessionId);
      if (!user) {
        return err("无法确定当前用户，请先绑定身份。");
      }

      const results = memories.search(user.id, params.keyword);

      // 只返回 id + content，不暴露内部字段
      const items = results.map((r: MemoryRow) => ({
        id: r.id,
        content: r.content,
      }));

      return {
        content: [{ type: "text", text: items.length === 0 ? "未找到匹配的记忆。" : `找到 ${items.length} 条记忆` }],
        details: { results: items },
      };
    },
  });

  // ---- memory_list ----

  const memory_list = defineTool({
    name: "memory_list",
    label: "列出记忆",
    description: "列出当前用户的所有长期记忆，支持分页。",
    parameters: Type.Object({
      offset: Type.Optional(Type.Number({ description: "分页偏移量（从 0 开始）" })),
      limit: Type.Optional(Type.Number({ description: "每页条数（默认 50）" })),
    }),
    promptGuidelines: [
      "当用户要求查看全部记忆或回顾之前记录的信息时使用。",
      "结果较多时可用 offset/limit 分页浏览。",
    ],
    execute: async (_toolCallId, params, _signal, _onUpdate, ctx) => {
      const sessionId = ctx.sessionManager.getSessionId();
      const user = getUserForSession(sessionId);
      if (!user) {
        return err("无法确定当前用户，请先绑定身份。");
      }

      const results = memories.list(user.id, params.offset, params.limit);

      const items = results.map((r: MemoryRow) => ({
        id: r.id,
        content: r.content,
      }));

      const total = memories.count(user.id);

      return {
        content: [{ type: "text", text: items.length === 0 ? "暂无记忆。" : `共 ${total} 条记忆（当前返回 ${items.length} 条）` }],
        details: { results: items, total },
      };
    },
  });

  // ---- memory_delete ----

  const memory_delete = defineTool({
    name: "memory_delete",
    label: "删除记忆",
    description: "按 id 删除当前用户的一条长期记忆。",
    parameters: Type.Object({
      id: Type.Number({ description: "要删除的记忆 ID" }),
    }),
    promptGuidelines: [
      "当用户要求忘记/删除某条信息时使用。",
      "只能删除当前用户的记忆，无法删除他人的。",
    ],
    execute: async (_toolCallId, params, _signal, _onUpdate, ctx) => {
      const sessionId = ctx.sessionManager.getSessionId();
      const user = getUserForSession(sessionId);
      if (!user) {
        return err("无法确定当前用户，请先绑定身份。");
      }

      const existing = memories.get(params.id);
      if (!existing) {
        return err(`找不到 id=${params.id} 的记忆，可能已被删除。`);
      }
      if (existing.user_id !== user.id) {
        return err(`找不到 id=${params.id} 的记忆（无权访问）。`);
      }

      memories.delete_(params.id);

      return {
        content: [{ type: "text", text: `已删除记忆 #${params.id}` }],
        details: { id: params.id },
      };
    },
  });

  return [memory_save, memory_search, memory_list, memory_delete];
}
