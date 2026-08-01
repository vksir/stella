/**
 * OneBot v11 反向 WS 传输层（协议 + 框架接入）。
 *
 * 结构：
 * - OneBotClient 接口：业务层（conversation.ts）依赖的发送边界
 * - OneBotHandlers 接口：本模块接收消息事件的输入边界（不依赖业务层类型）
 * - OneBotConnection：连接级，NapCat 每次重连新建（echo 关联/心跳/超时）
 * - OneBotApiClient：跨连接门面（attach/detach 切换连接，reply 降级重试）
 * - OneBotWsServer：注册到 Elysia 的 WS 路由（与 API 共享端口，不单独监听）
 *
 * 接口解决双向依赖：conversation → OneBotClient（type），本模块不 import 业务层。
 * 测试以真实 Bun WS 客户端扮演 NapCat，经同一 seam 进入。
 */

import { Elysia } from "elysia";
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

/** 事件回调：收到 message 事件后分发到这里（结构上由业务层对象满足） */
export interface OneBotHandlers {
  onMessage(event: OneBotMessageEvent): void | Promise<void>;
}

// ---- 工具 ----

function uuid(): string {
  return crypto.randomUUID();
}

function hasReplySegment(segs: Segment[]): boolean {
  return segs.some((s) => s.type === "reply");
}

/**
 * WS 连接最小接口：只声明用到的发送能力。
 * Elysia 的 ws.raw（Bun ServerWebSocket）结构满足；避免绑定框架内部类型。
 */
export interface WsSocket {
  send(data: string): unknown;
}

/** WS 回调所需的最小连接视图（Elysia 的 ElysiaWS 结构满足） */
export interface WsCtx {
  /** 升级时的原始请求（类型为索引签名，request 运行时必有） */
  data: { request?: Request };
  raw: WsSocket;
  close(code: number, reason?: string): void;
}

// ---- 连接状态 ----

/**
 * 单个 WS 连接：协议状态（echo 关联、心跳）归本对象。
 * NapCat 每次重连都会新建一个实例；pendingEchoes 为实例字段，多连接不串。
 */
export class OneBotConnection {
  /** 最近一次心跳时间戳（毫秒） */
  lastHeartbeat: number;

  /** echo UUID → 挂起调用（等待 API 回执） */
  private pendingEchoes = new Map<
    string,
    { resolve: (resp: ApiResponse) => void; reject: (err: Error) => void; timer: ReturnType<typeof setTimeout> }
  >();

  constructor(
    readonly ws: WsSocket,
    readonly selfId: string,
  ) {
    this.lastHeartbeat = Date.now();
  }

  /** 通过 WS 发送 API 调用并等待回执。 */
  sendApi(action: string, params: Record<string, unknown>, timeoutMs: number = 10000): Promise<ApiResponse> {
    const echo = uuid();
    const payload = JSON.stringify({ action, params, echo });

    return new Promise<ApiResponse>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pendingEchoes.delete(echo);
        reject(new Error(`API 调用超时: ${action} (echo=${echo})`));
      }, timeoutMs);

      this.pendingEchoes.set(echo, { resolve, reject, timer });

      this.ws.send(payload);
    });
  }

  /**
   * 消费 API 回执帧（含 echo 和 status）。
   * 命中则 resolve 对应调用并返回 true；否则返回 false，由服务端按 post_type 分流。
   */
  handleEchoFrame(data: Record<string, unknown>): boolean {
    if (!("echo" in data) || typeof data.echo !== "string" || !("status" in data)) {
      return false;
    }
    const echo = data.echo;
    const entry = this.pendingEchoes.get(echo);
    if (!entry) return false;

    clearTimeout(entry.timer);
    this.pendingEchoes.delete(echo);
    entry.resolve(data as unknown as ApiResponse);
    return true;
  }

  /** 心跳更新。 */
  markHeartbeat(): void {
    this.lastHeartbeat = Date.now();
  }

  /** 连接收尾：拒绝所有挂起调用，防泄漏。 */
  dispose(): void {
    for (const [, entry] of this.pendingEchoes) {
      clearTimeout(entry.timer);
      entry.reject(new Error("连接已关闭"));
    }
    this.pendingEchoes.clear();
  }
}

// ---- API 客户端 ----

/**
 * 跨连接 API 入口：业务层持有的稳定对象（实现 OneBotClient 接口）。
 * 连接随 WS 重连切换（attach/detach），入口本身不重建。
 */
export class OneBotApiClient implements OneBotClient {
  /** 当前连接的自机器人 ID（随连接更新） */
  selfId = "";

  private connection: OneBotConnection | null = null;

  /** 连接建立时挂载（由服务端在 open 时调用）。 */
  attach(conn: OneBotConnection): void {
    this.connection = conn;
    this.selfId = conn.selfId;
  }

  /** 连接关闭时摘除（由服务端在 close 时调用；仅当是当前连接）。 */
  detach(conn: OneBotConnection): void {
    if (this.connection !== conn) return;
    this.connection = null;
    this.selfId = "";
  }

  /** 发起 API 调用并等待 echo 回执。 */
  send(action: string, params: Record<string, unknown>, timeoutMs?: number): Promise<ApiResponse> {
    const conn = this.connection;
    if (!conn) return Promise.reject(new Error("NapCat 未连接"));
    return conn.sendApi(action, params, timeoutMs);
  }

