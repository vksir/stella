# Agent

## 设计准则

- 面向对象设计/编程，遵循 SOLID 原则
- 若无必要，勿增实体：代码保持极简，减少非必要抽象，禁止擅自新增测试用例
- 遵循业界通用设计方案，若需使用非标方案，必须获得用户许可

### 开发规范

- 永远做最小化修改
- 注释使用中文，必须精简，且仅描述当前事实，禁止记录过去和决策内容
- 日志使用英文
- 代码使用英文

## Agent skills

### Issue tracker

本地 Markdown：issue 和 spec 存放在 `.scratch/`。参见 `docs/agents/issue-tracker.md`。

### Triage labels

本地 issue 的 `Status:` 使用 `needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。参见 `docs/agents/triage-labels.md`。

### Domain docs

单一上下文：根目录 `CONTEXT.md` + `docs/adr/`。参见 `docs/agents/domain.md`。
