import { describe, expect, it } from "bun:test";
import type { AgentSession } from "@earendil-works/pi-coding-agent";
import { createConversation } from "../../../src/adapters/qq/conversation";
import type { ConversationDeps } from "../../../src/adapters/qq/conversation";
import type { OneBotClient } from "../../../src/adapters/qq/onebot";
import type { ApiResponse, OneBotMessageEvent, Segment } from "../../../src/adapters/qq/types";

/**
 * 回合驱动测试：以假会话（FakeSession）+ 假传输客户端（FakeClient）从
 * onMessage/onMetaEvent 接口之外驱动，覆盖触发回合、倾听入库与断连补偿。
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

  sendCustomMessage(text: string): void {
    this.customMessages.push(text);
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
    sessionStore: { get: () => null },
    ...overrides,
  };
  return Object.assign(deps, { sessionMap });
}

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

const META_CONNECT = {
  post_type: "meta_event",
  meta_event_type: "lifecycle",
  sub_type: "connect",
  self_id: 10001,
  time: 0,
} as const;

// ---- 触发回合 ----

describe("触发回合", () => {
  it("群聊 @bot → prompt → 收集回复 → 解析出站 → 发送", async () => {
    const { client, groupSends } = makeClient();
    const userSpy: { sid: string | null } = { sid: null };
    const conv = createConversation(
      makeDeps({ setSessionUser: (sid) => { userSpy.sid = sid; } }),
      client,
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

  it("回复为空时不发送", async () => {
    const { client, groupSends } = makeClient();
    const fake = new FakeSession();
    fake.prompt = async () => {
      fake.emit({ type: "agent_end" }); // 无 delta
    };
    const conv = createConversation(
      makeDeps({ sessions: { getOrCreate: async () => fake as unknown as AgentSession } }),
      client,
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
    const conv = createConversation(deps, client);

    await conv.onMessage(groupPassive(456, 222, "大家好啊"));

    expect(groupSends.length).toBe(0);
    const session = deps.sessionMap.get("qq:group:456");
    expect(session!.customMessages.length).toBe(1);
    expect(session!.customMessages[0]).toContain("大家好啊");
    expect(session!.customMessages[0]).toContain("(222)");
  });
});

// ---- 断连补偿 ----

describe("断连补偿", () => {
  const GROUP_HISTORY: ApiResponse = {
    status: "ok",
    retcode: 0,
    data: {
      messages: [
        // 机器人自己 → 跳过
        { post_type: "message", time: 90, user_id: 10001, message: [{ type: "text", data: { text: "自己的消息" } }] },
        // cutoff（last_active=100）之前 → 跳过
        { post_type: "message", time: 99, user_id: 222, message: [{ type: "text", data: { text: "空洞前的旧消息" } }] },
        // 正常 → 入库
        { post_type: "message", time: 120, user_id: 222, message: [{ type: "text", data: { text: "空洞期新消息" } }] },
        // 非 message 类型 → 跳过
        { post_type: "notice", time: 130, user_id: 222, message: [] },
      ],
    },
  };

  it("群聊：拉历史 → 过滤（自己/cutoff/类型）→ 格式化入库", async () => {
    const { client, calls } = makeClient((action) => {
      expect(action).toBe("get_group_msg_history");
      return GROUP_HISTORY;
    });
    const deps = makeDeps({
      sessionStore: {
        get: () => ({ platform: "qq", chat_key: "qq:group:456", session_path: "", created_at: 0, last_active: 100 }),
      },
    });
    const conv = createConversation(deps, client);

    // 先有活跃消息登记 knownChats，再触发 connect 补偿
    await conv.onMessage(groupPassive(456, 222, "活跃消息"));
    await conv.onMetaEvent(META_CONNECT);

    expect(calls[0]!.params).toEqual({ group_id: 456, count: 20 });

    const session = deps.sessionMap.get("qq:group:456");
    // 活跃消息 + 空洞期新消息，共 2 条
    expect(session!.customMessages.length).toBe(2);
    expect(session!.customMessages[1]).toContain("空洞期新消息");
    expect(session!.customMessages[1]).toContain("(222)");
  });

  it("私聊：拉历史 → 纯文本入库", async () => {
    const { client, calls } = makeClient((action) => {
      expect(action).toBe("get_friend_msg_history");
      return {
        status: "ok",
        retcode: 0,
        data: {
          messages: [
            { post_type: "message", time: 120, user_id: 999, message: [{ type: "text", data: { text: "私聊空洞" } }] },
          ],
        },
      };
    });
    const deps = makeDeps();
    const conv = createConversation(deps, client);

    await conv.onMessage({
      post_type: "message", message_type: "private", message_id: 1, user_id: 999,
      self_id: 10001, time: 100, sender: { nickname: "小红" },
      message: [{ type: "text", data: { text: "嗨" } }],
    });
    await conv.onMetaEvent(META_CONNECT);

    expect(calls[0]!.params).toEqual({ user_id: 999, count: 20 });
    const session = deps.sessionMap.get("qq:private:999");
    // 私聊消息总是触发（不入库），这里只有补偿回填的 1 条
    expect(session!.customMessages.length).toBe(1);
    expect(session!.customMessages[0]).toBe("私聊空洞"); // 无 [#id 说话人] 前缀
  });

  it("retcode 非 0 时不入库", async () => {
    const { client } = makeClient(() => ({ status: "failed", retcode: 100, data: {} }));
    const deps = makeDeps();
    const conv = createConversation(deps, client);

    await conv.onMessage(groupPassive(456, 222, "活跃"));
    await conv.onMetaEvent(META_CONNECT);

    const session = deps.sessionMap.get("qq:group:456");
    expect(session!.customMessages.length).toBe(1); // 只有活跃消息
  });
});
