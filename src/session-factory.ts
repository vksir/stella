import { createContextTrimmer } from "./context-trimmer";
import {
  createAgentSession,
  ModelRuntime,
  DefaultResourceLoader,
  SessionManager,
  SettingsManager,
} from "@earendil-works/pi-coding-agent";
import type { ToolDefinition, ExtensionFactory } from "@earendil-works/pi-coding-agent";
import type { StellaConfig } from "./config";
import type { ISessionFactory, SessionCreateResult } from "./sessions-registry";

export interface SessionFactoryOptions {
  /** 项目根目录（含 .pi/ 目录） */
  cwd: string;
  /** SDK 全局配置目录 */
  agentDir: string;
  /** 数据目录 */
  dataDir: string;
  /** 会话持久化目录 */
  sessionsDir: string;
  /** 应用配置 */
  config: StellaConfig;
  /** 自定义工具（记忆工具集合） */
  memoryTools?: ToolDefinition[];
  /** 额外扩展工厂 */
  extraExtensions?: ExtensionFactory[];
}

/**
 * 会话工厂：封装所有 SDK 依赖，按平台/chatKey 创建 AgentSession。
 *
 * SDK 依赖（ModelRuntime、createAgentSession、DefaultResourceLoader 等）
 * 全部收敛在此类中，业务代码通过 ISessionFactory 接口使用。
 */
export class SessionFactory implements ISessionFactory {
  private cwd: string;
  private agentDir: string;
  private dataDir: string;
  private sessionsDir: string;
  private config: StellaConfig;
  private memoryTools?: ToolDefinition[];
  private extraExtensions: ExtensionFactory[];
  private trimmer: ReturnType<typeof createContextTrimmer>;

  /** ModelRuntime 惰性初始化（首次 create 时创建，随后复用） */
  private modelRuntimePromise: Promise<ModelRuntime> | null = null;

  constructor(opts: SessionFactoryOptions) {
    this.cwd = opts.cwd;
    this.agentDir = opts.agentDir;
    this.dataDir = opts.dataDir;
    this.sessionsDir = opts.sessionsDir;
    this.config = opts.config;
    this.memoryTools = opts.memoryTools;
    this.extraExtensions = opts.extraExtensions ?? [];
    this.trimmer = createContextTrimmer(opts.config);
  }

  /** 为指定平台和聊天标识创建 AgentSession。 */
  async create(platform: string, chatKey: string): Promise<SessionCreateResult> {
    const modelRuntime = await this.getModelRuntime();

    const model = modelRuntime.getModel(
      this.config.model.provider,
      this.config.model.name,
    );
    if (!model) {
      throw new Error(
        `找不到模型: ${this.config.model.provider}/${this.config.model.name}`,
      );
    }

    const systemPrompt = this.buildSystemPrompt(platform, chatKey);

    const allExtensions = [this.trimmer.extensionFactory];
    if (this.extraExtensions.length > 0) {
      allExtensions.push(...this.extraExtensions);
    }

    const resourceLoader = new DefaultResourceLoader({
      cwd: this.cwd,
      agentDir: this.agentDir,
      systemPrompt,
      extensionFactories: allExtensions,
    });
    await resourceLoader.reload();

    const settingsManager = SettingsManager.inMemory({
      compaction: { enabled: false },
    });

    const sessionManager = SessionManager.create(this.dataDir, this.sessionsDir);

    const { session } = await createAgentSession({
      cwd: this.cwd,
      agentDir: this.agentDir,
      modelRuntime,
      model,
      thinkingLevel: this.config.model.thinking_level ?? "high",
      sessionManager,
      settingsManager,
      resourceLoader,
      customTools: this.memoryTools,
    });

    sessionManager.appendSessionInfo(chatKey);

    return {
      session,
      dispose: () => session.dispose(),
      sessionPath: session.sessionFile || `${this.sessionsDir}/${session.sessionId}.jsonl`,
    };
  }

  // ---- 私有方法 ----

  private async getModelRuntime(): Promise<ModelRuntime> {
    if (!this.modelRuntimePromise) {
      this.modelRuntimePromise = ModelRuntime.create({
        authPath: `${this.dataDir}/auth.json`,
        modelsPath: `${this.dataDir}/models.json`,
      });
    }
    return this.modelRuntimePromise;
  }

  private buildSystemPrompt(platform: string, chatKey: string): string {
    const base = "你是 Stella，一个友好、乐于助人的 AI 助手。";
    const memoryNote = "你拥有长期记忆工具，用于记住用户的重要信息。";

    if (platform === "qq") {
      if (chatKey.startsWith("group:")) {
        return `${base}\n你在一个 QQ 群聊中。群消息以 [#消息id 群名片(QQ号) HH:MM] 格式标注说话人。\n${memoryNote}`;
      }
      return `${base}\n${memoryNote}`;
    }

    return `${base}\n${memoryNote}`;
  }
}
