# Stella

Stella 是一个 LLM 聊天 agent 框架：核心 agent 循环驱动模型与工具交互，多种 transport（onebot、TUI 等）接入会话。

## Language

**Daemon**:
以 OS 服务（Windows Service / systemd）方式后台托管的 stella 实例，由 `stella daemon` 子命令管理。
_Avoid_: server、后台进程

**Tool**:
直接暴露给模型调用的工具，随请求下发给模型。
_Avoid_: 插件

**DynTool**:
不直接下发给模型的工具，通过 dyn 网关按需查看与执行，依靠系统提示词中的目录引导。
_Avoid_: extension tool、扩展工具

**Extension**:
仅 Name 必需，其余能力（Tool、DynTool、提示词 section、事件监听）按需实现小接口。
_Avoid_: 插件

**Content**:
一条消息的内容，由有序的 ContentPart 序列构成。

**ContentPart**:
单个内容块，类型为 text（文本）或 image（图片）。

**Description**:
工具的完整说明，随请求的 tools 参数下发。
_Avoid_: 与 Summary 混用

**Summary**:
工具的一行简讯，仅用于系统提示词中的工具目录。
_Avoid_: 与 Description 混用

**Event**:
agent 生命周期内发生的只读通知，扩展可监听但不可干预；干预类需求属于钩子，不属于事件。

**账单**:
一笔收支记录：类型（支出/收入）、类别、金额（分）、备注、日期。

**类别**:
账单的分类标签，全局一套，支持增删改。

**周期规则**:
定义重复收支的规则，查询时物化为账单。
_Avoid_: 定时任务

**退款**:
类别为「退款」的收入账单，备注引用原账单。
_Avoid_: 冲销
