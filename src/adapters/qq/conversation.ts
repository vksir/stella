/**
 * QQ 会话回合驱动（业务层）。
 *
 * 职责：
 * - 触发（Trigger）：消息 → 身份解析 → 会话 → 触发注记 → prompt → 回复收集 → 出站解析 → 发送
 * - 倾听（Listening）：未触发消息格式化入库；断连补偿回填消息空洞
 * - 触发注记注入（before_agent_start 扩展，当轮有效）
 *
 * 传输/协议在 onebot.ts；pi SDK（AgentSession、扩展机制）只出现在本模块实现中。
 * 业务编排经 OneBotClient interface 与传输层交互，不接触 Bun 类型。
 */

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
import type { OneBotClient } from "./onebot";
import type { IdentityResolver } from "../../identity";
import type { SessionRegistry } from "../../sessions-registry";
import type { SessionStore } from "../../stores/session";
import type { UserRow } from "../../stores/user";

// ---- 接口 ----

export interface Conversation {
  onMessage(event: OneBotMessageEvent): Promise<void>;
  onMetaEvent(event: OneBotMetaEvent): Promise<void>;
}

/**
 * 回合驱动所需的应用依赖（窄接口：仅业务路径实际用到的部分）。
 * AppContext 结构上满足该接口，qq.ts 无需改动装配方式。
 */
export interface ConversationDeps {
  identity: Pick<IdentityResolver, "resolve">;
  sessions: Pick<SessionRegistry, "getOrCreate">;
  setSessionUser: (sessionId: string, user: UserRow) => void;
  sessionStore: Pick<SessionStore, "get">;
}

const CATCHUP_COUNT = 20;

// ---- 触发注记（before_agent_start 扩展） ----

/**
 * 待注入的触发注记（按 sessionId 索引）。
 * 模块级：扩展在 bootstrap 时注册，早于会话创建；进程内仅一个会话驱动。
 */
const pendingTriggerNotes = new Map<string, string>();

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

// ---- 会话驱动 ----

/**
 * 创建会话回合驱动：把 OneBot 事件编排为 Stella 的一轮对话。
 */
