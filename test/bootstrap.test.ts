import { describe, expect, it, beforeAll, afterAll } from "bun:test";
import { mkdirSync, existsSync, rmSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { initDataDir } from "../src/bootstrap";

const tmpDir = resolve(import.meta.dir, "..", ".test", "bootstrap");

beforeAll(() => {
  if (existsSync(tmpDir)) rmSync(tmpDir, { recursive: true });
  mkdirSync(tmpDir, { recursive: true });
});

afterAll(() => {
  try { rmSync(tmpDir, { recursive: true }); } catch { /* ok */ }
});

describe("initDataDir", () => {
  it("创建 data_dir 及 sessions 子目录", () => {
    const dataDir = resolve(tmpDir, "data");
    initDataDir(dataDir);

    expect(existsSync(dataDir)).toBe(true);
    expect(existsSync(resolve(dataDir, "sessions"))).toBe(true);
  });

  it("目录已存在时不报错", () => {
    const dataDir = resolve(tmpDir, "data");
    // Already created by previous test
    expect(() => initDataDir(dataDir)).not.toThrow();
  });

  it("路径含嵌套目录时递归创建", () => {
    const dataDir = resolve(tmpDir, "deep", "nested", "data");
    initDataDir(dataDir);
    expect(existsSync(dataDir)).toBe(true);
    expect(existsSync(resolve(dataDir, "sessions"))).toBe(true);
  });
});
