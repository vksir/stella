/**
 * QQ 适配器 — ticket 05
 *
 * 组合根：装配 OneBot 传输层（onebot.ts）与会话回合驱动（conversation.ts），
 * 把 WS 路由注册到 Elysia 实例（与 API 共享端口）。此处只做接线。
 */

import { Elysia } from "elysia";
import type { AppContext } from "../../index";
import type { TriggerNoteBus } from "./conversation";
import { QQConversation } from "./conversation";
import { OneBotApiClient, OneBotWsServer } from "./onebot";

/**
 * 启动 QQ 适配器：把 OneBot 反向 WS 注册到 Elysia 实例（须在 listen 之前调用）。
 */
export interface QQAdapterHandle {
  stop(): void;
}

export async function startQQAdapter(
  ctx: AppContext,
  noteBus: TriggerNoteBus,
  app: Elysia,
): Promise<QQAdapterHandle> {
  const { napcat } = ctx.config;

  console.log(`[QQ] 注册 NapCat 反向 WS: ${napcat.path}`);

  // 依赖倒置装配顺序：client → conversation → server
  // conversation 结构上满足 OneBotHandlers（onMessage），直接作为事件回调传入
  const client = new OneBotApiClient();
  const conversation = new QQConversation(ctx, client, noteBus);
  const server = new OneBotWsServer(napcat, conversation, client);
  server.register(app);

  return {
    stop: () => server.stop(),
  };
}
