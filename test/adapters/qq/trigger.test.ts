import { describe, expect, it } from "bun:test";
import { isAtBot, isTrigger, triggerNoteText } from "../../../src/adapters/qq/trigger";
import type { Segment, SenderInfo } from "../../../src/adapters/qq/types";

const SELF_ID = "123456789";
const SELF_NAME = "Stella";

// ---- 触发判定 ----

describe("触发判定", () => {
  it("@机器人触发", () => {
    const msg: Segment[] = [
      { type: "at", data: { qq: "123456789" } },
      { type: "text", data: { text: " 你好" } },
    ];
    expect(isAtBot(msg, SELF_ID)).toBe(true);
    expect(isTrigger("group", msg, SELF_ID)).toBe(true);
  });

  it("@全体不触发（isAtBot 返回 false）", () => {
    const msg: Segment[] = [
      { type: "at", data: { qq: "all" } },
      { type: "text", data: { text: " 大家好啊" } },
    ];
    expect(isAtBot(msg, SELF_ID)).toBe(false);
    expect(isTrigger("group", msg, SELF_ID)).toBe(false);
  });

  it("同时 @机器人和 @全体 → 触发", () => {
    const msg: Segment[] = [
      { type: "at", data: { qq: "all" } },
      { type: "at", data: { qq: "123456789" } },
      { type: "text", data: { text: " 大家好" } },
    ];
    expect(isAtBot(msg, SELF_ID)).toBe(true);
    expect(isTrigger("group", msg, SELF_ID)).toBe(true);
  });

  it("无 @ 不触发", () => {
    const msg: Segment[] = [
      { type: "text", data: { text: "今天天气不错" } },
    ];
    expect(isAtBot(msg, SELF_ID)).toBe(false);
    expect(isTrigger("group", msg, SELF_ID)).toBe(false);
  });

  it("@别人不触发", () => {
    const msg: Segment[] = [
      { type: "at", data: { qq: "111111111" } },
      { type: "text", data: { text: " 你好" } },
    ];
    expect(isAtBot(msg, SELF_ID)).toBe(false);
    expect(isTrigger("group", msg, SELF_ID)).toBe(false);
  });

  it("私聊逐条必触发", () => {
    const msg: Segment[] = [
      { type: "text", data: { text: "你好" } },
    ];
    expect(isTrigger("private", msg, SELF_ID)).toBe(true);
  });

  it("self_id 类型比较（selfId 和 qq 都用 String 比较）", () => {
    const msg: Segment[] = [
      { type: "at", data: { qq: "123456789" } },
    ];
    expect(isAtBot(msg, SELF_ID)).toBe(true);
  });
});

// ---- 触发注记 ----

describe("触发注记 triggerNoteText", () => {
  it("群聊触发注记包含关键信息", () => {
    const note = triggerNoteText("group", { nickname: "小明", card: "小明(开发)" }, 1024, 888, SELF_NAME);
    expect(note).toContain("小明(开发)(888)");
    expect(note).toContain("#1024");
    expect(note).toContain("Stella");
    expect(note).toContain("[reply:#消息id]");
    expect(note).toContain("@QQ号");
  });

  it("私聊触发注记", () => {
    const note = triggerNoteText("private", { nickname: "小红" }, 0, 666, SELF_NAME);
    expect(note).toContain("小红(666)");
    expect(note).toContain("私聊");
  });

  it("无群名片时用昵称", () => {
    const note = triggerNoteText("group", { nickname: "张三" }, 2048, 777, SELF_NAME);
    expect(note).toContain("张三(777)");
  });
});
