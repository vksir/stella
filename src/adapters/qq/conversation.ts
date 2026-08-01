/**
 * QQ 会话回合驱动（业务层）。
 *
 * 职责：
 * - 触发（Trigger）：消息 → 身份解析 → 会话 → 触发注记 → prompt → 回复收集 → 出站解析 → 发送
 * - 倾听（Listening）：未触发消息格式化入库
 * - 触发注记注入（before_agent_start 扩展，当轮有效）
 *
 * 传输/协议在 onebot.ts；pi SDK（AgentSession、扩展机制）只出现在本模块实现中。
 * 业务编排依赖 OneBotApiClient 具体类（生产唯一实现，不设接口）。
 *
 * 平台无关件（ReplyCollector / TriggerNoteBus）暂居本模块，第二平台落地时上提共享。
 */

import type {
  ExtensionFactory,
  ExtensionAPI,
  BeforeAgentStartEvent,
  BeforeAgentStartEventResult,
  AgentSession,
  AgentSessionEvent,
} from "@earendil-works/pi-coding-agent";
import type { OneBotMessageEvent } from "./types";
import { SELF_NAME } from "./types";
import { segmentsToText, formatGroupMessage, parseOutbound } from "./format";
import { isTrigger, triggerNoteText } from "./trigger";
import type { OneBotClient } from "./onebot";
import type { IdentityResolver } from "../../identity";
import type { SessionRegistry } from "../../sessions-registry";
import type { UserRow } from "../../stores/user";

// ---- 接口 ----

/**
 * 回合驱动所需的应用依赖（窄接口：仅业务路径实际用到的部分）。
 * AppContext 结构上满足该接口，qq.ts 无需改动装配方式。
 */
export interface ConversationDeps {
  identity: Pick<IdentityResolver, "resolve">;
  sessions: Pick<SessionRegistry, "getOrCreate">;
  setSessionUser: (sessionId: string, user: UserRow) => void;
}

// ---- 触发注记（before_agent_start 扩展） ----

/**
 * 触发注记中转：会话驱动写入、扩展读取（读取即删，当轮有效）。
 * 实例由组合根（index.ts）创建并注入两处，替代模块级状态。
 */
export class TriggerNoteBus {
  private notes = new Map<string, string>();

  /** 写入当轮触发注记（按 sessionId 索引）。 */
  set(sessionId: string, note: string): void {
    this.notes.set(sessionId, note);
  }

  /** 取出注记（读取即删，当轮有效）。 */
  take(sessionId: string): string | undefined {
    const note = this.notes.get(sessionId);
    if (note !== undefined) this.notes.delete(sessionId);
    return note;
  }
}

/**
 * 创建触发注记扩展工厂。
 * 在 before_agent_start 事件中注入触发注记（当轮有效，不入库）。
 */
export function createTriggerNoteExtension(bus: TriggerNoteBus): ExtensionFactory {
  return (pi: ExtensionAPI) => {
    pi.on("before_agent_start", (event: BeforeAgentStartEvent, ctx) => {
      const sessionId = ctx.sessionManager.getSessionId();
      const note = bus.take(sessionId);
      if (!note) return;

      // 将触发注记追加到系统提示词末尾（当轮有效）
      const result: BeforeAgentStartEventResult = {
        systemPrompt: event.systemPrompt + "\n\n" + note,
      };
      return result;
    });
  };
}

// ---- 回复收集 ----

/**
 * 订阅会话事件流，累积 text_delta，agent_end 时交付完整回复文本。
 * 状态（累积文本、退订句柄）归本对象，dispose 显式退订防泄漏。
 */
export class ReplyCollector {
  private fullText = "";
  private unsub: (() => void) | null = null;

  constructor(private session: AgentSession) {}

