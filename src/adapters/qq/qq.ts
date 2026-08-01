/**
 * QQ 适配器 — ticket 05
 *
 * 组合根：装配 OneBot 传输客户端（onebot.ts）与会话回合驱动（conversation.ts），
 * 启动反向 WS 服务端。业务编排与传输层均在其各自 module 内，此处只做接线。
 */

import type { AppContext } from "../../index";
import { startOneBotServer } from "./onebot";
import { createConversation } from "./conversation";

/**
 * 启动 QQ 适配器（反向 WS 服务端）。
 */
export interface QQAdapterHandle {
  stop(): void;
}

export async function startQQAdapter(ctx: AppContext): Promise<QQAdapterHandle> {
  const { napcat } = ctx.config;

  console.log(`[QQ] 启动 NapCat 反向 WS 服务端: ${napcat.listen}`);

  const server = await startOneBotServer(napcat);
  const conversation = createConversation(ctx, server.client);

  // 事件分发：传输层回调 → 回合驱动
  server.setHandlers({
    onMessage: (event) => conversation.onMessage(event),
    onMetaEvent: (event) => conversation.onMetaEvent(event),
  });

  console.log(`[QQ] WS 服务端已启动: ws://${napcat.listen}`);

  return {
    stop: () => server.stop(),
  };
}