export function createConversation(deps: ConversationDeps, client: OneBotClient): Conversation {
  /**
   * 已知活跃会话的 chatKey 集合，用于断连补偿。
   * 归本模块持有（而非连接层）：WS 重连后依然存活，补偿才能真正回填消息空洞。
   */
  const knownChats = new Set<string>();

  // ---- 消息处理 ----

  /**
   * 处理单条 OneBot message 事件。
   */
  async function handleMessage(data: OneBotMessageEvent): Promise<void> {
    const selfId = String(data.self_id);
    const { message_type } = data;

    const triggered = isTrigger(message_type, data.message, selfId);

    if (message_type === "group") {
      const groupId = data.group_id!;
      const chatKey = `qq:group:${groupId}`;
      knownChats.add(chatKey);

      if (triggered) {
        // ---- 群聊触发 ----
        const user = deps.identity.resolve("qq", String(data.user_id));
        const session = await deps.sessions.getOrCreate("qq", chatKey);
        deps.setSessionUser(session.sessionId, user);

        // 设置触发注记
        const note = triggerNoteText("group", data.sender, data.message_id, data.user_id);
        pendingTriggerNotes.set(session.sessionId, note);

        // 格式化消息
        const formatted = formatGroupMessage(
          data.message, data.sender, data.message_id, data.time, data.user_id,
        );

        await processPromptAndReply(session, formatted, "group", groupId, data.user_id);
      } else {
        // ---- 群聊被动消息（仅入库） ----
        const session = await deps.sessions.getOrCreate("qq", chatKey);
        await injectPassiveMessage(
          session, data.message, data.sender, data.message_id, data.time, data.user_id,
        );
      }
    } else {
      // ---- 私聊 ----
      const chatKey = `qq:private:${data.user_id}`;
      knownChats.add(chatKey);

      const user = deps.identity.resolve("qq", String(data.user_id));
      const session = await deps.sessions.getOrCreate("qq", chatKey);
      deps.setSessionUser(session.sessionId, user);

      // 私聊触发注记
      const note = triggerNoteText("private", data.sender, data.message_id, data.user_id);
      pendingTriggerNotes.set(session.sessionId, note);

      // 私聊：纯文本（不标注说话人）
      const text = segmentsToText(data.message, SELF_NAME);

      await processPromptAndReply(session, text, "private", undefined, data.user_id);
    }
  }

  /**
   * 调用 session.prompt 并收集模型回复 → 解析 → 发送。
   */
  async function processPromptAndReply(
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

  // ---- 倾听（被动入库） ----

  /**
   * 入库一条被动消息文本（sendCustomMessage 的唯一出口）。
   */
  function ingest(session: AgentSession, text: string): void {
    try {
      (session as any).sendCustomMessage(text, { deliverAs: "nextTurn" });
    } catch (err) {
      console.error("[QQ] 消息入库失败:", err);
    }
  }

  /**
   * 群聊被动消息（不触发回复）格式化后写入会话历史。
   */
  function injectPassiveMessage(
    session: AgentSession,
    segs: Segment[],
    sender: SenderInfo,
    messageId: number,
    time: number,
    userId: number,
  ): void {
    ingest(session, formatGroupMessage(segs, sender, messageId, time, userId));
  }

  /**
   * 把历史消息按正序、过滤后逐条入库（断连补偿与实时倾听共用此路径）。
   * 过滤：晚于 cutoff、post_type 为 message、跳过机器人自己。
   */
  async function ingestHistory(
    chatKey: string,
    messages: any[],
    cutoff: number,
    format: (msg: any) => string,
  ): Promise<void> {
    const session = await deps.sessions.getOrCreate("qq", chatKey);

    const sorted = [...messages].sort((a, b) => (a.time ?? 0) - (b.time ?? 0));

    for (const msg of sorted) {
      if ((msg.time ?? 0) <= cutoff) continue;
      if (msg.post_type !== "message") continue;
      if (String(msg.user_id) === client.selfId) continue;

      ingest(session, format(msg));
    }
  }

  /** 历史消息段数组规范化（兼容 message 为字符串的旧格式）。 */
  function normalizeSegments(msg: any): Segment[] {
    return Array.isArray(msg.message)
      ? msg.message
      : [{ type: "text", data: { text: String(msg.message ?? "") } }];
  }

  /** 群聊历史消息 → 入库文本（格式与实时消息一致）。 */
  function formatGroupHistoryMessage(msg: any): string {
    const msgTime = msg.time ?? Math.floor(Date.now() / 1000);
    const sender: SenderInfo = {
      nickname: msg.sender?.nickname ?? "未知",
      card: msg.sender?.card,
    };
    return formatGroupMessage(
      normalizeSegments(msg), sender, msg.message_id ?? 0, msgTime, msg.user_id ?? 0,
    );
  }

  /** 私聊历史消息 → 入库文本（纯文本，不标注说话人）。 */
  function formatPrivateHistoryMessage(msg: any): string {
    return segmentsToText(normalizeSegments(msg), SELF_NAME);
  }

  // ---- 元事件处理 ----

  async function handleMetaEvent(data: OneBotMetaEvent): Promise<void> {
    if (data.meta_event_type === "lifecycle" && data.sub_type === "connect") {
      // 断连补偿：拉取消息空洞
      await catchupMissedMessages();
    }
  }

  /**
   * 断连补偿：对每个已知活跃会话拉取历史消息。
   */
  async function catchupMissedMessages(): Promise<void> {
    for (const chatKey of knownChats) {
      try {
        if (chatKey.startsWith("qq:group:")) {
          const groupId = parseInt(chatKey.slice("qq:group:".length), 10);
          await catchupGroupMessages(groupId, chatKey);
        } else if (chatKey.startsWith("qq:private:")) {
          const userId = parseInt(chatKey.slice("qq:private:".length), 10);
          await catchupPrivateMessages(userId, chatKey);
        }
      } catch (err) {
        console.error(`[QQ] 断连补偿失败 (${chatKey}):`, err);
      }
    }
  }

  /**
   * 拉取群聊历史并回填。群/私聊差异收敛为「API action + 格式化函数」两个参数。
   */
  async function catchupGroupMessages(
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

    const row = deps.sessionStore.get("qq", chatKey);
    await ingestHistory(chatKey, messages, row ? row.last_active : 0, formatGroupHistoryMessage);
  }

  async function catchupPrivateMessages(
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

    const row = deps.sessionStore.get("qq", chatKey);
    await ingestHistory(chatKey, messages, row ? row.last_active : 0, formatPrivateHistoryMessage);
  }

  return {
    onMessage: handleMessage,
    onMetaEvent: handleMetaEvent,
  };
}
