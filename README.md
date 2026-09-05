# 公考参谋部（ai-start）

基于多 Agent 协作的智能公考选岗分析系统。详见 [产品方案 PRD](docs/feat001/choose-job20260905.md)。

## 目录结构

```plain
├── cmd/
│   └── server/        # 服务启动入口
├── internal/          # 内部业务代码（不对外暴露）
│   ├── agent/         # 多 Agent 核心编排层
│   ├── api/           # HTTP 接口层（Gin 框架）
│   ├── config/        # 配置加载
│   ├── llm/           # 大模型客户端封装
│   ├── ocr/           # OCR 图像识别
│   └── rag/           # RAG 知识库检索
├── configs/           # 配置文件
└── docs/              # 项目文档
    ├── rule.md        # 开发规范
    └── feat001/       # 各需求对应的 PRD + 技术文档
```

## 开发规范

见 [docs/rule.md](docs/rule.md)。要点：代码必加注释；每个需求在 `docs/` 下建有对应目录，包含需求文档与技术文档，技术文档随开发同步更新。

## 运行

```bash
go run ./cmd/server
```
