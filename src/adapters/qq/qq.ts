/**
 * QQ 适配器 — ticket 05
 *
 * 组合根：装配 OneBot 传输客户端（onebot.ts）与消息管线，启动反向 WS 服务端。
 * 消息管线：触发判定 → 身份解析 → 会话运行时 → 输出解析 → 发送。
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
import type { Segment, SenderInfo, OneBotMessageEvent, OneBotMetaEvent } from "./types";
import { SELF_NAME } from "./types";
import { segmentsToText, formatGroupMessage, parseOutbound } from "./format";
import { isTrigger, triggerNoteText } from "./trigger";
import { startOneBotServer, type OneBotClient } from "./onebot";

// ---- 状态 ----

/**
 * 已知活跃会话的 chatKey 集合，用于断连补偿。
 * 模块级而非连接级：WS 重连后依然存活，补偿才能真正回填消息空洞。
 */
const knownChats = new Set<string>();

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
  client: OneBotClient,
  data: OneBotMessageEvent,
): Promise<void> {
  const selfId = String(data.self_id);
  const { message_type } = data;

  const triggered = isTrigger(message_type, data.message, selfId);

  if (message_type === "group") {
    const groupId = data.group_id!;
    const chatKey = `qq:group:${groupId}`;
    knownChats.add(chatKey);

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

      await processPromptAndReply(ctx, client, session, formatted, "group", groupId, data.user_id);
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
    knownChats.add(chatKey);

    const user = ctx.identity.resolve("qq", String(data.user_id));
    const session = await ctx.sessions.getOrCreate("qq", chatKey);
    ctx.setSessionUser(session.sessionId, user);

    // 私聊触发注记
    const note = triggerNoteText("private", data.sender, data.message_id, data.user_id);
    setPendingTriggerNote(session.sessionId, note);

    // 私聊：纯文本（不标注说话人）
    const text = segmentsToText(data.message, SELF_NAME);

    await processPromptAndReply(ctx, client, session, text, "private", undefined, data.user_id);
  }
}

/**
 * 调用 session.prompt 并收集模型回复 → 解析 → 发送。
 */
async function processPromptAndReply(
  _ctx: AppContext,
  client: OneBotClient,
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
    await client.sendGroupMessage(groupId, segments);
  } else {
    await client.sendPrivateMessage(senderUserId, segments);
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
  client: OneBotClient,
  data: OneBotMetaEvent,
): Promise<void> {
  if (data.meta_event_type === "lifecycle" && data.sub_type === "connect") {
    // 断连补偿：拉取消息空洞
    await catchupMissedMessages(ctx, client);
  }
}

/**
 * 断连补偿：对每个已知活跃会话拉取历史消息。
 */
async function catchupMissedMessages(
  ctx: AppContext,
  client: OneBotClient,
): Promise<void> {
  for (const chatKey of knownChats) {
    try {
      if (chatKey.startsWith("qq:group:")) {
        const groupId = parseInt(chatKey.slice("qq:group:".length), 10);
        await catchupGroupMessages(ctx, client, groupId, chatKey);
      } else if (chatKey.startsWith("qq:private:")) {
        const userId = parseInt(chatKey.slice("qq:private:".length), 10);
        await catchupPrivateMessages(ctx, client, userId, chatKey);
      }
    } catch (err) {
      console.error(`[QQ] 断连补偿失败 (${chatKey}):`, err);
    }
  }
}

async function catchupGroupMessages(
  ctx: AppContext,
  client: OneBotClient,
  groupId: number,
  chatKey: string,
): Promise<void> {
  const resp = await client.send("get_group_msg_history", {
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
    if (String(msg.user_id) === client.selfId) continue;

    const text = formatGroupMessage(segs, sender, msg.message_id ?? 0, msgTime, msg.user_id ?? 0);
    try {
      (session as any).sendCustomMessage(text, { deliverAs: "nextTurn" });
    } catch { /* ignore */ }
  }
}

async function catchupPrivateMessages(
  ctx: AppContext,
  client: OneBotClient,
  userId: number,
  chatKey: string,
): Promise<void> {
  const resp = await client.send("get_friend_msg_history", {
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
    if (String(msg.user_id) === client.selfId) continue;

    const segs: Segment[] = Array.isArray(msg.message)
      ? msg.message
      : [{ type: "text", data: { text: String(msg.message ?? "") } }];
    const text = segmentsToText(segs, SELF_NAME);

    try {
      (session as any).sendCustomMessage(text, { deliverAs: "nextTurn" });
    } catch { /* ignore */ }
  }
}

// ---- 组合根 ----

/**
 * 启动 QQ 适配器（反向 WS 服务端）。
 */
export interface QQAdapterHandle {
  stop(): void;
}

export async function startQQAdapter(ctx: AppContext): Promise<QQAdapterHandle> {
  const { napcat } = ctx.config;

  console.log(`[QQ] 启动 NapCat 反向 WS 服务端: ${napcat.listen}`);

  const server = await startOneBotServer(napcat);

  // 事件分发：传输层回调 → 消息管线
  server.setHandlers({
    onMessage: (event) => handleMessage(ctx, server.client, event),
    onMetaEvent: (event) => handleMetaEvent(ctx, server.client, event),
  });

  console.log(`[QQ] WS 服务端已启动: ws://${napcat.listen}`);

  return {
    stop: () => server.stop(),
  };
}
