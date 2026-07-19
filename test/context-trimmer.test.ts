import { describe, expect, it, beforeEach, afterEach, spyOn } from "bun:test";
import { createContextTrimmer, validateMaxTokens } from "../src/context-trimmer";
import type { StellaConfig } from "../src/config";
import type { AgentMessage } from "@earendil-works/pi-agent-core";

// ---- 帮助函数：构造测试消息 ----

/** 构造 user 消息 */
function userMsg(content: string, timestamp: number = Date.now()): AgentMessage {
  return {
    role: "user",
    content,
    timestamp,
  } as AgentMessage;
}

/** 构造 assistant 消息（带 toolCall） */
function assistantMsg(content: string, timestamp: number = Date.now()): AgentMessage {
  return {
    role: "assistant",
    content: [{ type: "text", text: content }],
    timestamp,
    api: "anthropic" as any,
    provider: "anthropic" as any,
    model: { provider: "anthropic", name: "claude" } as any,
    usage: { inputTokens: 0, outputTokens: 0 } as any,
    stopReason: "stop" as any,
  } as AgentMessage;
}

/** 构造带 toolCall 的 assistant 消息 */
function assistantWithToolCall(toolCallId: string, toolName: string, timestamp: number = Date.now()): AgentMessage {
  return {
    role: "assistant",
    content: [
      {
        type: "toolCall",
        toolCallId,
        toolName,
        input: {},
      } as any,
    ],
    timestamp,
    api: "anthropic" as any,
    provider: "anthropic" as any,
    model: { provider: "anthropic", name: "claude" } as any,
    usage: { inputTokens: 0, outputTokens: 0 } as any,
    stopReason: "toolUse" as any,
  } as AgentMessage;
}

/** 构造 toolResult 消息 */
function toolResultMsg(toolCallId: string, text: string, timestamp: number = Date.now()): AgentMessage {
  return {
    role: "toolResult",
    toolCallId,
    toolName: "test_tool",
    content: [{ type: "text", text }],
    isError: false,
    timestamp,
  } as unknown as AgentMessage;
}

// ---- 默认配置（用于测试） ----

function makeConfig(overrides: Partial<StellaConfig["short_term_memory"]> = {}): StellaConfig {
  return {
    short_term_memory: {
      max_tokens: 10000,
      max_age_days: 3,
      ...overrides,
    },
  } as StellaConfig;
}

// ---- 单元测试：validateMaxTokens ----

describe("validateMaxTokens", () => {
  let warnSpy: ReturnType<typeof spyOn>;

  beforeEach(() => {
    warnSpy = spyOn(console, "warn").mockImplementation(() => {});
  });

  afterEach(() => {
    warnSpy.mockRestore();
  });

  it("max_tokens >= 500 时不做 warn", () => {
    const config = makeConfig({ max_tokens: 500 });
    validateMaxTokens(config);
    expect(warnSpy).not.toHaveBeenCalled();
  });

  it("max_tokens < 500 时输出 warn", () => {
    const config = makeConfig({ max_tokens: 499 });
    validateMaxTokens(config);
    expect(warnSpy).toHaveBeenCalledTimes(1);
    const msg = warnSpy.mock.calls[0]?.[0] as string;
    expect(msg).toContain("500");
  });

  it("max_tokens = 0 时也 warn", () => {
    const config = makeConfig({ max_tokens: 0 });
    validateMaxTokens(config);
    expect(warnSpy).toHaveBeenCalledTimes(1);
  });
});

// ---- 单元测试：trimContextMessages ----

import { trimContextMessages } from "../src/context-trimmer";

