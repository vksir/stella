import { describe, expect, it, beforeAll, afterAll } from "bun:test";
import { resolve } from "node:path";
import { writeFileSync, unlinkSync, mkdirSync, existsSync } from "node:fs";
import { loadConfig } from "../src/config";

const tmpDir = resolve(import.meta.dir, "..", ".test", "config");

beforeAll(() => {
  if (!existsSync(tmpDir)) mkdirSync(tmpDir, { recursive: true });
});

afterAll(() => {
  // Clean up test files
  const files = [
    `${tmpDir}/valid.toml`,
    `${tmpDir}/partial.toml`,
    `${tmpDir}/bad.toml`,
  ];
  for (const f of files) {
    try { unlinkSync(f); } catch { /* ok */ }
  }
});

describe("loadConfig", () => {
  it("解析合法的完整 TOML 配置", () => {
    const toml = `
[owner]
qq = "123456789"
api_token = "sk-abc"

[napcat]
listen = "127.0.0.1:8082"
token = "napcat-secret"

[api]
listen = "127.0.0.1:3000"

[model]
provider = "anthropic"
name = "claude-sonnet-4-5"

[paths]
data_dir = "./data"

[memory]
max_entries_per_user = 200
max_content_chars = 2000

[short_term_memory]
max_tokens = 10000
max_age_days = 3

[tools]
whitelist = ["memory_save", "memory_search", "memory_list", "memory_delete"]
`;
    const path = `${tmpDir}/valid.toml`;
    writeFileSync(path, toml);
    const cfg = loadConfig(path);

    expect(cfg.owner.qq).toBe("123456789");
    expect(cfg.owner.api_token).toBe("sk-abc");
    expect(cfg.napcat.listen).toBe("127.0.0.1:8082");
    expect(cfg.napcat.token).toBe("napcat-secret");
    expect(cfg.api.listen).toBe("127.0.0.1:3000");
    expect(cfg.model.provider).toBe("anthropic");
    expect(cfg.model.name).toBe("claude-sonnet-4-5");
    expect(cfg.paths.data_dir).toBe("./data");
    expect(cfg.memory.max_entries_per_user).toBe(200);
    expect(cfg.memory.max_content_chars).toBe(2000);
    expect(cfg.short_term_memory.max_tokens).toBe(10000);
    expect(cfg.short_term_memory.max_age_days).toBe(3);
    expect(cfg.tools.whitelist).toEqual([
      "memory_save",
      "memory_search",
      "memory_list",
      "memory_delete",
    ]);
  });

  it("部分缺失字段使用默认值", () => {
    const toml = `
[owner]
qq = "111"
api_token = "tok"

[napcat]
listen = "0.0.0.0:8082"
token = "t"

[api]
listen = "0.0.0.0:3000"

[model]
provider = "openai"
name = "gpt-4"
`;
    const path = `${tmpDir}/partial.toml`;
    writeFileSync(path, toml);
    const cfg = loadConfig(path);

    // Required fields from file
    expect(cfg.owner.qq).toBe("111");
    // Defaults
    expect(cfg.paths.data_dir).toBe("./data");
    expect(cfg.memory.max_entries_per_user).toBe(200);
    expect(cfg.memory.max_content_chars).toBe(2000);
    expect(cfg.short_term_memory.max_tokens).toBe(10000);
    expect(cfg.short_term_memory.max_age_days).toBe(3);
    expect(cfg.tools.whitelist).toEqual([
      "memory_save",
      "memory_search",
      "memory_list",
      "memory_delete",
    ]);
  });

  it("文件不存在时报清晰错误", () => {
    expect(() => loadConfig("/nonexistent/path.toml")).toThrow(
      /找不到|not found|ENOENT/i,
    );
  });

  it("非法 TOML 语法时报清晰错误", () => {
    const path = `${tmpDir}/bad.toml`;
    writeFileSync(path, "this is not valid toml [[[");
    expect(() => loadConfig(path)).toThrow(/TOML|解析|parse/i);
  });

  it("必填字段缺失时报错", () => {
    // Missing owner section entirely
    const toml = `
[napcat]
listen = "0.0.0.0:8082"
token = "t"

[api]
listen = "0.0.0.0:3000"

[model]
provider = "x"
name = "y"
`;
    const path = `${tmpDir}/partial.toml`;
    writeFileSync(path, toml);
    expect(() => loadConfig(path)).toThrow(/owner/i);
  });
});
