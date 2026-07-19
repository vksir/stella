/**
 * Stella 打包脚本：bun build --compile 产 Windows / Linux 单文件二进制。
 *
 * 用法：
 *   bun run build         # Windows 产物 (dist/stella.exe)
 *   bun run build:linux   # Linux 产物 (dist/stella)
 *
 * 四条 compile 安全纪律：
 *   1. 不开 --minify（Elysia #1711）
 *   2. 不用 @elysiajs/static
 *   3. 不用 macro
 *   4. 不用 fromTypes
 *
 * 生成的二进制需与 config.toml 和 data/ 目录同目录部署。
 */

import { $ } from "bun";

const ENTRY = "src/index.ts";

async function main() {
  const target = process.argv[2] === "linux" ? "linux" : "windows";
  const outfile = target === "linux" ? "dist/stella" : "dist/stella.exe";

  console.log(`[build] 编译 ${target} → ${outfile} ...`);

  const result = await $`bun build --compile ${ENTRY} --outfile ${outfile}`.nothrow();

  if (result.exitCode !== 0) {
    console.error("[build] 编译失败:");
    console.error(result.stderr.toString());
    process.exit(1);
  }

  console.log(`[build] 完成: ${outfile}`);
  console.log(`[build] 部署: 将 ${outfile}、config.toml 放入同一目录，创建 data/ 子目录即可启动`);
}

await main();