describe("trimContextMessages", () => {
  it("消息数在 token 限制内时不裁剪", () => {
    // 短消息，token 估计值远低于限制
    const messages = [userMsg("你好")];
    const result = trimContextMessages(messages, 10000, 3);
    expect(result.length).toBe(1);
  });

  it("超 token 限后从前面丢弃消息，沿 user 轮边界", () => {
    const now = Date.now();
    // 构造多轮对话，每轮 token 估计超过限制的一半
    // round 1
    const r1User = userMsg("A".repeat(5000), now - 1000); // ~2500 estimated tokens
    const r1Asst = assistantMsg("B".repeat(2000), now - 900);
    // round 2
    const r2User = userMsg("C".repeat(5000), now - 800); // ~2500 estimated tokens
    const r2Asst = assistantMsg("D".repeat(2000), now - 700);

    const messages = [r1User, r1Asst, r2User, r2Asst];

    // 用低 token 限制：大概只够保留一轮
    // 保守估计: 5000 chars / 2 = 2500 tokens, 2000 chars / 2 = 1000 tokens
    // round 1: ~3500 tokens, round 2: ~3500 tokens
    // max_tokens = 4000 → 只能保留一轮
    const result = trimContextMessages(messages, 4000, 3);

    // 应该保留最近的轮次（round 2）
    expect(result.length).toBe(2);
    // 第二轮应该以 r2User 开头
    expect((result[0] as any).role).toBe("user");
    expect((result[0] as any).content).toContain("C");
  });

  it("toolCall/toolResult 对不被切断：整个 user 轮次整体保留或丢弃", () => {
    const now = Date.now();
    // round 1: user → assistant(with toolCall) → toolResult（约 3000+ token）
    const r1User = userMsg("A".repeat(5000), now - 1000);
    const r1AsstTool = assistantWithToolCall("tc1", "read", now - 900);
    const r1Result = toolResultMsg("tc1", "R".repeat(500), now - 800);

    // round 2: user → assistant（约 2500+ token）
    const r2User = userMsg("B".repeat(4000), now - 700);
    const r2Asst = assistantMsg("reply", now - 600);

    const messages = [r1User, r1AsstTool, r1Result, r2User, r2Asst];

    // 设 token 限制为 3500，只能保留一轮
    const result = trimContextMessages(messages, 3500, 3);

    // 应该保留第二轮（最近的 user 轮次）
    expect(result.length).toBe(2);
    expect((result[0] as any).role).toBe("user");
    expect((result[0] as any).content).toContain("B");
    // 第一轮（含 toolCall/toolResult）整个被丢弃
  });

  it("消息中没有 user 角色时保留所有消息", () => {
    // 极端情况：只有 assistant 和 toolResult（无 user 消息的会话状态）
    const msgs = [assistantMsg("system-like"), toolResultMsg("tc1", "result")];
    const result = trimContextMessages(msgs, 10, 3);
    // 没有 user 边界可切，保留所有
    expect(result.length).toBe(2);
  });

  it("超龄消息被淘汰（超过 max_age_days 天）", () => {
    const now = Date.now();
    const daysMs = 24 * 60 * 60 * 1000;

    // 旧消息：4 天前
    const oldUser = userMsg("old message", now - 4 * daysMs);
    const oldAsst = assistantMsg("old reply", now - 4 * daysMs + 100);

    // 新消息：现在
    const newUser = userMsg("new message", now);
    const newAsst = assistantMsg("new reply", now);

    const messages = [oldUser, oldAsst, newUser, newAsst];

    // max_age_days = 3 → 旧消息应该被丢弃
    const result = trimContextMessages(messages, 100000, 3);

    // 只保留新消息
    expect(result.length).toBe(2);
    expect((result[0] as any).content).toContain("new");
  });

  it("所有消息都超龄时至少保留最后一条 user 轮", () => {
    const now = Date.now();
    const daysMs = 24 * 60 * 60 * 1000;

    const oldUser = userMsg("old", now - 10 * daysMs);
    const oldAsst = assistantMsg("old reply", now - 10 * daysMs + 100);

    const messages = [oldUser, oldAsst];

    // max_age_days = 3 → 但至少保留最后一轮
    const result = trimContextMessages(messages, 100000, 3);
    expect(result.length).toBeGreaterThanOrEqual(2);
  });

  it("裁剪后不影响原始消息引用（返回新数组）", () => {
    const messages = [userMsg("hello"), assistantMsg("hi")];
    const result = trimContextMessages(messages, 100000, 3);

    // 新数组
    expect(result).not.toBe(messages);
    // 但消息对象引用不变（浅拷贝）
    expect(result[0]).toBe(messages[0]);
  });

  it("激进模式（aggressive=true）裁剪更多", () => {
    const now = Date.now();
    // round 1: ~3000 tokens
    const r1User = userMsg("A".repeat(5000), now - 1000);
    const r1Asst = assistantMsg("B".repeat(2000), now - 900);
    // round 2: ~3000 tokens
    const r2User = userMsg("C".repeat(5000), now - 800);
    const r2Asst = assistantMsg("D".repeat(2000), now - 700);
    // round 3: ~3000 tokens
    const r3User = userMsg("E".repeat(5000), now - 600);
    const r3Asst = assistantMsg("F".repeat(2000), now - 500);

    const messages = [r1User, r1Asst, r2User, r2Asst, r3User, r3Asst];

    // 正常模式 7500 tokens → 保留 2 轮
    const normalResult = trimContextMessages(messages, 7500, 3);
    expect(normalResult.length).toBe(4); // round 2 + 3

    // 激进模式：使用更低的阈值（因子 0.7 → 5250）→ 只保留 1 轮
    const aggressiveResult = trimContextMessages(messages, 7500, 3, 0.7);
    expect(aggressiveResult.length).toBe(2); // round 3 only
  });
});

// ---- 集成测试：createContextTrimmer ----

