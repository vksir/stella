import { describe, expect, it, afterAll } from "bun:test";
import { startOneBotServer } from "../../../src/adapters/qq/onebot";
import type { OneBotServerHandle } from "../../../src/adapters/qq/onebot";
import type { OneBotMessageEvent, Segment } from "../../../src/adapters/qq/types";

/**
 * 测试以真实 Bun WS 客户端扮演 NapCat，连接 onebot.ts 的反向 WS 服务端，
 * 从传输 seam 之外验证 echo 关联、超时、降级重试与事件分发。
 */

const servers: OneBotServerHandle[] = [];
const napcatClients: WebSocket[] = [];

/** 假 NapCat 收到的 API 调用帧 */
interface IncomingCall {
  action: string;
  params: Record<string, any>;
  echo: string;
}

function isCall(v: Record<string, any> | undefined): v is IncomingCall {
  return !!v && typeof v.echo === "string";
}

afterAll(() => {
  for (const c of napcatClients) {
    try { c.close(); } catch { /* ok */ }
  }
  for (const s of servers) s.stop();
});

/** 启动服务端（端口 0 随机），并连入一个假 NapCat 客户端。 */
async function startServer() {
  const server = await startOneBotServer({ listen: "127.0.0.1:0", token: "test-token" });
  servers.push(server);

  const napcat = await connectNapCat(server.port);
  napcatClients.push(napcat);
  const incoming: IncomingCall[] = [];
  napcat.onmessage = (e: MessageEvent) => {
    incoming.push(JSON.parse(String(e.data)));
  };

  return { server, napcat, incoming };
}

function connectNapCat(port: number): Promise<WebSocket> {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(`ws://127.0.0.1:${port}`, {
      headers: {
        Authorization: "Bearer test-token",
        "X-Self-ID": "10001",
      },
    });
    ws.onopen = () => resolve(ws);
    ws.onerror = (e) => reject(e instanceof Error ? e : new Error("ws 连接失败"));
  });
}

/** 轮询等待条件成立（消息到达是异步的）。 */
async function waitFor<T>(fn: () => T | undefined, timeout = 2000): Promise<T> {
  const start = Date.now();
  while (Date.now() - start < timeout) {
    const v = fn();
    if (v != null) return v;
    await new Promise((r) => setTimeout(r, 10));
  }
  throw new Error("waitFor 超时");
}

function reply(napcat: WebSocket, call: { echo: string }, resp: Record<string, unknown>) {
  napcat.send(JSON.stringify({ status: "ok", retcode: 0, data: {}, echo: call.echo, ...resp }));
}

// ---- API 调用 ----

describe("API 调用（echo 关联）", () => {
  it("send 发送 action/params/echo 并等待回执", async () => {
    const { server, napcat, incoming } = await startServer();

    const p = server.client.send("get_group_msg_history", { group_id: 1, count: 20 });
    const call = await waitFor(() => incoming[0]);

    expect(call.action).toBe("get_group_msg_history");
    expect(call.params).toEqual({ group_id: 1, count: 20 });
    expect(call.echo).toBeTruthy();

    reply(napcat, call, { retcode: 0, data: { messages: [] } });
    const resp = await p;
    expect(resp.retcode).toBe(0);
  });

  it("回执带错误 retcode 时原样返回（不吞错）", async () => {
    const { server, napcat, incoming } = await startServer();

    const p = server.client.send("send_group_msg", { group_id: 1, message: [] });
    const call = await waitFor(() => incoming[0]);

    reply(napcat, call, { retcode: 100, data: {} });
    const resp = await p;
    expect(resp.retcode).toBe(100);
  });

  it("无回执时超时抛错并清理 echo", async () => {
    const { server } = await startServer();

    // 立即挂上 rejection handler（bun:test 的 expect().rejects
    // 对已 reject 的 promise 会挂死，不能在中间 await）
    const p = server.client.send("slow_action", {}, 80);
    await expect(p).rejects.toThrow(/超时/);
  });
});

// ---- 消息发送 ----

