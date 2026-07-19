import { describe, expect, it } from "bun:test";
import { segmentsToText, formatGroupMessage, parseOutbound, stripReplySegments } from "../../../src/adapters/qq/format";
import type { Segment, SenderInfo } from "../../../src/adapters/qq/types";

const SELF_NAME = "Stella";

// ---- segmentsToText（纯文本提取） ----

describe("segmentsToText", () => {
  it("纯文本直接提取", () => {
    const segs: Segment[] = [
      { type: "text", data: { text: "你好世界" } },
    ];
    expect(segmentsToText(segs, SELF_NAME)).toBe("你好世界");
  });

  it("@自己保留为 @Stella", () => {
    const segs: Segment[] = [
      { type: "at", data: { qq: "123456789", name: SELF_NAME } },
      { type: "text", data: { text: " 帮忙" } },
    ];
    const result = segmentsToText(segs, SELF_NAME);
    expect(result).toContain("@Stella");
    expect(result).toContain("帮忙");
  });

  it("@全体 → @全体成员", () => {
    const segs: Segment[] = [
      { type: "at", data: { qq: "all" } },
    ];
    expect(segmentsToText(segs, SELF_NAME)).toBe("@全体成员");
  });

  it("@别人 → 显示 QQ 号", () => {
    const segs: Segment[] = [
      { type: "at", data: { qq: "111222333" } },
    ];
    const result = segmentsToText(segs, SELF_NAME);
    expect(result).toContain("@111222333");
  });

  it("未知段类型 → [type] 占位", () => {
    const segs: Segment[] = [
      { type: "unknown_type", data: {} },
    ];
    expect(segmentsToText(segs, SELF_NAME)).toBe("[unknown_type]");
  });

  it("引用 → [引用#id]", () => {
    const segs: Segment[] = [
      { type: "reply", data: { id: "999" } },
    ];
    expect(segmentsToText(segs, SELF_NAME)).toBe("[引用#999]");
  });

  it("图片/表情/语音 → 占位符", () => {
    expect(segmentsToText([{ type: "image", data: {} }], SELF_NAME)).toBe("[图片]");
    expect(segmentsToText([{ type: "face", data: {} }], SELF_NAME)).toBe("[表情]");
    expect(segmentsToText([{ type: "record", data: {} }], SELF_NAME)).toBe("[语音]");
  });
});

// ---- 群消息格式化 ----

describe("群消息格式化", () => {
  it("基本格式：群名片优先于昵称", () => {
    const segs: Segment[] = [
      { type: "text", data: { text: "大家好啊" } },
    ];
    const sender: SenderInfo = { nickname: "小明", card: "小明(开发)" };
    const result = formatGroupMessage(segs, sender, 1024, 90000, 123456, SELF_NAME);
    expect(result).toContain("[#1024");
    expect(result).toContain("小明(开发)(123456)");
    expect(result).toContain("大家好啊");
  });

  it("无群名片时用昵称", () => {
    const segs: Segment[] = [
      { type: "text", data: { text: "你好" } },
    ];
    const sender: SenderInfo = { nickname: "小红" };
    const result = formatGroupMessage(segs, sender, 1, 0, 789, SELF_NAME);
    expect(result).toContain("小红(789)");
    expect(result).toContain("[#1");
  });

  it("@Stella → 行内保留 @Stella", () => {
    const segs: Segment[] = [
      { type: "at", data: { qq: "123456789", name: SELF_NAME } },
      { type: "text", data: { text: " 帮我查一下" } },
    ];
    const sender: SenderInfo = { nickname: "测试用户" };
    const result = formatGroupMessage(segs, sender, 5, 0, 111, SELF_NAME);
    expect(result).toContain("@Stella");
    expect(result).toContain("帮我查一下");
  });

  it("@别人 → @QQ号 格式", () => {
    const segs: Segment[] = [
      { type: "at", data: { qq: "111222333" } },
      { type: "text", data: { text: " 来看这个" } },
    ];
    const sender: SenderInfo = { nickname: "发言人" };
    const result = formatGroupMessage(segs, sender, 10, 0, 555, SELF_NAME);
    expect(result).toContain("@111222333");
  });

  it("@全体 → @全体成员", () => {
    const segs: Segment[] = [
      { type: "at", data: { qq: "all" } },
      { type: "text", data: { text: " 通知" } },
    ];
    const sender: SenderInfo = { nickname: "管理员" };
    const result = formatGroupMessage(segs, sender, 20, 0, 999, SELF_NAME);
    expect(result).toContain("@全体成员");
  });

  it("图片 → [图片] 占位", () => {
    const segs: Segment[] = [
      { type: "image", data: { url: "http://example.com/pic.jpg" } },
    ];
    const sender: SenderInfo = { nickname: "图控" };
    const result = formatGroupMessage(segs, sender, 30, 0, 111, SELF_NAME);
    expect(result).toContain("[图片]");
  });

  it("表情 → [表情] 占位", () => {
    const segs: Segment[] = [
      { type: "face", data: { id: "178" } },
    ];
    const sender: SenderInfo = { nickname: "表情帝" };
    const result = formatGroupMessage(segs, sender, 40, 0, 222, SELF_NAME);
    expect(result).toContain("[表情]");
  });

  it("语音 → [语音] 占位", () => {
    const segs: Segment[] = [
      { type: "record", data: {} },
    ];
    const sender: SenderInfo = { nickname: "语音控" };
    const result = formatGroupMessage(segs, sender, 50, 0, 333, SELF_NAME);
    expect(result).toContain("[语音]");
  });

  it("引用 → [引用#id]", () => {
    const segs: Segment[] = [
      { type: "reply", data: { id: "999" } },
      { type: "text", data: { text: "说得对" } },
    ];
    const sender: SenderInfo = { nickname: "引用者" };
    const result = formatGroupMessage(segs, sender, 60, 0, 444, SELF_NAME);
    expect(result).toContain("[引用#999]");
    expect(result).toContain("说得对");
  });

  it("混合消息：文本 + @Stella + 图片 + @别人", () => {
    const segs: Segment[] = [
      { type: "text", data: { text: "看看这个 " } },
      { type: "at", data: { qq: "123456789", name: SELF_NAME } },
      { type: "text", data: { text: " " } },
      { type: "image", data: {} },
      { type: "text", data: { text: " " } },
      { type: "at", data: { qq: "987654321" } },
    ];
    const sender: SenderInfo = { nickname: "混合用户", card: "混合卡" };
    const result = formatGroupMessage(segs, sender, 100, 0, 888, SELF_NAME);
    expect(result).toContain("@Stella");
    expect(result).toContain("[图片]");
    expect(result).toContain("@987654321");
    expect(result).toContain("混合卡(888)");
  });

  it("time 格式化为 HH:MM", () => {
    const segs: Segment[] = [{ type: "text", data: { text: "test" } }];
    const sender: SenderInfo = { nickname: "测试" };
    const result = formatGroupMessage(segs, sender, 1, 3600 * 8 + 60 * 30, 777, SELF_NAME);
    expect(result).toMatch(/\d{2}:\d{2}/);
  });
});

