/**
 * QQ 适配器 — ticket 05
 *
 * Bun 原生 WS 服务端接收 NapCat 反向 WebSocket 连接。
 * 消息管线：触发判定 → 身份解析 → 会话运行时 → 输出解析 → API 发送。
 *
 * 模块导出纯函数（便于单测）和 startQQAdapter 主入口。
 */

import type { AppContext } from "../../index";
import type {
  ExtensionFactory,
  ExtensionAPI,
  BeforeAgentStartEvent,
  BeforeAgentStartEventResult,
} from "@earendil-works/pi-coding-agent";
import type { AgentSession } from "@earendil-works/pi-coding-agent";
import type { Segment, SenderInfo } from "./types";
import { SELF_NAME } from "./types";
import { segmentsToText, formatGroupMessage, parseOutbound, stripReplySegments } from "./format";
import { isTrigger, triggerNoteText } from "./trigger";

// ---- 类型定义 ----

/** WS API 调用回执 */
export interface ApiResponse {
  status: string;
  retcode: number;
  data: Record<string, unknown>;
  echo?: string;
  message?: string;
  wording?: string;
}

/** OneBot 消息事件（精简） */
interface OneBotMessageEvent {
  post_type: "message";
  message_type: "private" | "group";
  message_id: number;
  user_id: number;
  group_id?: number;
  self_id: number;
  time: number;
  sender: {
    nickname: string;
    card?: string;
  };
  message: Segment[];
}

/** OneBot 元事件 */
interface OneBotMetaEvent {
  post_type: "meta_event";
  meta_event_type: "lifecycle" | "heartbeat";
  self_id: number;
  time: number;
  sub_type?: string;
  status?: { online: boolean; good: boolean };
  interval?: number;
}

type OneBotEvent = OneBotMessageEvent | OneBotMetaEvent;

// ---- 连接状态 ----

interface ConnectionState {
  ws: import("bun").ServerWebSocket<unknown>;
  selfId: string;
  connected: boolean;
  lastHeartbeat: number;
  /** 已知活跃会话的 chatKey 集合，用于断连补偿 */
  knownChats: Set<string>;
}

/** echo UUID → resolve 映射（等待 API 回执） */
const pendingEchoes = new Map<string, (resp: ApiResponse) => void>();

// ---- UUID 工具 ----

function uuid(): string {
  return crypto.randomUUID();
}

// ---- 消息发送 ----

/**
 * 通过 WS 发送 API 调用并等待回执。
 */
function sendApi(
  ws: import("bun").ServerWebSocket<unknown>,
  action: string,
  params: Record<string, unknown>,
  timeoutMs: number = 10000,
): Promise<ApiResponse> {
  const echo = uuid();
  const payload = JSON.stringify({ action, params, echo });

  return new Promise<ApiResponse>((resolve, reject) => {
    const timer = setTimeout(() => {
      pendingEchoes.delete(echo);
      reject(new Error(`API 调用超时: ${action} (echo=${echo})`));
    }, timeoutMs);

    pendingEchoes.set(echo, (resp: ApiResponse) => {
      clearTimeout(timer);
      resolve(resp);
    });

    ws.send(payload);
  });
}

/**
 * 发送群聊消息（含降级重试）。
 */
async function sendGroupMessage(
  ws: import("bun").ServerWebSocket<unknown>,
  groupId: number,
  segments: Segment[],
): Promise<void> {
  try {
    const resp = await sendApi(ws, "send_group_msg", {
      group_id: groupId,
      message: segments,
    });

    // 引用过期/reply 相关错误 → 去掉 reply 段重发
    if (resp.retcode !== 0 && hasReplySegment(segments)) {
      console.log(`[QQ] 群 ${groupId} 发送失败（retcode=${resp.retcode}），去掉 reply 重发`);
      const stripped = stripReplySegments(segments);
      await sendApi(ws, "send_group_msg", {
        group_id: groupId,
        message: stripped,
      });
    }
  } catch (err) {
    console.error(`[QQ] 群 ${groupId} 发送异常:`, err);
  }
}

/**
 * 发送私聊消息。
 */
async function sendPrivateMessage(
  ws: import("bun").ServerWebSocket<unknown>,
  userId: number,
  segments: Segment[],
): Promise<void> {
  try {
    await sendApi(ws, "send_private_msg", {
      user_id: userId,
      message: segments,
    });
  } catch (err) {
    console.error(`[QQ] 私聊 ${userId} 发送异常:`, err);
  }
}

function hasReplySegment(segs: Segment[]): boolean {
  return segs.some((s) => s.type === "reply");
}

// ---- 被动消息入库 ----

/**
 * 群聊被动消息（不触发回复）格式化后写入会话历史。
 */
