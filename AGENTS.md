# Agent

## 设计准则

- 面向对象设计/编程，遵循 SOLID 原则
- 若无必要，勿增实体：代码保持极简，减少非必要抽象，减少非必要测试用例
- 遵循业界通用设计方案，若需使用非标方案，必须获得用户许可

### 开发规范

- 注释使用中文，仅描述当前事实，禁止记录过去和决策内容
- 日志使用英文，所有操作都必须记录日志，所有错误都必须记录日志，必要时需记录堆栈
- 代码使用英文

## Agent skills

### Issue tracker

本地 Markdown：issue 以文件形式存放在 `.scratch/`。参见 `docs/agents/issue-tracker.md`。

### Triage labels

五个角色标签：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。参见 `docs/agents/triage-labels.md`。

### Domain docs

单一上下文：根目录 `CONTEXT.md` + `docs/adr/`。参见 `docs/agents/domain.md`。