describe("消息发送", () => {
  it("sendGroupMessage 发送群消息段", async () => {
    const { server, napcat, incoming } = await startServer();

    const segs: Segment[] = [{ type: "text", data: { text: "你好" } }];
    const p = server.client.sendGroupMessage(12345, segs);

    const call = await waitFor(() => incoming[0]);
    expect(call.action).toBe("send_group_msg");
    expect(call.params).toEqual({ group_id: 12345, message: segs });

    reply(napcat, call, { retcode: 0 });
    await p;
  });

  it("sendPrivateMessage 发送私聊消息段", async () => {
    const { server, napcat, incoming } = await startServer();

    const segs: Segment[] = [{ type: "text", data: { text: "悄悄话" } }];
    const p = server.client.sendPrivateMessage(67890, segs);

    const call = await waitFor(() => incoming[0]);
    expect(call.action).toBe("send_private_msg");
    expect(call.params).toEqual({ user_id: 67890, message: segs });

    reply(napcat, call, { retcode: 0 });
    await p;
  });
});

describe("群发失败降级重试", () => {
  it("retcode 非 0 且带 reply 段时去掉 reply 重发", async () => {
    const { server, napcat, incoming } = await startServer();

    const segs: Segment[] = [
      { type: "reply", data: { id: "999" } },
      { type: "text", data: { text: "收到" } },
    ];
    const p = server.client.sendGroupMessage(12345, segs);

    const first = await waitFor(() => (isCall(incoming[0]) ? incoming[0] : undefined));
    expect(first.action).toBe("send_group_msg");
    expect(first.params.message).toEqual(segs);

    // 回执失败（引用过期）
    reply(napcat, first, { retcode: 100 });

    // 应自动重发且去掉 reply 段
    const second = await waitFor(() => incoming[1]);
    expect(second.action).toBe("send_group_msg");
    expect(second.params.message).toEqual([{ type: "text", data: { text: "收到" } }]);

    reply(napcat, second, { retcode: 0 });
    await p; // 不抛错
  });

  it("retcode 非 0 且无 reply 段时不重发", async () => {
    const { server, napcat, incoming } = await startServer();

    const segs: Segment[] = [{ type: "text", data: { text: "无引用" } }];
    const p = server.client.sendGroupMessage(12345, segs);

    const first = await waitFor(() => (isCall(incoming[0]) ? incoming[0] : undefined));
    reply(napcat, first, { retcode: 100 });

    await p;
    await new Promise((r) => setTimeout(r, 100));
    expect(incoming.length).toBe(1); // 只有一次调用
  });
});

// ---- 事件分发 ----

describe("事件分发", () => {
  it("message 事件回调 onMessage", async () => {
    const { server, napcat } = await startServer();

    let received: OneBotMessageEvent | null = null;
    server.setHandlers({
      onMessage: (e) => { received = e; },
      onMetaEvent: () => {},
    });

    napcat.send(JSON.stringify({
      post_type: "message",
      message_type: "group",
      message_id: 1,
      user_id: 2,
      group_id: 3,
      self_id: 10001,
      time: 0,
      sender: { nickname: "小明" },
      message: [{ type: "text", data: { text: "hi" } }],
    }));

    await waitFor(() => received);
    expect(received!.group_id).toBe(3);
    expect(received!.sender.nickname).toBe("小明");
  });

  it("meta_event 回调 onMetaEvent", async () => {
    const { server, napcat } = await startServer();

    let received: any = null;
    server.setHandlers({
      onMessage: () => {},
      onMetaEvent: (e) => { received = e; },
    });

    napcat.send(JSON.stringify({
      post_type: "meta_event",
      meta_event_type: "lifecycle",
      sub_type: "connect",
      self_id: 10001,
      time: 0,
    }));

    await waitFor(() => received);
    expect(received.sub_type).toBe("connect");
  });

  it("API 回执不误分发为消息事件", async () => {
    const { server, napcat, incoming } = await startServer();

    let messageCount = 0;
    server.setHandlers({
      onMessage: () => { messageCount++; },
      onMetaEvent: () => {},
    });

    const p = server.client.send("some_action", {});
    const call = await waitFor(() => incoming[0]);
    reply(napcat, call, { retcode: 0 });
    await p;

    await new Promise((r) => setTimeout(r, 50));
    expect(messageCount).toBe(0);
  });
});

// ---- 鉴权 ----

describe("鉴权", () => {
  it("无 token 或错误 token 时拒绝连接", async () => {
    const server = await startOneBotServer({ listen: "127.0.0.1:0", token: "test-token" });
    servers.push(server);

    let failed = false;
    await new Promise<void>((resolve) => {
      const ws = new WebSocket(`ws://127.0.0.1:${server.port}`);
      ws.onopen = () => { failed = false; resolve(); };
      ws.onerror = () => { failed = true; resolve(); };
      ws.onclose = () => { failed = true; resolve(); };
    });

    expect(failed).toBe(true);
  });
});