async function injectPassiveMessage(
  session: AgentSession,
  segs: Segment[],
  sender: SenderInfo,
  messageId: number,
  time: number,
  userId: number,
): Promise<void> {
  const text = formatGroupMessage(segs, sender, messageId, time, userId);
  try {
    (session as any).sendCustomMessage(text, { deliverAs: "nextTurn" });
  } catch (err) {
    console.error("[QQ] 被动消息入库失败:", err);
  }
}

// ---- before_agent_start 扩展 ----

/**
 * 待注入的触发注记（按 sessionId 索引）。
 */
const pendingTriggerNotes = new Map<string, string>();

/**
 * 为指定会话设置当轮触发注记。
 */
export function setPendingTriggerNote(sessionId: string, note: string): void {
  pendingTriggerNotes.set(sessionId, note);
}

/**
 * 创建触发注记扩展工厂。
 * 在 before_agent_start 事件中注入触发注记（当轮有效，不入库）。
 */
export function createTriggerNoteExtension(): ExtensionFactory {
  return (pi: ExtensionAPI) => {
    pi.on("before_agent_start", (event: BeforeAgentStartEvent, ctx) => {
      const sessionId = ctx.sessionManager.getSessionId();
      const note = pendingTriggerNotes.get(sessionId);
      if (!note) return;

      pendingTriggerNotes.delete(sessionId);

      // 将触发注记追加到系统提示词末尾（当轮有效）
      const result: BeforeAgentStartEventResult = {
        systemPrompt: event.systemPrompt + "\n\n" + note,
      };
      return result;
    });
  };
}

// ---- 消息处理 ----

/**
 * 处理单条 OneBot message 事件。
 */
async function handleMessage(
  ctx: AppContext,
  conn: ConnectionState,
  data: OneBotMessageEvent,
): Promise<void> {
  const selfId = String(data.self_id);
  const { message_type } = data;

  const triggered = isTrigger(message_type, data.message, selfId);

  if (message_type === "group") {
    const groupId = data.group_id!;
    const chatKey = `qq:group:${groupId}`;
    conn.knownChats.add(chatKey);

    if (triggered) {
      // ---- 群聊触发 ----
      const user = ctx.identity.resolve("qq", String(data.user_id));
      const session = await ctx.sessions.getOrCreate("qq", chatKey);
      ctx.setSessionUser(session.sessionId, user);

      // 设置触发注记
      const note = triggerNoteText("group", data.sender, data.message_id, data.user_id);
      setPendingTriggerNote(session.sessionId, note);

      // 格式化消息
      const formatted = formatGroupMessage(
        data.message, data.sender, data.message_id, data.time, data.user_id,
      );

      await processPromptAndReply(ctx, conn, session, formatted, "group", groupId, data.user_id);
    } else {
      // ---- 群聊被动消息（仅入库） ----
      const session = await ctx.sessions.getOrCreate("qq", chatKey);
      await injectPassiveMessage(
        session, data.message, data.sender, data.message_id, data.time, data.user_id,
      );
    }
  } else {
    // ---- 私聊 ----
    const chatKey = `qq:private:${data.user_id}`;
    conn.knownChats.add(chatKey);

    const user = ctx.identity.resolve("qq", String(data.user_id));
    const session = await ctx.sessions.getOrCreate("qq", chatKey);
    ctx.setSessionUser(session.sessionId, user);

    // 私聊触发注记
    const note = triggerNoteText("private", data.sender, data.message_id, data.user_id);
    setPendingTriggerNote(session.sessionId, note);

    // 私聊：纯文本（不标注说话人）
    const text = segmentsToText(data.message, SELF_NAME);

    await processPromptAndReply(ctx, conn, session, text, "private", undefined, data.user_id);
  }
}

/**
 * 调用 session.prompt 并收集模型回复 → 解析 → 发送。
 */
async function processPromptAndReply(
  _ctx: AppContext,
  conn: ConnectionState,
  session: AgentSession,
  promptText: string,
  chatType: "group" | "private",
  groupId: number | undefined,
  senderUserId: number,
): Promise<void> {
  // 开始收集回复
  const replyPromise = collectReply(session);

  try {
    await session.prompt(promptText);
  } catch (err) {
    console.error("[QQ] session.prompt 失败:", err);
    return;
  }

  const replyText = await replyPromise;

  if (!replyText.trim()) return;

  // 解析模型输出为段数组
  const senderQq = chatType === "group" ? String(senderUserId) : undefined;
  const segments = parseOutbound(replyText, chatType, senderQq);

  // 发送
  if (chatType === "group" && groupId) {
    await sendGroupMessage(conn.ws, groupId, segments);
  } else {
    await sendPrivateMessage(conn.ws, senderUserId, segments);
  }
}

