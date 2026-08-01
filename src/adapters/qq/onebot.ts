/**
 * OneBot v11 反向 WS 传输客户端（传输 + 协议层）。
 *
 * 职责：
 * - Bun.serve 反向 WebSocket 服务端（鉴权握手、连接生命周期、心跳）
 * - OneBot API 调用：echo 关联、超时清理、reply 段过期降级重试
 * - 事件帧分发：post_type 分流后回调 handlers
 *
 * 不含业务逻辑（触发/会话/回复在 conversation.ts）；Bun 类型不出此模块。
 * 测试以真实 Bun WS 客户端扮演 NapCat，经同一 seam 进入。
 */

import type { NapcatConfig } from "../../config";
import type { ApiResponse, OneBotMessageEvent, OneBotMetaEvent, Segment } from "./types";
import { stripReplySegments } from "./format";

// ---- 接口 ----

/** OneBot 客户端句柄：业务代码与传输层之间的 interface */
export interface OneBotClient {
  /** 当前连接的自机器人 ID（随 WS 连接更新） */
  selfId: string;
  /** 发起 API 调用并等待 echo 回执 */
  send(action: string, params: Record<string, unknown>, timeoutMs?: number): Promise<ApiResponse>;
  /** 发送群聊消息（含 reply 段过期降级重试） */
  sendGroupMessage(groupId: number, segments: Segment[]): Promise<void>;
  /** 发送私聊消息 */
  sendPrivateMessage(userId: number, segments: Segment[]): Promise<void>;
}

/** 事件回调：连接层收到消息/元事件后分发到这里 */
export interface OneBotHandlers {
  onMessage(event: OneBotMessageEvent): void | Promise<void>;
  onMetaEvent(event: OneBotMetaEvent): void | Promise<void>;
}

/** startOneBotServer 返回句柄 */
export interface OneBotServerHandle {
  client: OneBotClient;
  /** 实际监听端口（listen 填 0 时用于测试取真实端口） */
  port: number;
  /** 晚接线事件回调（client 就绪后再挂 conversation） */
  setHandlers(handlers: OneBotHandlers): void;
  stop(): void;
}

// ---- 连接状态 ----

interface ConnectionState {
  ws: import("bun").ServerWebSocket<WSData>;
  selfId: string;
  connected: boolean;
  lastHeartbeat: number;
}

/** WS 连接的自定义 data 类型 */
interface WSData {
  authHeader: string | null;
  selfIdHeader: string | null;
  conn?: ConnectionState;
}

/** echo UUID → resolve 映射（等待 API 回执） */
const pendingEchoes = new Map<string, (resp: ApiResponse) => void>();

// ---- UUID 工具 ----

function uuid(): string {
  return crypto.randomUUID();
}

// ---- API 调用 ----

/**
 * 通过 WS 发送 API 调用并等待回执。
 */
function sendApi(
  ws: import("bun").ServerWebSocket<WSData>,
  action: string,
  params: Record<string, unknown>,
  timeoutMs: number = 10000,
): Promise<ApiResponse> {
  const echo = uuid();
  const payload = JSON.stringify({ action, params, echo });

  return new Promise<ApiResponse>((resolve, reject) => {
    const timer = setTimeout(() => {
      pendingEchoes.delete(echo);
      reject(new Error(`API 调用超时: ${action} (echo=${echo})`));
    }, timeoutMs);

    pendingEchoes.set(echo, (resp: ApiResponse) => {
      clearTimeout(timer);
      resolve(resp);
    });

    ws.send(payload);
  });
}

function hasReplySegment(segs: Segment[]): boolean {
  return segs.some((s) => s.type === "reply");
}

/**
 * 发送群聊消息（含降级重试）。
 */
async function sendGroupMessage(
  ws: import("bun").ServerWebSocket<WSData>,
  groupId: number,
  segments: Segment[],
): Promise<void> {
  try {
    const resp = await sendApi(ws, "send_group_msg", {
      group_id: groupId,
      message: segments,
    });

    // 引用过期/reply 相关错误 → 去掉 reply 段重发
    if (resp.retcode !== 0 && hasReplySegment(segments)) {
      console.log(`[QQ] 群 ${groupId} 发送失败（retcode=${resp.retcode}），去掉 reply 重发`);
      const stripped = stripReplySegments(segments);
      await sendApi(ws, "send_group_msg", {
        group_id: groupId,
        message: stripped,
      });
    }
  } catch (err) {
    console.error(`[QQ] 群 ${groupId} 发送异常:`, err);
  }
}

/**
 * 发送私聊消息。
 */
async function sendPrivateMessage(
  ws: import("bun").ServerWebSocket<WSData>,
  userId: number,
  segments: Segment[],
): Promise<void> {
  try {
    await sendApi(ws, "send_private_msg", {
      user_id: userId,
      message: segments,
    });
  } catch (err) {
    console.error(`[QQ] 私聊 ${userId} 发送异常:`, err);
  }
}

// ---- WS 服务端 ----

const DEFAULT_HANDLERS: OneBotHandlers = {
  onMessage: () => {},
  onMetaEvent: () => {},
};

