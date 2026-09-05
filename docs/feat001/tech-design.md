# feat001 智能选岗参谋 — 技术设计文档

> 对应需求文档：[choose-job20260905.md](choose-job20260905.md)
> 本文档随开发过程同步更新（见 docs/rule.md 第 3 条）。

## 1. 技术选型

- 语言：Go 1.25
- Web 框架：Gin（`internal/api` 路由与接口层）
- 大模型接入：`internal/llm`（接口抽象，Provider 可插拔）
- OCR：`internal/ocr`（接口抽象）
- 知识库：`internal/rag`（RAG 检索）

## 2. 模块划分

| 模块 | 目录 | 职责 |
| --- | --- | --- |
| 入口 | `cmd/server` | 配置加载、依赖装配、服务启动 |
| 接口层 | `internal/api` | HTTP 请求处理、参数校验 |
| Agent 编排 | `internal/agent` | 条件解析 / 竞争分析 / 策略生成 / 多轮对话 |
| 大模型 | `internal/llm` | 统一对话接口，屏蔽 Provider 差异 |
| OCR | `internal/ocr` | 职位表截图、证件文字识别 |
| 知识库 | `internal/rag` | 职位表、分数线、报录比等数据检索 |

## 3. 接口设计

（待开发过程中补充）

## 4. 数据设计

（待开发过程中补充）
