/**
 * 上下文裁剪扩展 — ticket 03
 *
 * 功能：
 * 1. extensionFactory：注册 `context` 事件，每次 LLM 调用前检查 token 量，
 *    超限时沿 user 轮边界裁剪消息。
 * 2. handleOverflow：LLM 溢出时激进裁剪 + 重试。
 */

import type {
  ExtensionFactory,
  ExtensionAPI,
  ContextEvent,
  ExtensionContext,
  AgentSession,
} from "@earendil-works/pi-coding-agent";
import type { StellaConfig } from "./config";

/**
 * 从 ContextEvent 提取消息类型。
 * 避免直接依赖未导出的 AgentMessage / ContextEventResult。
 */
type ChatMessage = ContextEvent["messages"][number];
type ChatMessages = ChatMessage[];

// ---- 消息轮次分组 ----

/**
 * 一个 user 轮：从一条 user 消息开始，后面跟 assistant/toolResult 消息，
 * 直到下一条 user 消息前。
 */
interface UserRound {
  /** 该轮所有消息（含起始的 user 消息） */
  messages: ChatMessages;
  /** 起始 user 消息的索引 */
  startIndex: number;
}

/**
 * 将消息列表按 user 轮边界分组。
 * 第一条 user 消息之前的所有消息（如 system 类消息）归入一个"前导"轮。
 * 返回的每个 UserRound 以 user 消息开头。
 */
export function groupByUserRounds(messages: ChatMessages): UserRound[] {
  const rounds: UserRound[] = [];
  let current: ChatMessages = [];
  let startIndex = 0;

  for (let i = 0; i < messages.length; i++) {
    const msg = messages[i]!;

    if (msg.role === "user" && current.length > 0 && current.some((m) => m.role === "user")) {
      // 遇到新的 user 消息，且当前轮已有一条 user → 结束当前轮
      rounds.push({ messages: current, startIndex });
      current = [];
      startIndex = i;
    }

    current.push(msg);
  }

  // 最后一轮
  if (current.length > 0) {
    rounds.push({ messages: current, startIndex });
  }

  return rounds;
}

// ---- Token 估算 ----

/**
 * 估算单条消息的 token 数。
 * 使用保守字符 / 2 估算（适用于中英混合文本）。
 * 工具角色消息有额外结构开销，toolCall 和 toolResult 各加固定 token。
 */
function estimateMessageTokens(msg: ChatMessage): number {
  const content = extractMessageContent(msg);
  let tokens = content ? Math.ceil(content.length / 2) : 0;

  // 工具消息的结构开销
  if (msg.role === "toolResult") {
    tokens += 50; // tool_result 格式开销
  }
  if (msg.role === "assistant" && hasToolCalls(msg)) {
    tokens += 100; // tool_call 格式开销（含 input schema）
  }

  // 每条消息的消息头开销（role, timestamp, etc.）
  tokens += 20;

  return tokens;
}

/** 检查 assistant 消息是否包含 toolCall。 */
function hasToolCalls(msg: ChatMessage): boolean {
  if (msg.role !== "assistant") return false;
  const c = (msg as any).content;
  if (!Array.isArray(c)) return false;
  return c.some((item: any) => item.type === "toolCall");
}

/**
 * 提取消息的文本内容（用于 token 估算）。
 * 对于 toolResult 消息，提取 content 数组中的 text；
 * 对于 assistant 的 toolCall，序列化 input 对象。
 */
function extractMessageContent(msg: ChatMessage): string {
  if (!("content" in msg)) return "";

  const c = (msg as any).content;
  if (typeof c === "string") return c;

  if (Array.isArray(c)) {
    return c
      .map((item: any) => {
        if (item.type === "text") return item.text as string;
        if (item.type === "thinking") return item.thinking as string;
        // toolCall 的 input 按 JSON 估算
        if (item.type === "toolCall") return JSON.stringify(item.input ?? {});
        return "";
      })
      .join("");
  }

  return "";
}

// ---- 主裁剪逻辑 ----

/**
 * 沿 user 轮边界裁剪消息。
 *
 * @param messages - 原始消息列表
 * @param maxTokens - token 上限
 * @param maxAgeDays - 消息最大保留天数
 * @param thresholdFactor - 阈值因子（默认 1.0，激进模式用 0.7）
 * @returns 裁剪后的新数组
 */