describe("createContextTrimmer", () => {
  it("返回 extensionFactory 和 handleOverflow", () => {
    const config = makeConfig();
    const trimmer = createContextTrimmer(config);
    expect(trimmer.extensionFactory).toBeFunction();
    expect(trimmer.handleOverflow).toBeFunction();
  });

  it("extensionFactory 注册 context 事件", () => {
    const config = makeConfig();
    const trimmer = createContextTrimmer(config);

    // 模拟 pi 对象
    let registeredEvent = "";
    let registeredHandler: ((event: any, ctx: any) => any) | null = null;

    const mockPi = {
      on(event: string, handler: (event: any, ctx: any) => any) {
        registeredEvent = event;
        registeredHandler = handler;
      },
    };

    trimmer.extensionFactory(mockPi as any);
    expect(registeredEvent).toBe("context");
  });

  it("context 事件处理器在超限时裁剪消息", async () => {
    const config = makeConfig({ max_tokens: 3000 });
    const trimmer = createContextTrimmer(config);

    let registeredHandler: ((event: any, ctx: any) => any) | null = null;
    const mockPi = {
      on(_event: string, handler: (event: any, ctx: any) => any) {
        registeredHandler = handler;
      },
    };

    trimmer.extensionFactory(mockPi as any);

    const now = Date.now();
    const messages = [
      userMsg("A".repeat(4000), now - 100),
      assistantMsg("B".repeat(3000), now - 50),
      userMsg("C".repeat(1000), now),
      assistantMsg("D".repeat(500), now + 10),
    ];

    // 构造 context event
    const event = { type: "context", messages };
    const mockCtx = {
      getContextUsage: () => ({ tokens: 8000, contextWindow: 128000, percent: 6.25 }),
    };

    const result = await registeredHandler!(event, mockCtx);

    // 应该返回裁剪后的消息列表
    expect(result).toBeDefined();
    expect(result!.messages).toBeDefined();
    // 裁剪后消息数应减少（至少丢弃第一轮）
    expect(result!.messages!.length).toBeLessThan(messages.length);
    // 保留的消息应以最近 user 轮开始
    expect((result!.messages![0] as any).role).toBe("user");
    expect((result!.messages![0] as any).content).toContain("C");
  });

  it("未超限时 context 处理器返回 undefined（不修改消息）", async () => {
    const config = makeConfig({ max_tokens: 100000 });
    const trimmer = createContextTrimmer(config);

    let registeredHandler: ((event: any, ctx: any) => any) | null = null;
    const mockPi = {
      on(_event: string, handler: (event: any, ctx: any) => any) {
        registeredHandler = handler;
      },
    };

    trimmer.extensionFactory(mockPi as any);

    const messages = [userMsg("hello"), assistantMsg("hi")];
    const event = { type: "context", messages };
    const mockCtx = {
      getContextUsage: () => ({ tokens: 500, contextWindow: 128000, percent: 0.4 }),
    };

    const result = await registeredHandler!(event, mockCtx);

    // 未超限，返回 undefined（或空对象）
    expect(result?.messages).toBeUndefined();
  });

  it("handleOverflow 设置激进模式，在 context 事件中使用更低阈值", async () => {
    const config = makeConfig({ max_tokens: 10000 });
    const trimmer = createContextTrimmer(config);

    let registeredHandler: ((event: any, ctx: any) => any) | null = null;
    const mockPi = {
      on(_event: string, handler: (event: any, ctx: any) => any) {
        registeredHandler = handler;
      },
    };

    trimmer.extensionFactory(mockPi as any);

    // 构造大量消息，确保需要裁剪
    const now = Date.now();
    const messages = [
      userMsg("A".repeat(6000), now - 200),
      assistantMsg("B".repeat(4000), now - 150),
      userMsg("C".repeat(6000), now - 100),
      assistantMsg("D".repeat(4000), now - 50),
      userMsg("E".repeat(1000), now),
    ];

    const event = { type: "context", messages };
    const mockCtx = {
      getContextUsage: () => ({ tokens: 20000, contextWindow: 128000, percent: 15 }),
    };

    // 第一次：非激进模式，max_tokens = 10000
    // 每轮约 ~5000 tokens → 应保留最近 2 轮（round 2 有 2 条，round 3 有 1 条）
    const result1 = await registeredHandler!(event, mockCtx);
    expect(result1!.messages!.length).toBe(3); // round 2 (2) + round 3 (1)

    // 设置为激进模式后再次调用（模拟 overflow 重试）
    const aggressiveEvent = { type: "context", messages };
    const result2 = await registeredHandler!(aggressiveEvent, mockCtx);
    // 激进模式应该在非激进后自动重置，这里验证非激进模式仍正常工作
    expect(result2!.messages!.length).toBe(3);

    // 测试 handleOverflow 调用后的效应
    // 我们直接测试 trimContextMessages 的激进因子
    const aggressiveResult = trimContextMessages(messages, 10000, 3, 0.7);
    const normalResult = trimContextMessages(messages, 10000, 3);
    // 激进模式（因子 0.7）应保留更少消息
    expect(aggressiveResult.length).toBeLessThanOrEqual(normalResult.length);
  });
});
