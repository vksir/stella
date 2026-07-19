import type { Segment, SenderInfo } from "./types";
import { SELF_NAME } from "./types";

/**
 * 把消息段数组渲染为行内文本。
 */
export function segmentsToText(segs: Segment[], selfName: string): string {
  let out = "";
  for (const s of segs) {
    switch (s.type) {
      case "text":
        out += s.data.text ?? "";
        break;
      case "at": {
        const qq = String(s.data.qq);
        if (qq === "all") out += "@全体成员";
        else if (s.data.name === selfName) out += `@${selfName}`;
        else out += s.data.name ? `@${s.data.name}(${qq})` : `@${qq}`;
        break;
      }
      case "reply":
        out += `[引用#${s.data.id}]`;
        break;
      case "image":
        out += "[图片]";
        break;
      case "face":
        out += "[表情]";
        break;
      case "record":
        out += "[语音]";
        break;
      default:
        out += `[${s.type}]`;
    }
  }
  return out.trim();
}

/** 时间戳转 HH:MM 格式（本地时间）。 */
function hhmm(timeSec: number): string {
  const d = new Date(timeSec * 1000);
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

/**
 * 格式化群聊消息为入库文本。
 *
 * 格式：`[#消息id 群名片(QQ号) HH:MM] 内容`
 */
export function formatGroupMessage(
  segs: Segment[],
  sender: SenderInfo,
  messageId: number,
  time: number,
  userId: number | string,
  selfName: string = SELF_NAME,
): string {
  const name = sender.card || sender.nickname;
  const text = segmentsToText(segs, selfName);
  return `[#${messageId} ${name}(${userId}) ${hhmm(time)}] ${text}`;
}

/**
 * 将模型输出文本解析为 OneBot v11 消息段数组。
 */
export function parseOutbound(
  modelText: string,
  chatType: "group" | "private",
  currentSpeakerQq?: string,
): Segment[] {
  let text = modelText.trim();
  const segs: Segment[] = [];

  // 行首引用标记
  const replyMatch = text.match(/^\[reply:#(\d+)\]\s*/);
  if (replyMatch) {
    segs.push({ type: "reply", data: { id: replyMatch[1]! } });
    text = text.slice(replyMatch[0].length);
  }

  // 群聊自动 at 当前说话人
  if (chatType === "group" && currentSpeakerQq) {
    segs.push({ type: "at", data: { qq: currentSpeakerQq } });
  }

  // 正文里的 @QQ号（5+ 位数字）→ at 段
  const re = /@(\d{5,})/g;
  let last = 0;
  let m: RegExpExecArray | null;
  const pushText = (t: string) => {
    if (t.length > 0) segs.push({ type: "text", data: { text: t } });
  };
  while ((m = re.exec(text)) !== null) {
    pushText(text.slice(last, m.index));
    segs.push({ type: "at", data: { qq: m[1]! } });
    last = m.index + m[0].length;
  }
  pushText(text.slice(last));

  return segs;
}

/**
 * 去掉消息段数组中的 reply 段（降级重发用）。
 */
export function stripReplySegments(segs: Segment[]): Segment[] {
  return segs.filter((s) => s.type !== "reply");
}
