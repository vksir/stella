/**
 * E2E 测试：bootstrap 拉起完整服务 → 创建会话 → 发消息 → 验证 SSE 流式回复 → 停服。
 *
 * 用法：bun run test/e2e.ts
 */

import { bootstrap, type AppContext } from "../src/index";

interface SseToken {
  token: string;
}

async function main(): Promise<void> {
  console.log("[e2e] 启动服务...");

  const ctx: AppContext = await bootstrap();
  const port = ctx.apiServer.server.port;
  const token = ctx.config.owner.api_token;
  const base = `http://127.0.0.1:${port}`;

  try {
    // 1. 创建会话
    const createRes = await fetch(`${base}/sessions`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
    });

    if (createRes.status !== 201) {
      throw new Error(`创建会话失败: ${createRes.status}`);
    }
    const { session_id } = await createRes.json() as { session_id: string };
    console.log(`[e2e] session: ${session_id}`);

    // 2. 发消息 + SSE 流式收
    console.log("[e2e] 发送: '回复一个字：好'");

    const ctrl = new AbortController();
    setTimeout(() => ctrl.abort(), 120_000);

    const msgRes = await fetch(`${base}/sessions/${session_id}/messages`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ content: "回复一个字：好" }),
      signal: ctrl.signal,
    });

    if (msgRes.status !== 200) {
      throw new Error(`消息请求失败: ${msgRes.status}`);
    }

    // 3. 解析 SSE 流
    const reader = msgRes.body!.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    const tokens: string[] = [];

    while (true) {
      const { value, done } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() ?? "";

      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed || trimmed.startsWith(":") || !trimmed.startsWith("data: ")) continue;

        const raw = trimmed.slice("data: ".length);
        if (raw === "[DONE]") break;

        const payload = JSON.parse(raw) as SseToken;
        tokens.push(payload.token);
        process.stdout.write(payload.token);
      }
    }
    process.stdout.write("\n");
    clearTimeout(ctrl as unknown as number);

    const reply = tokens.join("");
    if (!reply.trim()) {
      throw new Error("模型未返回任何内容");
    }

    console.log(`[e2e] 完成，${tokens.length} tokens`);
  } finally {
    ctx.sessions.disposeAll();
    ctx.qqAdapter?.stop();
    await ctx.apiServer.stop();
    console.log("[e2e] 服务已停止");
    process.exit(0);
  }
}

await main();
