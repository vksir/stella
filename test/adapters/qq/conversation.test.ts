import { describe, expect, it } from "bun:test";
import type { AgentSession } from "@earendil-works/pi-coding-agent";
import { QQConversation, TriggerNoteBus } from "../../../src/adapters/qq/conversation";
import type { ConversationDeps } from "../../../src/adapters/qq/conversation";
import type { OneBotClient } from "../../../src/adapters/qq/onebot";
import type { ApiResponse, OneBotMessageEvent, Segment } from "../../../src/adapters/qq/types";

/**
 * 回合驱动测试：以假会话（FakeSession）+ 假传输客户端（FakeClient extends OneBotApiClient）
 * 从 onMessage 接口之外驱动，覆盖触发回合与倾听入库。
 */

// ---- 假对象 ----

class FakeSession {
  static n = 0;
  sessionId = `fake-${++FakeSession.n}`;
  customMessages: string[] = [];
  promptText = "";
  private handler: ((e: any) => void) | null = null;

  async prompt(text: string): Promise<void> {
    this.promptText = text;
    // 模拟模型输出：两段 delta + agent_end
    this.emit({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "你" } });
    this.emit({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "好" } });
    this.emit({ type: "agent_end" });
  }

  subscribe(fn: (e: any) => void): () => void {
    this.handler = fn;
    return () => { this.handler = null; };
  }

  sendCustomMessage(message: { content?: unknown }): void {
    this.customMessages.push(String(message.content ?? ""));
  }

  /** 测试驱动：主动发射事件（模拟模型输出/回合结束） */
  emit(e: any): void {
    this.handler?.(e);
  }
}

const FAKE_USER = { id: 1, display_name: "小明", role: "guest" as const, created_at: 0 };

function makeDeps(
  overrides: Partial<ConversationDeps> = {},
): ConversationDeps & { sessionMap: Map<string, FakeSession> } {
  const sessionMap = new Map<string, FakeSession>();
  const deps: ConversationDeps = {
    identity: { resolve: () => FAKE_USER },
    sessions: {
      getOrCreate: async (_platform: string, chatKey: string) => {
        let s = sessionMap.get(chatKey);
        if (!s) {
          s = new FakeSession();
          sessionMap.set(chatKey, s);
        }
        return s as unknown as AgentSession;
      },
    },
    setSessionUser: () => {},
    ...overrides,
  };
  return Object.assign(deps, { sessionMap });
}

/** 假传输客户端：实现 OneBotClient 接口（记录调用）。 */
function makeClient(history?: (action: string) => ApiResponse) {
  const calls: Array<{ action: string; params: Record<string, unknown> }> = [];
  const groupSends: Array<{ groupId: number; segments: Segment[] }> = [];
  const privateSends: Array<{ userId: number; segments: Segment[] }> = [];

  const client: OneBotClient = {
    selfId: "10001",
    async send(action, params) {
      calls.push({ action, params });
      if (history) return history(action);
      return { status: "ok", retcode: 0, data: {} };
    },
    async sendGroupMessage(groupId, segments) {
      groupSends.push({ groupId, segments });
    },
    async sendPrivateMessage(userId, segments) {
      privateSends.push({ userId, segments });
    },
  };

  return { client, calls, groupSends, privateSends };
}

/** 群聊被动消息（无 @bot）。 */
function groupPassive(groupId: number, userId: number, text: string): OneBotMessageEvent {
  return {
    post_type: "message",
    message_type: "group",
    message_id: 1,
    user_id: userId,
    group_id: groupId,
    self_id: 10001,
    time: 100,
    sender: { nickname: "小明" },
    message: [{ type: "text", data: { text } }],
  };
}

/** 群聊触发消息（@bot）。 */
function groupTrigger(groupId: number, userId: number, text: string): OneBotMessageEvent {
  return {
    post_type: "message",
    message_type: "group",
    message_id: 2,
    user_id: userId,
    group_id: groupId,
    self_id: 10001,
    time: 101,
    sender: { nickname: "小明" },
    message: [
      { type: "at", data: { qq: "10001" } },
      { type: "text", data: { text } },
    ],
  };
}

/** 私聊消息（总是触发）。 */
function privateMessage(userId: number, text: string): OneBotMessageEvent {
  return {
    post_type: "message",
    message_type: "private",
    message_id: 1,
    user_id: userId,
    self_id: 10001,
    time: 100,
    sender: { nickname: "小红" },
    message: [{ type: "text", data: { text } }],
  };
}

// ---- 触发回合 ----

describe("触发回合", () => {
  it("群聊 @bot → prompt → 收集回复 → 解析出站 → 发送", async () => {
    const { client, groupSends } = makeClient();
    const userSpy: { sid: string | null } = { sid: null };
    const conv = new QQConversation(
      makeDeps({ setSessionUser: (sid) => { userSpy.sid = sid; } }),
      client,
      new TriggerNoteBus(),
    );

    await conv.onMessage(groupTrigger(456, 222, "在吗"));

    expect(userSpy.sid).toBe("fake-1");
    expect(groupSends).toEqual([
      {
        groupId: 456,
        segments: [
          { type: "at", data: { qq: "222" } },
          { type: "text", data: { text: "你好" } },
        ],
      },
    ]);
  });

  it("私聊总是触发 → prompt → 发送私聊", async () => {
    const { client, privateSends } = makeClient();
    const conv = new QQConversation(makeDeps(), client, new TriggerNoteBus());

    await conv.onMessage(privateMessage(999, "嗨"));

    expect(privateSends).toEqual([
      { userId: 999, segments: [{ type: "text", data: { text: "你好" } }] },
    ]);
  });

  it("回复为空时不发送", async () => {
    const { client, groupSends } = makeClient();
    const fake = new FakeSession();
    fake.prompt = async () => {
      fake.emit({ type: "agent_end" }); // 无 delta
    };
    const conv = new QQConversation(
      makeDeps({ sessions: { getOrCreate: async () => fake as unknown as AgentSession } }),
      client,
      new TriggerNoteBus(),
    );

    await conv.onMessage(groupTrigger(456, 222, "在吗"));

    expect(groupSends.length).toBe(0);
  });
});

// ---- 倾听（被动入库） ----

describe("倾听（被动入库）", () => {
  it("群聊无 @ 的消息仅入库，不回复", async () => {
    const { client, groupSends } = makeClient();
    const deps = makeDeps();
    const conv = new QQConversation(deps, client, new TriggerNoteBus());

    await conv.onMessage(groupPassive(456, 222, "大家好啊"));

    expect(groupSends.length).toBe(0);
    const session = deps.sessionMap.get("qq:group:456");
    expect(session!.customMessages.length).toBe(1);
    expect(session!.customMessages[0]).toContain("大家好啊");
    expect(session!.customMessages[0]).toContain("(222)");
  });
});