/**
 * 订阅会话事件流，收集 text_delta 累积完整回复文本。
 * 返回 Promise，在 agent_end 时 resolve 完整文本。
 */
function collectReply(session: AgentSession): Promise<string> {
  return new Promise<string>((resolve) => {
    let fullText = "";
    const unsub = session.subscribe((event: any) => {
      if (event.type === "message_update" && event.assistantMessageEvent?.type === "text_delta") {
        fullText += event.assistantMessageEvent.delta;
      }
      if (event.type === "agent_end") {
        unsub();
        resolve(fullText);
      }
    });
  });
}

// ---- 元事件处理 ----

const CATCHUP_COUNT = 20;

async function handleMetaEvent(
  ctx: AppContext,
  conn: ConnectionState,
  data: OneBotMetaEvent,
): Promise<void> {
  if (data.meta_event_type === "lifecycle" && data.sub_type === "connect") {
    conn.connected = true;
    console.log(`[QQ] NapCat 已连接 (self_id=${conn.selfId})`);

    // 断连补偿：拉取消息空洞
    await catchupMissedMessages(ctx, conn);
  } else if (data.meta_event_type === "heartbeat") {
    conn.lastHeartbeat = Date.now();
    if (data.status) {
      console.log(
        `[QQ] 心跳 (online=${data.status.online}, good=${data.status.good}, interval=${data.interval}ms)`,
      );
    }
  }
}

/**
 * 断连补偿：对每个已知活跃会话拉取历史消息。
 */
async function catchupMissedMessages(
  ctx: AppContext,
  conn: ConnectionState,
): Promise<void> {
  for (const chatKey of conn.knownChats) {
    try {
      if (chatKey.startsWith("qq:group:")) {
        const groupId = parseInt(chatKey.slice("qq:group:".length), 10);
        await catchupGroupMessages(ctx, conn, groupId, chatKey);
      } else if (chatKey.startsWith("qq:private:")) {
        const userId = parseInt(chatKey.slice("qq:private:".length), 10);
        await catchupPrivateMessages(ctx, conn, userId, chatKey);
      }
    } catch (err) {
      console.error(`[QQ] 断连补偿失败 (${chatKey}):`, err);
    }
  }
}

async function catchupGroupMessages(
  ctx: AppContext,
  conn: ConnectionState,
  groupId: number,
  chatKey: string,
): Promise<void> {
  const resp = await sendApi(conn.ws, "get_group_msg_history", {
    group_id: groupId,
    count: CATCHUP_COUNT,
  });

  if (resp.retcode !== 0 || !resp.data) return;

  const messages = (resp.data as any).messages as any[] | undefined;
  if (!messages || messages.length === 0) return;

  const row = ctx.sessionStore.get("qq", chatKey);
  const cutoff = row ? row.last_active : 0;

  const session = await ctx.sessions.getOrCreate("qq", chatKey);

  // 按时间正序处理
  const sorted = [...messages].sort((a, b) => (a.time ?? 0) - (b.time ?? 0));

  for (const msg of sorted) {
    if ((msg.time ?? 0) <= cutoff) continue;
    if (msg.post_type !== "message") continue;

    const msgTime = msg.time ?? Math.floor(Date.now() / 1000);
    const sender: SenderInfo = {
      nickname: msg.sender?.nickname ?? "未知",
      card: msg.sender?.card,
    };
    const segs: Segment[] = Array.isArray(msg.message)
      ? msg.message
      : [{ type: "text", data: { text: String(msg.message ?? "") } }];

    // 跳过机器人自己的消息
    if (String(msg.user_id) === conn.selfId) continue;

    const text = formatGroupMessage(segs, sender, msg.message_id ?? 0, msgTime, msg.user_id ?? 0);
    try {
      (session as any).sendCustomMessage(text, { deliverAs: "nextTurn" });
    } catch { /* ignore */ }
  }
}

async function catchupPrivateMessages(
  ctx: AppContext,
  conn: ConnectionState,
  userId: number,
  chatKey: string,
): Promise<void> {
  const resp = await sendApi(conn.ws, "get_friend_msg_history", {
    user_id: userId,
    count: CATCHUP_COUNT,
  });

  if (resp.retcode !== 0 || !resp.data) return;

  const messages = (resp.data as any).messages as any[] | undefined;
  if (!messages || messages.length === 0) return;

  const row = ctx.sessionStore.get("qq", chatKey);
  const cutoff = row ? row.last_active : 0;

  const session = await ctx.sessions.getOrCreate("qq", chatKey);

  const sorted = [...messages].sort((a, b) => (a.time ?? 0) - (b.time ?? 0));

  for (const msg of sorted) {
    if ((msg.time ?? 0) <= cutoff) continue;
    if (msg.post_type !== "message") continue;
    if (String(msg.user_id) === conn.selfId) continue;

    const segs: Segment[] = Array.isArray(msg.message)
      ? msg.message
      : [{ type: "text", data: { text: String(msg.message ?? "") } }];
    const text = segmentsToText(segs, SELF_NAME);

    try {
      (session as any).sendCustomMessage(text, { deliverAs: "nextTurn" });
    } catch { /* ignore */ }
  }
}