  /**
   * 发送群聊消息（含降级重试）。
   * 引用过期/reply 相关错误 → 去掉 reply 段重发。
   */
  async sendGroupMessage(groupId: number, segments: Segment[]): Promise<void> {
    try {
      const resp = await this.send("send_group_msg", {
        group_id: groupId,
        message: segments,
      });

      if (resp.retcode !== 0 && hasReplySegment(segments)) {
        console.log(`[QQ] 群 ${groupId} 发送失败（retcode=${resp.retcode}），去掉 reply 重发`);
        const stripped = stripReplySegments(segments);
        await this.send("send_group_msg", {
          group_id: groupId,
          message: stripped,
        });
      }
    } catch (err) {
      console.error(`[QQ] 群 ${groupId} 发送异常:`, err);
    }
  }

  /** 发送私聊消息。 */
  async sendPrivateMessage(userId: number, segments: Segment[]): Promise<void> {
    try {
      await this.send("send_private_msg", {
        user_id: userId,
        message: segments,
      });
    } catch (err) {
      console.error(`[QQ] 私聊 ${userId} 发送异常:`, err);
    }
  }
}

// ---- WS 服务端（Elysia 接入） ----

/**
 * OneBot 反向 WS：注册到 Elysia 实例的 WS 路由（与 API 共享端口）。
 * 鉴权在 open 时校验握手头（Authorization / X-Self-ID），失败立即 close。
 * 连接注册表以 Bun 原生 WS 对象为 key（跨回调稳定）。
 */
export class OneBotWsServer {
  private connections = new Map<WsSocket, OneBotConnection>();

  constructor(
    private readonly config: NapcatConfig,
    private readonly handlers: OneBotHandlers,
    private readonly client: OneBotApiClient,
  ) {}

  /** 注册 WS 路由到 Elysia 实例；必须在 listen 之前调用。 */
  register(app: Elysia<any, any, any, any, any, any, any>): void {
    app.ws(this.config.path, {
      open: (ws: WsCtx) => {
        const request = ws.data.request;
        if (!request) {
          ws.close(4001, "Missing request");
          return;
        }

        // 鉴权：校验 Bearer token
        const auth = request.headers.get("Authorization");
        if (this.config.token && auth !== `Bearer ${this.config.token}`) {
          console.log("[QQ] WS 鉴权失败，拒绝连接");
          ws.close(4001, "Unauthorized");
          return;
        }

        const selfId = request.headers.get("X-Self-ID");
        if (!selfId) {
          console.log("[QQ] WS 缺少 X-Self-ID 头");
          ws.close(4001, "Missing X-Self-ID");
          return;
        }

        const conn = new OneBotConnection(ws.raw, selfId);
        this.connections.set(ws.raw, conn);
        this.client.attach(conn);

        console.log(`[QQ] NapCat 已连接 WebSocket (self_id=${selfId})`);
      },

      message: (ws: WsCtx, message: unknown) => {
        const conn = this.connections.get(ws.raw);
        if (!conn) return;

        // Elysia 默认解析 JSON 帧（{ 开头）；非对象帧（心跳文本等）丢弃
        if (typeof message !== "object" || message === null) return;

        this.dispatch(conn, message as Record<string, unknown>);
      },

      close: (ws: WsCtx) => {
        const conn = this.connections.get(ws.raw);
        if (!conn) return;

        conn.dispose();
        this.client.detach(conn);
        this.connections.delete(ws.raw);
        console.log(`[QQ] NapCat 已断开 (self_id=${conn.selfId})`);
      },
    });
  }

  /** 断开所有连接（适配器停止用）。 */
  stop(): void {
    for (const conn of this.connections.values()) {
      conn.dispose();
    }
    this.connections.clear();
  }

  // ---- 帧分流 ----

  private dispatch(conn: OneBotConnection, data: Record<string, unknown>): void {
    // API 回执（含 echo 和 status）→ 连接层消费
    if (conn.handleEchoFrame(data)) return;

    const postType = data.post_type as string | undefined;

    // 按 post_type 分流
    if (postType === "message") {
      Promise.resolve(this.handlers.onMessage(data as unknown as OneBotMessageEvent)).catch((err) => {
        console.error("[QQ] 消息处理异常:", err);
      });
    } else if (postType === "meta_event") {
      const meta = data as unknown as OneBotMetaEvent;
      if (meta.meta_event_type === "heartbeat") {
        conn.markHeartbeat();
        if (meta.status) {
          console.log(
            `[QQ] 心跳 (online=${meta.status.online}, good=${meta.status.good}, interval=${meta.interval}ms)`,
          );
        }
      } else if (meta.meta_event_type === "lifecycle") {
        console.log(`[QQ] lifecycle (${meta.sub_type ?? "unknown"})`);
      }
    } else if (postType === "message_sent") {
      console.log(`[QQ] message_sent (忽略): user=${data.user_id}`);
    } else if (postType === "notice") {
      console.log(`[QQ] notice (忽略): type=${data.notice_type}`);
    } else if (postType === "request") {
      console.log(`[QQ] request (忽略): type=${data.request_type}`);
    } else {
      console.log(`[QQ] 未知 post_type: ${postType}`);
    }
  }
}
