/** OneBot v11 消息段 */
export interface Segment {
  type: string;
  data: Record<string, string | undefined>;
}

/** 群消息发送者信息（精简版） */
export interface SenderInfo {
  nickname: string;
  card?: string;
}

/** 机器人自身名称（v1 硬编码） */
export const SELF_NAME = "Stella";