/**
 * 启动 OneBot 反向 WS 服务端（传输 + 协议层）。
 */
export async function startOneBotServer(config: NapcatConfig): Promise<OneBotServerHandle> {
  const [host, portStr] = config.listen.split(":");
  const port = parseInt(portStr!, 10);
  let handlers: OneBotHandlers = DEFAULT_HANDLERS;

  const client: OneBotClient = {
    selfId: "",
    send: (action, params, timeoutMs) => {
      const conn = currentConn;
      if (!conn) return Promise.reject(new Error("NapCat 未连接"));
      return sendApi(conn.ws, action, params, timeoutMs);
    },
    sendGroupMessage: (groupId, segments) => {
      const conn = currentConn;
      if (!conn) return Promise.reject(new Error("NapCat 未连接"));
      return sendGroupMessage(conn.ws, groupId, segments);
    },
    sendPrivateMessage: (userId, segments) => {
      const conn = currentConn;
      if (!conn) return Promise.reject(new Error("NapCat 未连接"));
      return sendPrivateMessage(conn.ws, userId, segments);
    },
  };

  // 当前活动连接（NapCat 重连时更新）
  let currentConn: ConnectionState | null = null;

  const server = Bun.serve<WSData>({
    hostname: host,
    port,
    fetch(req, server) {
      // 解析握手头
      const auth = req.headers.get("Authorization");
      const selfId = req.headers.get("X-Self-ID");

      // 鉴权：校验 Bearer token
      const expectedToken = `Bearer ${config.token}`;
      if (config.token && auth !== expectedToken) {
        console.log("[QQ] WS 鉴权失败，返回 401");
        return new Response("Unauthorized", { status: 401 });
      }

      if (!selfId) {
        console.log("[QQ] WS 缺少 X-Self-ID 头");
        return new Response("Missing X-Self-ID", { status: 400 });
      }

      // 升级 WebSocket，传递握手头信息
      const upgraded = server.upgrade(req, {
        data: { authHeader: auth, selfIdHeader: selfId },
      });

      if (!upgraded) {
        return new Response("WebSocket upgrade failed", { status: 500 });
      }

      return undefined; // 升级后无需返回 Response
    },
    websocket: {
      open(ws) {
        const selfId = ws.data.selfIdHeader;
        if (!selfId) {
          ws.close(4001, "Missing X-Self-ID");
          return;
        }

        const conn: ConnectionState = {
          ws,
          selfId,
          connected: false,
          lastHeartbeat: Date.now(),
        };
        ws.data.conn = conn;
        currentConn = conn;
        client.selfId = selfId;

        console.log(`[QQ] NapCat 已连接 WebSocket (self_id=${selfId})`);
      },

      message(ws, raw) {
        const conn = ws.data.conn;
        if (!conn) return;

        let data: Record<string, unknown>;
        try {
          data = JSON.parse(raw as string) as Record<string, unknown>;
        } catch {
          console.error("[QQ] 无法解析 WS 消息:", String(raw).slice(0, 200));
          return;
        }

        // 检查是否为 API 回执（含 echo 和 status）
        if ("echo" in data && typeof data.echo === "string" && "status" in data) {
          const resp = data as unknown as ApiResponse;
          const handler = pendingEchoes.get(resp.echo!);
          if (handler) {
            pendingEchoes.delete(resp.echo!);
            handler(resp);
          }
          return;
        }

        const postType = data.post_type as string | undefined;

        // 按 post_type 分流
        if (postType === "message") {
          Promise.resolve(handlers.onMessage(data as unknown as OneBotMessageEvent)).catch((err) => {
            console.error("[QQ] 消息处理异常:", err);
          });
        } else if (postType === "meta_event") {
          const meta = data as unknown as OneBotMetaEvent;
          if (meta.meta_event_type === "heartbeat") {
            conn.lastHeartbeat = Date.now();
            if (meta.status) {
              console.log(
                `[QQ] 心跳 (online=${meta.status.online}, good=${meta.status.good}, interval=${meta.interval}ms)`,
              );
            }
          }
          Promise.resolve(handlers.onMetaEvent(meta)).catch((err) => {
            console.error("[QQ] 元事件处理异常:", err);
          });
        } else if (postType === "message_sent") {
          console.log(`[QQ] message_sent (忽略): user=${data.user_id}`);
        } else if (postType === "notice") {
          console.log(`[QQ] notice (忽略): type=${data.notice_type}`);
        } else if (postType === "request") {
          console.log(`[QQ] request (忽略): type=${data.request_type}`);
        } else {
          console.log(`[QQ] 未知 post_type: ${postType}`);
        }
      },

      close(ws) {
        const conn = ws.data.conn;
        if (conn) {
          conn.connected = false;
          if (currentConn === conn) currentConn = null;
          console.log(`[QQ] NapCat 已断开 (self_id=${conn.selfId})`);
        }
      },
    },
  });

  return {
    client,
    port: server.port ?? 0,
    setHandlers: (h) => {
      handlers = h;
    },
    stop: () => {
      server.stop(true);
    },
  };
}
