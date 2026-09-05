# feat001 测试问题记录

> 本文档记录开发测试过程中发现的问题、成因分析与解决方案，随迭代持续更新（见 docs/rule.md 第 3 条）。

## 问题清单

| # | 日期 | 问题 | 状态 |
| --- | --- | --- | --- |
| 1 | 2026-09-05 | LLM 省份映射不合规 | ✅ 已修复（2026-09-05） |
| 2 | 2026-09-05 | LLM 字段幻觉（应届身份） | ✅ 已修复（2026-09-05） |
| 3 | 2026-09-05 | 校验层未清理矛盾/无关不确定项 | ✅ 已修复（2026-09-05） |

## 问题 3：校验层未清理矛盾/无关不确定项

**现象**：输入 `"我是双非本科，软件工程，汕头人考汕头，工作2~3年，中共党员"`，应届身份已被正确解析为"否"，但确认区仍要求确认应届/性别/年龄；且清理后状态仍是 need_confirm（置信度 0.8）。

**成因**：

- LLM 照抄 Prompt 示例惯性，把已确定的字段（应届=否）也列入 uncertain_fields，理由还写"用户未提及"——校验层未检查"字段是否已有确定值"
- 性别/年龄是可选字段，未提及不应阻塞确认流程
- LLM 按 Prompt 惯性自报 confidence=0.8，清理后无不确定项仍被该自报值阻塞（< 0.85 阈值）

**解决方案**（`internal/agent/validate.go`）：

- 新增 `pruneUncertainFields`：字段已有确定值却仍列不确定 → 移除；非核心可选字段（性别/年龄等）→ 移除不阻塞
- 置信度完全由规则计算（不再采信 LLM 自报值），状态判定基于清理后的不确定项数量
- 新增回归测试：`TestValidateContradictoryUncertain`、`TestValidateAllProvincesInvalid`

**验证**：同一输入返回 `status: success`、应届=否、汕头→广东、无多余确认项、`confidence: 1.0`。

---

## 问题 1：LLM 省份映射不合规

**现象**：输入 `"我是计算机本科，党员，想考广州"`，LLM 返回 `target_provinces: ["广州"]`，未按 Prompt 规则将城市映射为省份（应为 `["广东"]`）。

**成因**：

- Prompt 第 5 条映射规则埋在 6 条规则中间，输出 JSON 示例中 `target_provinces: []` 无取值约束说明
- GLM-4-Flash 为 9B 级小模型，指令遵循能力有限；当模型"知识"（广州是城市）与 Prompt 规则（映射为省）冲突时，小模型倾向按字面输出

**解决方案**（双保险）：

- Prompt 层：关键约束前置，补充 few-shot 示例（完整"输入→正确输出"对），明确 `target_provinces` 只接受省份或"国家"，城市必须映射为所属省份
- 代码层：LLM 输出后增加规则归一/校验——复用 `simple_parser.go` 的 `cityProvinceMap` 将城市名归一为省份，非法值降级到 `uncertain_fields` 走确认流程

**边界原则**：LLM 负责理解模糊输入，确定性校验收归代码层兜底。

## 问题 2：LLM 字段幻觉（应届身份）

**现象**：用户输入未提及应届身份，LLM 返回 `is_fresh_graduate: true`，且自报 `confidence: 0.95`。

**成因**：

- **类型设计缺陷（根因）**：`is_fresh_graduate` 为 bool 类型，无"未知"状态。Prompt 要求"未提及→标记不确定"，但 JSON 示例中该字段为 `false`，模型在"必须填布尔值"与"标记不确定"之间选择猜测；考公语境应届生占比高，模型有先验偏好
- LLM 自报置信度不可信，模型并未真正执行自我检查

**解决方案**（三层）：

- 类型层：`is_fresh_graduate` 改为 `*bool`（可空）——未提及即为 null，进入 `uncertain_fields` 走确认流程
- Prompt 层：增加负例约束（"用户未提应届身份时，is_fresh_graduate 必须为 null 并加入 uncertain_fields"）
- 代码层：置信度改为规则计算（按核心字段空缺/不确定数量计算真实置信度），LLM 自报值仅作参考

## 经验总结

两个问题的共同根因：**把"判定"完全交给小模型，缺少规则兜底**。对应 README「AI 能力边界划分原则」——错了代价高的判定（字段合规性、置信度）必须留在代码层，LLM 只做理解和抽取。

## 修复记录（2026-09-05）

**改动清单**：

- `internal/types`：`UserProfile.IsFreshGraduate` 改为 `*bool`（nil = 未提及/未知，消除模型被迫猜测的根因）
- `internal/prompt/prompts_parser.go`：Prompt 重写——硬约束前置（禁止推断/省份白名单/应届三态）+ few-shot 完整示例
- `internal/agent/validate.go`（新增）：规则校验层，LLM 输出统一过 `validateResult`：
  - 省份归一（城市→省份、非法值剔除并标记）
  - 核心字段空缺检查（学历/专业/政治面貌/应届）
  - 置信度规则重算（每个不确定字段 -0.15，LLM 自报值仅作上限）
  - 状态判定强制执行（存在不确定字段 → need_confirm）
- `internal/agent/validate_test.go`（新增）：3 个回归测试覆盖两个问题的修复

**真实模型验证结果**：输入 `"我是计算机本科，党员，想考广州"` 返回
`target_provinces: ["广东"]`、`is_fresh_graduate: null`（进入 uncertain_fields）、`status: need_confirm`、`confidence: 0.7`——两个问题均修复。

## 遗留待办

- [ ] 配置文件中的模型密钥抽离（`config.example.yaml` 入库，真实 `config.yaml` 加入 .gitignore）