  /** 开始收集；返回在 agent_end 时 resolve 完整文本的 Promise。 */
  collect(): Promise<string> {
    return new Promise<string>((resolve) => {
      this.unsub = this.session.subscribe((event: AgentSessionEvent) => {
        if (event.type === "message_update" && event.assistantMessageEvent?.type === "text_delta") {
          this.fullText += event.assistantMessageEvent.delta;
        }
        if (event.type === "agent_end") {
          this.dispose();
          resolve(this.fullText);
        }
      });
    });
  }

  /** 退订事件流。 */
  dispose(): void {
    this.unsub?.();
    this.unsub = null;
  }
}

// ---- 会话驱动 ----

/**
 * 会话回合驱动：把 OneBot 事件编排为 Stella 的一轮对话。
 * 群聊/私聊差异（chatKey、触发判断、注记、格式化、发送目标）留在分支内，
 * 回合骨架（getSession / runTurn / ingest）收敛为本类私有方法。
 */
export class QQConversation {
  constructor(
    private deps: ConversationDeps,
    private client: OneBotClient,
    private noteBus: TriggerNoteBus,
  ) {}

  /**
   * 处理单条 OneBot message 事件。
   */
  async onMessage(data: OneBotMessageEvent): Promise<void> {
    const selfId = String(data.self_id);
    const { message_type } = data;

    if (message_type === "group") {
      const groupId = data.group_id!;
      const chatKey = `qq:group:${groupId}`;
      const session = await this.getSession(chatKey, data.user_id);

      if (isTrigger("group", data.message, selfId)) {
        // ---- 群聊触发 ----
        this.noteBus.set(
          session.sessionId,
          triggerNoteText("group", data.sender, data.message_id, data.user_id),
        );
        const formatted = formatGroupMessage(
          data.message, data.sender, data.message_id, data.time, data.user_id,
        );
        await this.runTurn(session, formatted, "group", groupId, data.user_id);
      } else {
        // ---- 群聊被动消息（仅入库） ----
        await this.ingest(
          session,
          formatGroupMessage(data.message, data.sender, data.message_id, data.time, data.user_id),
        );
      }
    } else {
      // ---- 私聊（总是触发） ----
      const chatKey = `qq:private:${data.user_id}`;
      const session = await this.getSession(chatKey, data.user_id);

      this.noteBus.set(
        session.sessionId,
        triggerNoteText("private", data.sender, data.message_id, data.user_id),
      );

      // 私聊：纯文本（不标注说话人）
      const text = segmentsToText(data.message, SELF_NAME);
      await this.runTurn(session, text, "private", undefined, data.user_id);
    }
  }

  // ---- 私有：会话 ----

  /**
   * 身份解析 → 取会话 → 绑定当前说话人（三步收敛为一处）。
   */
  private async getSession(chatKey: string, userId: number): Promise<AgentSession> {
    const user = this.deps.identity.resolve("qq", String(userId));
    const session = await this.deps.sessions.getOrCreate("qq", chatKey);
    this.deps.setSessionUser(session.sessionId, user);
    return session;
  }

  // ---- 私有：回合 ----

  /**
   * 回合骨架：先挂起回复收集器 → 触发模型 → 收集完整回复 → 解析出站 → 发送。
   */
  private async runTurn(
    session: AgentSession,
    promptText: string,
    chatType: "group" | "private",
    groupId: number | undefined,
    senderUserId: number,
  ): Promise<void> {
    // 先订阅再触发：text_delta 在 prompt 返回前就开始发射
    const replyPromise = new ReplyCollector(session).collect();

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
      await this.client.sendGroupMessage(groupId, segments);
    } else {
      await this.client.sendPrivateMessage(senderUserId, segments);
    }
  }

  // ---- 私有：倾听（被动入库） ----

  /**
   * 入库一条消息文本（sendCustomMessage 的唯一出口）。
   */
  private async ingest(session: AgentSession, text: string): Promise<void> {
    try {
      await session.sendCustomMessage(
        { customType: "qq.chat_message", content: text, display: true },
        { deliverAs: "nextTurn" },
      );
    } catch (err) {
      console.error("[QQ] 消息入库失败:", err);
    }
  }
}
