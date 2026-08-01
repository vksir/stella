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

// ---- OneBot v11 协议类型 ----

/** WS API 调用回执 */
export interface ApiResponse {
  status: string;
  retcode: number;
  data: Record<string, unknown>;
  echo?: string;
  message?: string;
  wording?: string;
}

/** OneBot 消息事件（精简） */
export interface OneBotMessageEvent {
  post_type: "message";
  message_type: "private" | "group";
  message_id: number;
  user_id: number;
  group_id?: number;
  self_id: number;
  time: number;
  sender: {
    nickname: string;
    card?: string;
  };
  message: Segment[];
}

/** OneBot 元事件 */
export interface OneBotMetaEvent {
  post_type: "meta_event";
  meta_event_type: "lifecycle" | "heartbeat";
  self_id: number;
  time: number;
  sub_type?: string;
  status?: { online: boolean; good: boolean };
  interval?: number;
}
