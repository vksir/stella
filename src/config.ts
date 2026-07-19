import { readFileSync } from "node:fs";

// ---- 类型定义 ----

export interface OwnerConfig {
  qq: string;
  api_token: string;
}

export interface NapcatConfig {
  listen: string;
  token: string;
}

export interface ApiConfig {
  listen: string;
}

export interface ModelConfig {
  provider: string;
  name: string;
  /** 思考深度，默认 "high" */
  thinking_level?: "off" | "minimal" | "low" | "medium" | "high" | "xhigh";
}

export interface PathsConfig {
  data_dir: string;
}

export interface MemoryConfig {
  max_entries_per_user: number;
  max_content_chars: number;
}

export interface ShortTermMemoryConfig {
  max_tokens: number;
  max_age_days: number;
}

export interface ToolsConfig {
  whitelist: string[];
}

export interface StellaConfig {
  owner: OwnerConfig;
  napcat: NapcatConfig;
  api: ApiConfig;
  model: ModelConfig;
  paths: PathsConfig;
  memory: MemoryConfig;
  short_term_memory: ShortTermMemoryConfig;
  tools: ToolsConfig;
}

// ---- 默认值 ----

const DEFAULTS: Partial<StellaConfig> = {
  paths: { data_dir: "./data" },
  model: { provider: "", name: "", thinking_level: "high" },
  memory: { max_entries_per_user: 200, max_content_chars: 2000 },
  short_term_memory: { max_tokens: 10000, max_age_days: 3 },
  tools: {
    whitelist: [
      "memory_save",
      "memory_search",
      "memory_list",
      "memory_delete",
    ],
  },
};

// ---- 校验 ----

function validate(raw: Record<string, unknown>): StellaConfig {
  const owner = raw.owner as Record<string, unknown> | undefined;
  if (!owner || typeof owner.qq !== "string" || !owner.qq) {
    throw new Error("配置缺少必填字段: owner.qq（管理员 QQ 号）");
  }
  if (!owner || typeof owner.api_token !== "string" || !owner.api_token) {
    throw new Error("配置缺少必填字段: owner.api_token（管理员 API token）");
  }

  const napcat = raw.napcat as Record<string, unknown> | undefined;
  if (!napcat || typeof napcat.listen !== "string" || !napcat.listen) {
    throw new Error("配置缺少必填字段: napcat.listen");
  }
  if (!napcat || typeof napcat.token !== "string" || !napcat.token) {
    throw new Error("配置缺少必填字段: napcat.token");
  }

  const api = raw.api as Record<string, unknown> | undefined;
  if (!api || typeof api.listen !== "string" || !api.listen) {
    throw new Error("配置缺少必填字段: api.listen");
  }

  const model = raw.model as Record<string, unknown> | undefined;
  if (!model || typeof model.provider !== "string" || !model.provider) {
    throw new Error("配置缺少必填字段: model.provider");
  }
  if (!model || typeof model.name !== "string" || !model.name) {
    throw new Error("配置缺少必填字段: model.name");
  }

  return {
    owner: { qq: owner.qq as string, api_token: owner.api_token as string },
    napcat: { listen: napcat.listen as string, token: napcat.token as string },
    api: { listen: api.listen as string },
    model: {
      provider: model.provider as string,
      name: model.name as string,
      thinking_level: (model.thinking_level as ModelConfig["thinking_level"]) ?? "high",
    },
    paths: {
      data_dir:
        typeof raw.paths === "object" && raw.paths
          ? (raw.paths as Record<string, unknown>).data_dir as string ?? DEFAULTS.paths!.data_dir
          : DEFAULTS.paths!.data_dir,
    },
    memory: {
      max_entries_per_user:
        typeof raw.memory === "object" && raw.memory
          ? ((raw.memory as Record<string, unknown>).max_entries_per_user as number) ?? DEFAULTS.memory!.max_entries_per_user
          : DEFAULTS.memory!.max_entries_per_user,
      max_content_chars:
        typeof raw.memory === "object" && raw.memory
          ? ((raw.memory as Record<string, unknown>).max_content_chars as number) ?? DEFAULTS.memory!.max_content_chars
          : DEFAULTS.memory!.max_content_chars,
    },
    short_term_memory: {
      max_tokens:
        typeof raw.short_term_memory === "object" && raw.short_term_memory
          ? ((raw.short_term_memory as Record<string, unknown>).max_tokens as number) ?? DEFAULTS.short_term_memory!.max_tokens
          : DEFAULTS.short_term_memory!.max_tokens,
      max_age_days:
        typeof raw.short_term_memory === "object" && raw.short_term_memory
          ? ((raw.short_term_memory as Record<string, unknown>).max_age_days as number) ?? DEFAULTS.short_term_memory!.max_age_days
          : DEFAULTS.short_term_memory!.max_age_days,
    },
    tools: {
      whitelist:
        typeof raw.tools === "object" && raw.tools && Array.isArray((raw.tools as Record<string, unknown>).whitelist)
          ? (raw.tools as Record<string, unknown>).whitelist as string[]
          : DEFAULTS.tools!.whitelist,
    },
  };
}

// ---- 公开 API ----

export function loadConfig(path: string): StellaConfig {
  let raw: string;
  try {
    raw = readFileSync(path, "utf-8");
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    throw new Error(`找不到配置文件: ${path}（${msg}）`);
  }

  let parsed: Record<string, unknown>;
  try {
    parsed = Bun.TOML.parse(raw) as Record<string, unknown>;
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    throw new Error(`TOML 解析失败: ${path}（${msg}）`);
  }

  return validate(parsed);
}
