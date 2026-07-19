import { mkdirSync, existsSync } from "node:fs";
import { resolve } from "node:path";

/**
 * 创建数据目录结构：data_dir/ 及 data_dir/sessions/ 和 data_dir/agent/
 */
export function initDataDir(dataDir: string): void {
  if (!existsSync(dataDir)) {
    mkdirSync(dataDir, { recursive: true });
  }
  const sessionsDir = resolve(dataDir, "sessions");
  if (!existsSync(sessionsDir)) {
    mkdirSync(sessionsDir, { recursive: true });
  }
  const agentDir = resolve(dataDir, "agent");
  if (!existsSync(agentDir)) {
    mkdirSync(agentDir, { recursive: true });
  }
}
