# 项目 TODO（跨会话续作指南）

> 本文档汇总各 feat 的未完成事项，供后续会话快速恢复上下文。
> 项目根目录：`Gopher-Official-v1/`（module: ai-start，Go 1.25 + Gin + 手写多 Agent 总线）
> 阅读顺序：README.md（理念与问答）→ docs/rule.md（规范）→ docs/featXXX/（各需求文档）
> 运行：Docker 起 kaogong-mysql/kaogong-pgvector/kaogong-redis → `go run ./cmd/server` → http://localhost:8080

## 已完成（勿重做）

| Feat | 内容 |
| --- | --- |
| feat001 | 条件解析（文本+毕业证融合、置信度校验、档案一用户一档）、岗位查询（六维筛选）、收藏、登录注册、前端页面、2026 国考 20714 岗位 + 7300 条进面分数线导入 |
| feat002 | 多轮对话：意图路由（router）+ ReAct（advisor）+ 工具白名单 + SSE 流式 + 身份跨总线透传 |
| feat003 | 记忆体系：三层作用域（session 共享/user 共享/agent 隔离）、短期 Redis 缓冲、save_insight 自主沉淀、规则触发写入 |
| feat004 | 选岗推荐：researcher/analyzer/strategist/responder 流水线 + run_advise 工具 + reports 表 + 报告接口 |

## 待办（按建议优先级）

### P1 业务功能

- [ ] **面试考官 Agent**（feat002 P3）：已编码，复用现有多轮追问入口，支持练习/模拟、追问点评；待真实模型交互验收
- [ ] **省考职位表导入**：目前只有国考 2026。用 `scripts/import_positions`（支持 .xls/.xlsx）导广东省考数据
- [ ] **笔试中枢**（feat002 P4）：行测诊断 + 申论批改。⚠️ 阻塞项：题库数据源未定（TODO-1：外部 API / 真题整理 / LLM 出题兜底）
- [ ] **无领导小组**（feat002 P5）：多角色扮演回合制编排，最复杂（TODO-3 待细化）

### P2 数据质量（feat001 回滚待重做，先讨论语义再动手）

- [ ] 学历"仅限"语义：`仅限本科` 是等于语义（硕士不能报），当前下限比较会误放行
- [ ] 未识别专业误放行：大类归一失败的岗位被当"不限"（如 0827 核科学与技术）
- [ ] 专业目录完善：2 位门类码（07理学/08工学）、研究生一级学科前缀不全
- [ ] 户籍维度建模：部分岗位备注含户籍限制，未结构化

### P3 记忆优化（feat003 三件套）

- [ ] 召回闸门：短追问/明显承接上文时跳过向量召回
- [ ] 会话压缩摘要：旧消息摘要沉淀长期记忆（配合 save_insight）
- [ ] 记忆衰减评分：相似度 × 新鲜度综合排序

### P4 技术债

- [ ] SSE 心跳（15s `: ping`，防代理断连）
- [ ] Reviewer 防幻觉校验节点（推荐结论引用真实性校验）
- [ ] Parser 评测集回归测试
- [ ] 日志文件轮转（lumberjack）
- [ ] 用户名格式校验
- [ ] 节点多副本/flow 并行组（等压测数据）
- [ ] RabbitMQ 任务队列（多实例/持久化时再引入，见 README Bus vs MQ 问答）

## 关键上下文（新会话必读）

- **文档规范**（docs/rule.md）：代码必加注释；企业级目录规范；每需求在 docs/ 下建目录（PRD + 技术文档）；过程文档（`*-todo.md`/`*-issues.md`）不入库
- **密钥**：`configs/config.yaml`（gitignore，不入库）含 LLM_API_KEY（智谱）/ EMBEDDING_API_KEY（硅基流动）；丢失时从 `configs/config.example.yaml` 复制重填
- **测试注意**：Git Bash 里 curl 中文参数会被转成 `??`，中文测试内容用 `--data-binary @文件` 传
- **架构认知**：README 的六组问答（架构理念/能力边界/存储选型/多 Agent 设计/流式机制/核心链路速查）是全部设计决策的沉淀
