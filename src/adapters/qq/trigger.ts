import type { Segment, SenderInfo } from "./types";
import { SELF_NAME } from "./types";

/**
 * 检查消息段数组中是否存在 @机器人的 at 段。
 */
export function isAtBot(message: Segment[], selfId: string): boolean {
  return message.some(
    (s) => s.type === "at" && String(s.data.qq) === String(selfId),
  );
}

/**
 * 检查消息是否触发 Stella 回复。
 * - 私聊：总是触发
 * - 群聊：仅当被 @机器人 时触发（@全体不算）
 */
export function isTrigger(
  messageType: string,
  message: Segment[],
  selfId: string,
): boolean {
  if (messageType === "private") return true;
  return isAtBot(message, selfId);
}

/**
 * 生成当轮触发注记文本。
 */
export function triggerNoteText(
  chatType: "group" | "private",
  sender: SenderInfo,
  messageId: number,
  userId: number | string,
  selfName: string = SELF_NAME,
): string {
  const name = sender.card || sender.nickname;
  if (chatType === "private") {
    return `【本轮】${name}(${userId}) 私聊你，直接回复即可。`;
  }
  return (
    `【本轮触发】${name}(${userId}) 在 #${messageId} @ 了你（${selfName}）。` +
    `你的回复会自动 @ ${name}；行首写 [reply:#消息id] 可引用某条消息，` +
    `正文里写 @QQ号 可提及他人。`
  );
}