// ---- 输出解析 parseOutbound ----

describe("输出解析 parseOutbound", () => {
  it("纯文本输出", () => {
    const result = parseOutbound("你好，有什么可以帮你的？", "private");
    expect(result).toEqual([
      { type: "text", data: { text: "你好，有什么可以帮你的？" } },
    ]);
  });

  it("行首 [reply:#id] → reply 段", () => {
    const result = parseOutbound("[reply:#1024] 收到，已处理", "private");
    expect(result[0]).toEqual({ type: "reply", data: { id: "1024" } });
    expect(result[1]).toEqual({ type: "text", data: { text: "收到，已处理" } });
  });

  it("正文 @QQ号（5+ 位）→ at 段", () => {
    const result = parseOutbound("你好 @123456 和 @789012 都来看看", "private");
    expect(result).toEqual([
      { type: "text", data: { text: "你好 " } },
      { type: "at", data: { qq: "123456" } },
      { type: "text", data: { text: " 和 " } },
      { type: "at", data: { qq: "789012" } },
      { type: "text", data: { text: " 都来看看" } },
    ]);
  });

  it("小于 5 位数字不识别为 @", () => {
    const result = parseOutbound("@1234 不是 QQ 号", "private");
    expect(result).toEqual([
      { type: "text", data: { text: "@1234 不是 QQ 号" } },
    ]);
  });

  it("群聊自动在开头 at 当前说话人", () => {
    const result = parseOutbound("大家好啊", "group", "111222333");
    expect(result).toEqual([
      { type: "at", data: { qq: "111222333" } },
      { type: "text", data: { text: "大家好啊" } },
    ]);
  });

  it("私聊不加 at", () => {
    const result = parseOutbound("你好", "private");
    expect(result.length).toBe(1);
    expect(result[0]).toEqual({ type: "text", data: { text: "你好" } });
  });

  it("reply + at 组合（群聊）", () => {
    const result = parseOutbound("[reply:#555] @999888 说得对", "group", "111");
    expect(result[0]).toEqual({ type: "reply", data: { id: "555" } });
    expect(result[1]).toEqual({ type: "at", data: { qq: "111" } });
    expect(result[2]).toEqual({ type: "at", data: { qq: "999888" } });
    expect(result[3]).toEqual({ type: "text", data: { text: " 说得对" } });
  });

  it("空输出返回空 text 段（群聊保留 at）", () => {
    const result = parseOutbound("", "group", "111");
    expect(result).toEqual([
      { type: "at", data: { qq: "111" } },
    ]);
  });

  it("[reply:#id] 不在行首时不解析", () => {
    const result = parseOutbound("这里有个 [reply:#1024] 标记", "private");
    expect(result).toEqual([
      { type: "text", data: { text: "这里有个 [reply:#1024] 标记" } },
    ]);
  });
});

// ---- 降级路径 ----

describe("降级路径", () => {
  it("stripReplySegments 去掉 reply 段", () => {
    const segs: Segment[] = [
      { type: "reply", data: { id: "999" } },
      { type: "at", data: { qq: "111" } },
      { type: "text", data: { text: " 回复内容" } },
    ];
    const stripped = stripReplySegments(segs);
    expect(stripped).toEqual([
      { type: "at", data: { qq: "111" } },
      { type: "text", data: { text: " 回复内容" } },
    ]);
  });

  it("stripReplySegments 无 reply 段保持不变", () => {
    const segs: Segment[] = [
      { type: "text", data: { text: "纯文本" } },
    ];
    expect(stripReplySegments(segs)).toEqual(segs);
  });
});