// ---- WS 服务端 ----

/** WS 连接的自定义 data 类型 */
interface WSData {
  authHeader: string | null;
  selfIdHeader: string | null;
}

/**
 * 启动 QQ 适配器（反向 WS 服务端）。
 */
export interface QQAdapterHandle {
  stop(): void;
}

export async function startQQAdapter(ctx: AppContext): Promise<QQAdapterHandle> {
  const { napcat } = ctx.config;
  const [host, portStr] = napcat.listen.split(":");
  const port = parseInt(portStr!, 10);

  console.log(`[QQ] 启动 NapCat 反向 WS 服务端: ${napcat.listen}`);

  type QQWebSocket = import("bun").ServerWebSocket<WSData>;

  const server = Bun.serve<WSData>({
    hostname: host,
    port,
    fetch(req, server) {
      // 解析握手头
      const auth = req.headers.get("Authorization");
      const selfId = req.headers.get("X-Self-ID");

      // 鉴权：校验 Bearer token
      const expectedToken = `Bearer ${napcat.token}`;
      if (napcat.token && auth !== expectedToken) {
        console.log("[QQ] WS 鉴权失败，返回 401");
        return new Response("Unauthorized", { status: 401 });
      }

      if (!selfId) {
        console.log("[QQ] WS 缺少 X-Self-ID 头");
        return new Response("Missing X-Self-ID", { status: 400 });
      }

      // 升级 WebSocket，传递握手头信息
      const upgraded = server.upgrade(req, {
        data: { authHeader: auth, selfIdHeader: selfId },
      });

      if (!upgraded) {
        return new Response("WebSocket upgrade failed", { status: 500 });
      }

      return undefined; // 升级后无需返回 Response
    },
    websocket: {
      open(ws) {
        const selfId = ws.data.selfIdHeader;
        if (!selfId) {
          ws.close(4001, "Missing X-Self-ID");
          return;
        }

        const conn: ConnectionState = {
          ws: ws as unknown as import("bun").ServerWebSocket<unknown>,
          selfId,
          connected: false,
          lastHeartbeat: Date.now(),
          knownChats: new Set(),
        };

        (ws as any).__qqConn = conn;

        console.log(`[QQ] NapCat 已连接 WebSocket (self_id=${selfId})`);
      },

      message(ws, raw) {
        const conn = (ws as any).__qqConn as ConnectionState | undefined;
        if (!conn) return;

        let data: Record<string, unknown>;
        try {
          data = JSON.parse(raw as string) as Record<string, unknown>;
        } catch {
          console.error("[QQ] 无法解析 WS 消息:", String(raw).slice(0, 200));
          return;
        }

        // 检查是否为 API 回执（含 echo 和 status）
        if ("echo" in data && typeof data.echo === "string" && "status" in data) {
          const resp = data as unknown as ApiResponse;
          const handler = pendingEchoes.get(resp.echo!);
          if (handler) {
            pendingEchoes.delete(resp.echo!);
            handler(resp);
          }
          return;
        }

        const postType = data.post_type as string | undefined;

        // 按 post_type 分流
        if (postType === "message") {
          handleMessage(ctx, conn, data as unknown as OneBotMessageEvent).catch((err) => {
            console.error("[QQ] 消息处理异常:", err);
          });
        } else if (postType === "meta_event") {
          handleMetaEvent(ctx, conn, data as unknown as OneBotMetaEvent).catch((err) => {
            console.error("[QQ] 元事件处理异常:", err);
          });
        } else if (postType === "message_sent") {
          console.log(`[QQ] message_sent (忽略): user=${data.user_id}`);
        } else if (postType === "notice") {
          console.log(`[QQ] notice (忽略): type=${data.notice_type}`);
        } else if (postType === "request") {
          console.log(`[QQ] request (忽略): type=${data.request_type}`);
        } else {
          console.log(`[QQ] 未知 post_type: ${postType}`);
        }
      },

      close(ws) {
        const conn = (ws as any).__qqConn as ConnectionState | undefined;
        if (conn) {
          conn.connected = false;
          console.log(`[QQ] NapCat 已断开 (self_id=${conn.selfId})`);
        }
      },
    },
  });

  console.log(`[QQ] WS 服务端已启动: ws://${napcat.listen}`);

  return {
    stop: () => { server.stop(true); },
  };
}