export function trimContextMessages(
  messages: ChatMessages,
  maxTokens: number,
  maxAgeDays: number,
  thresholdFactor: number = 1.0,
): ChatMessages {
  const effectiveMaxTokens = Math.floor(maxTokens * thresholdFactor);

  if (messages.length === 0) return messages;

  const now = Date.now();
  const ageCutoff = now - maxAgeDays * 24 * 60 * 60 * 1000;

  // 1. 按 user 轮分组
  const rounds = groupByUserRounds(messages);

  // 2. 检查是否有 user 轮（否则无法切分，保留全部）
  const userRounds = rounds.filter((r) =>
    r.messages.some((m) => m.role === "user"),
  );

  if (userRounds.length === 0) {
    // 没有 user 消息，保留全部
    return [...messages];
  }

  // 3. 计算每轮的 token 估算和年龄
  interface RoundInfo {
    round: UserRound;
    totalTokens: number;
    oldestTimestamp: number;
    hasUser: boolean;
  }

  const roundInfos: RoundInfo[] = rounds.map((round) => {
    let totalTokens = 0;
    let oldestTimestamp = now;

    for (const msg of round.messages) {
      totalTokens += estimateMessageTokens(msg);
      if ("timestamp" in msg && typeof (msg as any).timestamp === "number") {
        oldestTimestamp = Math.min(oldestTimestamp, (msg as any).timestamp);
      }
    }

    return {
      round,
      totalTokens,
      oldestTimestamp,
      hasUser: round.messages.some((m) => m.role === "user"),
    };
  });

  // 4. 先处理超龄淘汰（从前往后找超龄轮）
  //    标记需要保留的起始索引
  let keepFromRoundIndex = 0;

  for (let i = 0; i < roundInfos.length; i++) {
    const info = roundInfos[i]!;
    if (info.hasUser && info.oldestTimestamp >= ageCutoff) {
      // 这一轮有 user 且在有效期内，从这里开始保留
      keepFromRoundIndex = i;
      break;
    }
    // 如果是最后一个 user 轮，即使超龄也保留
    if (
      info.hasUser &&
      roundInfos.slice(i + 1).every((r) => !r.hasUser || r.oldestTimestamp < ageCutoff)
    ) {
      keepFromRoundIndex = i;
      break;
    }
  }

  // 5. 基于 token 量裁剪（从前往后丢弃）
  //    确保至少保留最后一轮
  const minKeepIndex = Math.min(keepFromRoundIndex, roundInfos.length - 1);

  let totalTokens = 0;
  for (let i = roundInfos.length - 1; i >= 0; i--) {
    totalTokens += roundInfos[i]!.totalTokens;
  }

  let cutIndex = 0;
  for (let i = 0; i < roundInfos.length - 1; i++) {
    if (totalTokens <= effectiveMaxTokens) break;
    const info = roundInfos[i]!;
    // 只丢弃有 user 的轮（或前导轮）
    totalTokens -= info.totalTokens;
    cutIndex = i + 1;
  }

  // 取年龄和 token 裁剪的交集：保留更晚的索引
  const startRoundIndex = Math.max(cutIndex, keepFromRoundIndex);

  // 6. 组装结果
  const result: ChatMessages = [];
  for (let i = startRoundIndex; i < rounds.length; i++) {
    result.push(...rounds[i]!.messages);
  }

  return result;
}

// ---- 上下文裁剪扩展工厂 ----

/**
 * 创建上下文裁剪扩展。
 *
 * @returns extensionFactory（注入 DefaultResourceLoader 的 extensionFactories）
 *         和 handleOverflow（外部调用用于溢出重试）
 */
export function createContextTrimmer(config: StellaConfig): {
  extensionFactory: ExtensionFactory;
  handleOverflow: (error: unknown, session: AgentSession, promptText: string) => Promise<void>;
} {
  // 配置加载时校验
  validateMaxTokens(config);

  const { max_tokens, max_age_days } = config.short_term_memory;

  /**
   * 激进模式标记：handleOverflow 设为 true，
   * context 事件处理器看到后使用更低阈值，然后重置。
   */
  let aggressiveMode = false;

  const extensionFactory: ExtensionFactory = (pi: ExtensionAPI) => {
    pi.on(
      "context",
      (event: ContextEvent, ctx: ExtensionContext): { messages: ChatMessages } | void => {
        const messages = event.messages;

        // 检查 token 用量
        const usage = ctx.getContextUsage();
        const thresholdFactor = aggressiveMode ? 0.7 : 1.0;
        const effectiveMaxTokens = Math.floor(max_tokens * thresholdFactor);

        // 重置激进模式
        aggressiveMode = false;

        // 检查是否需要裁剪：token 超限或存在超龄消息
        const needsAgeTrim = hasExpiredMessages(messages, max_age_days);
        const needsTokenTrim =
          usage && usage.tokens !== null && usage.tokens > effectiveMaxTokens;

        if (!needsAgeTrim && !needsTokenTrim) {
          return; // 无需裁剪
        }

        const trimmed = trimContextMessages(
          messages,
          max_tokens,
          max_age_days,
          thresholdFactor,
        );

        // 如果裁剪后没有变化，不做返回
        if (trimmed.length === messages.length) {
          return;
        }

        return { messages: trimmed };
      },
    );
  };

  const handleOverflow = async (
    _error: unknown,
    session: AgentSession,
    promptText: string,
  ): Promise<void> => {
    aggressiveMode = true;
    try {
      await session.prompt(promptText);
    } catch (retryError: unknown) {
      // 重试后仍失败，抛出原始错误
      throw _error instanceof Error ? _error : new Error(String(_error));
    }
  };

  return { extensionFactory, handleOverflow };
}

// ---- 辅助函数 ----

/**
 * 检查消息列表中是否有超过 max_age_days 的消息。
 */
function hasExpiredMessages(messages: ChatMessages, maxAgeDays: number): boolean {
  if (messages.length === 0) return false;

  const now = Date.now();
  const ageCutoff = now - maxAgeDays * 24 * 60 * 60 * 1000;

  for (const msg of messages) {
    if ("timestamp" in msg && typeof (msg as any).timestamp === "number") {
      const ts = (msg as any).timestamp as number;
      if (ts < ageCutoff) return true;
    }
  }

  return false;
}

/**
 * 校验配置中的 max_tokens，过低时输出警告。
 */
export function validateMaxTokens(config: StellaConfig): void {
  const mt = config.short_term_memory.max_tokens;
  if (mt < 500) {
    console.warn(
      `[Stella] 警告: short_term_memory.max_tokens = ${mt}，低于推荐的最小值 500。` +
        `过低的 token 限制可能导致上下文裁剪过于激进。`,
    );
  }
}

/**
 * 判断错误是否为 token 溢出错误。
 */
export function isOverflowError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  const msg = error.message.toLowerCase();
  const keywords = ["context", "token", "overflow", "length", "too long", "exceed", "limit"];
  return keywords.some((kw) => msg.includes(kw));
}
