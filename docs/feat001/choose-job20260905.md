# 产品需求文档（PRD）
# 智能选岗参谋系统（Position Advisor Agent）

> 版本：v1.0 MVP  
> 形态：纯后端API服务（Go）  
> 日期：2026-09-05  
> 文档状态：待开发

---

## 一、产品概述

### 1.1 核心目标

构建一个基于多Agent协作的智能选岗后端服务。用户通过API提交个人条件（文字或职位表截图），系统通过OCR+LLM解析条件，检索职位数据库，运用竞争策略分析，返回"冲-稳-保"三档岗位推荐报告，并支持多轮追问。

### 1.2 用户交互模式（API层面）

由于无UI，所有交互抽象为状态机驱动的API调用：

- **模式A（直接提交）**：用户已明确条件，直接POST结构化数据
- **模式B（OCR上传）**：用户上传职位表截图，后端OCR解析，低置信度时返回 `need_confirm` 状态等待用户确认
- **模式C（多轮对话）**：基于已有推荐报告，用户继续追问

### 1.3 技术架构（产品视角）

```
用户请求（HTTP API）
↓
API Gateway（路由+限流）
↓
Supervisor Agent（调度中枢）
├── Parser Agent（条件解析：OCR/自然语言 → 结构化JSON）
├── Researcher Agent（职位检索：硬条件匹配+专业大类映射）
├── Analyzer Agent（竞争分析：调取历年分数线/报录比）
├── Strategist Agent（策略生成：冲稳保分类+推荐理由）
└── Responder Agent（报告生成：0基础版/进阶版格式化输出）
↓
RAG知识库（pgvector：职位表+分数线+专业目录+政策文件）
MCP工具层（可选预留：外部数据源查询接口）
```

---

## 二、核心功能模块

### 2.1 模块1：条件解析引擎（Parser Agent）

**功能**：将用户输入（图片/文字）转换为标准化的UserProfile结构。

**输入类型**：
- **结构化直接提交**：用户已填好表单，直接提交JSON
- **自然语言**："我是计算机本科，党员，想考广州"
- **图片OCR**：用户上传职位表截图或毕业证/学位证照片

**OCR方案C（降级策略）**：
- 后端先进行OCR识别，提取所有可见字段
- 计算每个字段的置信度（0-1）
- **规则**：
  - 所有字段置信度 ≥ 0.85：直接解析，进入下一步
  - 任一字段置信度 < 0.85：API返回 `status: "need_confirm"`，附带 `uncertain_fields` 列表

```json
{
  "field": "major",
  "raw_text": "计箅机科?技术",
  "confidence": 0.62,
  "suggested_value": "计算机科学与技术",
  "reason": "OCR识别到疑似错别字，根据上下文推断"
}
```

### 2.2 模块2：职位检索引擎（Researcher Agent）

**功能**：根据硬条件从数据库筛选匹配职位。

**硬条件匹配规则**：
- **学历**：用户学历 ≥ 岗位要求学历（本科可报"本科"和"不限"，不能报"硕士"）
- **专业**：精确匹配 或 专业大类匹配（见3.2专业目录映射）
- **政治面貌**：用户面貌满足岗位要求（党员可报"党员""不限"，不能报"共青团员仅限"）
- **应届身份**：用户身份满足岗位要求
- **基层经验**：用户年限 ≥ 岗位要求年限
- **地域**：用户选择省份与岗位省份匹配
- **户籍**：如岗位限本地户籍，需用户确认是否满足

**输出**：初步匹配职位ID列表（可能50-200个）

### 2.3 模块3：竞争分析引擎（Analyzer Agent）

**功能**：对初步匹配的每个职位进行竞争烈度分析。

**分析维度**：
- 历年进面分数线（最近2-3年）
- 历年报录比（报名截止日数据，MVP可用报名中期数据）
- 限制条件强度评分（限制越多，竞争通常越小）
- 岗位热度（是否热门单位/热门城市）

**输出**：每个职位的 `competition_score`（0-100，100为最难）

### 2.4 模块4：策略生成引擎（Strategist Agent）

**功能**：将分析结果转化为用户可执行的选岗策略。

**策略分类**：
- **冲（1个）**：岗位好但竞争激烈，适合实力强或有特殊优势（如党员+应届双重限制）的用户
- **稳（2个）**：条件匹配度高，历年数据稳定，上岸概率适中
- **保（1个）**：限制条件极多，竞争小，确保有面试可上

**策略逻辑**：
- 考虑用户画像中的"优势标签"（如党员、应届、硕士）
- 优先推荐能最大化利用优势标签的岗位
- 排除明显不匹配或风险过高的岗位（如用户模考分远低于历年进面分）

### 2.5 模块5：报告生成引擎（Responder Agent）

**功能**：将策略结果格式化为最终报告。

**双模式输出**：
- `mode: "beginner"`（0基础版）：详细解释每个概念、每句推荐理由都带教育属性
- `mode: "advanced"`（进阶版）：精简数据、专业术语、直接给结论

---

## 三、API接口设计

### 3.1 接口总览

| 接口 | 方法 | 路径 | 说明 |
|------|------|------|------|
| 条件解析 | POST | `/api/v1/parse` | 提交文字/图片，解析为结构化条件 |
| 确认修正 | POST | `/api/v1/parse/confirm` | 对OCR低置信度字段进行人工确认 |
| 选岗推荐 | POST | `/api/v1/advise` | 提交结构化条件，获取选岗报告 |
| 多轮追问 | POST | `/api/v1/chat` | 基于已有报告进行追问 |
| 报告查询 | GET | `/api/v1/reports/{report_id}` | 查询历史报告 |
| 收藏岗位 | POST | `/api/v1/favorites` | 收藏心仪岗位 |

### 3.2 接口详情

#### 3.2.1 条件解析接口 POST /api/v1/parse

**请求体**：
```json
{
  "input_type": "text",
  "content": "我是计算机本科，党员，想考广州",
  "image_base64": "",
  "mode": "beginner"
}
```

**响应体（直接成功）**：
```json
{
  "status": "success",
  "data": {
    "profile": {
      "education": "本科",
      "major": "计算机科学与技术",
      "major_category": "计算机类",
      "political_status": "中共党员",
      "is_fresh_graduate": true,
      "target_provinces": ["广东"],
      "work_experience_years": 0,
      "gender": "",
      "age": 0,
      "other_requirements": []
    },
    "parsed_from": "text",
    "confidence": 0.95
  }
}
```

**响应体（需要确认 - OCR方案C）**：
```json
{
  "status": "need_confirm",
  "data": {
    "profile_partial": {
      "education": "本科",
      "major": "",
      "target_provinces": ["广东"]
    },
    "uncertain_fields": [
      {
        "field": "major",
        "raw_text": "计箅机科?技术",
        "confidence": 0.62,
        "suggested_value": "计算机科学与技术",
        "reason": "OCR识别到疑似错别字，根据上下文推断"
      },
      {
        "field": "political_status",
        "raw_text": "",
        "confidence": 0.0,
        "suggested_value": "",
        "reason": "图片中未找到政治面貌相关信息"
      }
    ],
    "session_id": "parse_abc123"
  }
}
```

#### 3.2.2 确认修正接口 POST /api/v1/parse/confirm

**请求体**：
```json
{
  "session_id": "parse_abc123",
  "confirmed_fields": {
    "major": "计算机科学与技术",
    "political_status": "中共党员"
  }
}
```

**响应体**：同3.2.1的success格式

#### 3.2.3 选岗推荐接口 POST /api/v1/advise

**请求体**：
```json
{
  "profile": {
    "education": "本科",
    "major": "计算机科学与技术",
    "major_category": "计算机类",
    "political_status": "中共党员",
    "is_fresh_graduate": true,
    "target_provinces": ["广东", "国家"],
    "work_experience_years": 0,
    "gender": "男",
    "age": 22,
    "other_requirements": ["不接受经常出差"]
  },
  "mode": "beginner",
  "exam_type": "省考",
  "session_id": "optional_existing_session"
}
```

**响应体**：
```json
{
  "status": "success",
  "data": {
    "report_id": "rpt_20260905_001",
    "session_id": "sess_xyz789",
    "summary": {
      "total_matched": 47,
      "after_filter": 12,
      "recommendations": {
        "rush": 1,
        "stable": 2,
        "safe": 1
      }
    },
    "recommendations": [
      {
        "category": "rush",
        "position": {
          "id": "pos_001",
          "name": "一级行政执法员",
          "department": "广州市税务局信息中心",
          "province": "广东",
          "city": "广州",
          "education_req": "本科及以上",
          "major_req": "计算机类",
          "political_req": "中共党员",
          "fresh_graduate_req": true,
          "other_restrictions": ["限男性"]
        },
        "analysis": {
          "competition_score": 78,
          "historical_scores": {
            "2025": 142,
            "2024": 138
          },
          "estimated_applicants_ratio": "1:80",
          "reasoning": "你的党员+应届双重身份完美匹配，但历年进面分较高，建议模考135+再冲"
        },
        "advice": {
          "beginner": "这个岗位属于税务局，待遇较好。要求计算机类专业、中共党员、应届毕业生，你的条件完全吻合。但注意，2024年进面分数线为138分，属于高分岗。如果你目前的模考成绩稳定在135分以上，可以冲刺。",
          "advanced": "匹配度95%。双重限制（党员+应届）筛除大量竞争者，但进面分138说明考生质量高。建议作为冲档，需配合行测75+实力。"
        }
      }
    ],
    "risk_alerts": [
      {
        "type": "warning",
        "message": "你收藏的'广东省发改委信息化岗'不限应届，社会考生实力强，建议放弃",
        "related_position_id": "pos_xxx"
      }
    ],
    "next_steps": [
      "已为你生成能力雷达图，可在笔试备考阶段调用",
      "报名期间可每日监控岗位竞争数据"
    ]
  }
}
```

#### 3.2.4 多轮追问接口 POST /api/v1/chat

**请求体**：
```json
{
  "session_id": "sess_xyz789",
  "question": "A岗和B岗哪个待遇更好？",
  "context_type": "comparison",
  "position_ids": ["pos_001", "pos_002"]
}
```

**响应体**：
```json
{
  "status": "success",
  "data": {
    "answer": "从公开信息看，广州市税务局（A岗）待遇通常优于区县级市场监管局（B岗）。但需注意：A岗备注'需24小时值班'，实际工作强度可能更高。",
    "citations": [
      {
        "source": "公务员待遇公开数据",
        "url": "",
        "confidence": "medium"
      }
    ],
    "suggested_followups": [
      "这两个岗位的工作强度如何？",
      "如果我的目标是上岸，应该优先选哪个？"
    ]
  }
}
```

---

## 四、Agent交互时序图

### 4.1 主流程：选岗推荐（成功路径）

```
用户                    API层              Supervisor          Parser          Researcher        Analyzer         Strategist       Responder
 |                       |                    |                |                |               |                |               |
 |--POST /advise-------->|                    |                |                |               |                |               |
 |                       |--dispatch--------->|                |                |               |                |               |
 |                       |                    |--parse-------->|                |               |                |               |
 |                       |                    |<--profile------|                |               |                |               |
 |                       |                    |--research----------------------->|               |                |               |
 |                       |                    |<--matched_ids--------------------|               |                |               |
 |                       |                    |--analyze----------------------------------------->|               |                |
 |                       |                    |<--competition_data--------------------------------|               |                |
 |                       |                    |--strategy------------------------------------------------------->|               |
 |                       |                    |<--strategy_result------------------------------------------------|               |
 |                       |                    |--generate------------------------------------------------------------------------>| 
 |                       |                    |<--report---------------------------------------------------------------------------|
 |                       |<--response--------|                |                |               |                |               |
 |<--JSON报告------------|                    |                |                |               |                |               |
```

### 4.2 异常流程：OCR需要确认（方案C）

```
用户                    API层              Supervisor          Parser(OCR)      用户(再次)
 |                       |                    |                |                |
 |--POST /parse(img)---->|                    |                |                |
 |                       |--dispatch--------->|                |                |
 |                       |                    |--ocr_parse---->|                |
 |                       |                    |<--need_confirm-|                |
 |                       |<--need_confirm----|                |                |
 |<--uncertain_fields----|                    |                |                |
 |                       |                    |                |                |
 |--POST /parse/confirm->|                    |                |                |
 |                       |--dispatch--------->|                |                |
 |                       |                    |--confirm------>|                |
 |                       |                    |<--profile------|                |
 |                       |<--success---------|                |                |
 |<--profile JSON--------|                    |                |                |
```

### 4.3 追问流程

```
用户                    API层              Supervisor          Responder        RAG检索
 |                       |                    |                |                |
 |--POST /chat--------->|                    |                |                |
 |                       |--dispatch--------->|                |                |
 |                       |                    |--load_history  |                |
 |                       |                    |--intent_parse  |                |
 |                       |                    |--retrieve---------------------->|
 |                       |                    |<--docs-------------------------|
 |                       |                    |--generate---->|                |
 |                       |                    |<--answer------|                |
 |                       |<--response--------|                |                |
 |<--answer JSON---------|                    |                |                |
```

---

## 五、数据模型定义

### 5.1 职位表 positions

```sql
CREATE TABLE positions (
    id VARCHAR(64) PRIMARY KEY,
    exam_type VARCHAR(20) NOT NULL, -- 国考 | 省考
    province VARCHAR(50) NOT NULL,
    city VARCHAR(50),
    department VARCHAR(200) NOT NULL, -- 招录单位
    position_name VARCHAR(200) NOT NULL, -- 岗位名称
    position_code VARCHAR(100), -- 职位代码
    
    -- 硬条件
    education_req VARCHAR(50), -- 大专 | 本科 | 硕士 | 博士 | 不限
    major_req_exact TEXT, -- 精确专业要求，逗号分隔
    major_req_category VARCHAR(100), -- 专业大类，如"计算机类"
    political_req VARCHAR(50), -- 群众 | 共青团员 | 中共党员 | 不限
    is_fresh_graduate_req BOOLEAN, -- 是否限应届
    work_experience_years_req INT DEFAULT 0, -- 基层工作年限要求
    gender_req VARCHAR(10), -- 男 | 女 | 不限
    age_limit INT, -- 年龄上限
    other_restrictions TEXT, -- 其他限制条件（JSON数组）
    remarks TEXT, -- 岗位备注
    
    -- 竞争数据
    historical_score_2025 INT,
    historical_score_2024 INT,
    historical_score_2023 INT,
    applicant_ratio_2025 VARCHAR(20), -- 如 "1:85"
    applicant_ratio_2024 VARCHAR(20),
    
    -- 元数据
    source_url TEXT,
    data_version VARCHAR(20),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 向量检索用（职位描述、备注的embedding）
CREATE TABLE position_embeddings (
    id SERIAL PRIMARY KEY,
    position_id VARCHAR(64) REFERENCES positions(id),
    embedding_type VARCHAR(50), -- description | remarks | combined
    embedding vector(1536),
    content_text TEXT
);
```

### 5.2 专业目录映射 major_directory

```sql
CREATE TABLE major_directory (
    id SERIAL PRIMARY KEY,
    major_name VARCHAR(100) NOT NULL, -- 计算机科学与技术
    major_code VARCHAR(20), -- 080901
    major_category VARCHAR(100) NOT NULL, -- 计算机类
    major_category_code VARCHAR(20), -- 0809
    discipline VARCHAR(100), -- 工学
    level VARCHAR(20), -- 本科 | 研究生
    aliases TEXT -- JSON数组，别名如["计科","CS"]
);
```

### 5.3 用户查询会话 user_sessions

```sql
CREATE TABLE user_sessions (
    id VARCHAR(64) PRIMARY KEY,
    user_device_id VARCHAR(100), -- MVP无登录，用设备ID
    mode VARCHAR(20) NOT NULL, -- beginner | advanced
    status VARCHAR(20) NOT NULL, -- parsing | confirming | advising | completed
    profile JSONB, -- 用户条件快照
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### 5.4 推荐报告 reports

```sql
CREATE TABLE reports (
    id VARCHAR(64) PRIMARY KEY,
    session_id VARCHAR(64) REFERENCES user_sessions(id),
    profile_used JSONB NOT NULL,
    recommendations JSONB NOT NULL, -- 冲稳保结果数组
    risk_alerts JSONB,
    conversation_history JSONB DEFAULT '[]', -- 多轮对话记录
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### 5.5 用户收藏 favorites

```sql
CREATE TABLE favorites (
    id SERIAL PRIMARY KEY,
    user_device_id VARCHAR(100),
    position_id VARCHAR(64) REFERENCES positions(id),
    report_id VARCHAR(64) REFERENCES reports(id),
    category VARCHAR(20), -- rush | stable | safe | custom
    notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

---

## 六、Prompt框架设计

### 6.1 System Prompt 分层策略

**全局规则（所有Agent共用）**：
```
1. 你是公务员考试选岗参谋，回答必须基于提供的职位数据和政策文件，禁止编造不存在的岗位或分数线。
2. 涉及政策解读时，必须标注信息来源（如"根据2024年广东省考职位表"）。
3. 不确定的信息明确告知用户"该信息暂未收录"，不要猜测。
4. 所有分析必须考虑用户的"优势标签"（如党员、应届、硕士），并指出如何利用这些优势。
```

**模式差异（beginner vs advanced）**：

| 维度 | Beginner Mode | Advanced Mode |
|------|---------------|---------------|
| 语言风格 | 亲切、解释概念、用比喻 | 专业、直接、术语 |
| 输出结构 | 分步骤引导，每步有"为什么" | 结论先行，数据支撑 |
| 专业术语 | 必须解释（如"进面分"→"进入面试的最低分数线"） | 直接使用，不解释 |
| 建议颗粒度 | 具体到"你现在该做什么" | 只给策略，执行层面不展开 |
| 风险提示 | 详细解释风险原因 | 一句话警示 |

### 6.2 Parser Agent Prompt

```
角色：你是条件解析专家，负责将用户的自然语言或OCR结果转换为结构化的考公选岗条件。

输入处理规则：
1. 学历映射：
   - "本科"、"大学本科"、" Bachelor" → 本科
   - "研究生"、"硕士"、"Master" → 硕士
   - 模糊时询问确认

2. 专业映射（关键！）：
   - 先尝试精确匹配专业目录
   - 未精确匹配时，尝试匹配别名（如"计科"→"计算机科学与技术"）
   - 仍未匹配时，返回need_confirm，建议用户从列表中选择

3. 政治面貌映射：
   - "党员"、"中共党员"、"正式党员" → 中共党员
   - "预备党员" → 预备党员（注意：预备党员可报限党员岗，但某些特殊岗位可能要求正式党员）
   - "团员"、"共青团员" → 共青团员

4. 应届身份判断：
   - 明确说"应届"、"今年毕业"、"2026届" → true
   - 说"毕业2年"、"工作3年" → false
   - 未提及且无法推断 → 返回need_confirm

5. 省份识别：
   - "广东"、"广东省"、"广州" → 广东
   - "国家"、"国考"、"中央" → 国家（国考）
   - 支持多省份（如"广东或湖南"）

输出格式：严格按UserProfile JSON结构输出，不确定字段标记为null并加入uncertain_fields列表。
```

### 6.3 Strategist Agent Prompt

```
角色：你是选岗策略专家，负责根据用户条件和职位竞争数据，生成"冲-稳-保"三档推荐。

策略原则：
1. 冲档（1个）：
   - 岗位质量高（热门单位、核心部门）
   - 用户条件完全匹配
   - 但竞争激烈（历年进面分高或报录比高）
   - 用户有独特优势可对冲竞争（如党员+应届双重限制）

2. 稳档（2个）：
   - 用户条件匹配度高（≥80%）
   - 历年数据稳定（分数线波动≤5分）
   - 限制条件较多（专业+政治面貌+应届），竞争可控
   - 用户模考成绩与历年进面分接近（±5分）

3. 保底档（1个）：
   - 限制条件极多（≥3个硬性限制）
   - 历年竞争比低（1:30以下）
   - 确保用户"有岗可上"
   - 可能是偏远地区或非热门单位

排除规则：
- 用户条件明显不满足的（如学历不够、专业不符）→ 直接排除
- 用户明确排斥的（如备注"需经常出差"且用户已说明不接受）→ 排除
- 历年进面分远高于用户模考分（差距>20分）且无明显优势 → 不推荐为冲档

输出要求：
- 每个推荐必须包含：岗位信息 + 匹配度评分 + 推荐理由 + 风险提示
- 推荐理由必须引用具体数据（如"2024年进面分138分"）
- 必须指出用户的"优势标签"如何被利用
```

---

## 七、MVP边界与验收标准

### 7.1 In Scope（必须做）

| 功能 | 优先级 | 验收标准 |
|------|--------|----------|
| 条件解析（文本+结构化） | P0 | 能正确解析"我是计算机本科，党员，想考广州" |
| 条件解析（OCR+确认流程） | P0 | 低置信度时返回need_confirm，用户确认后成功解析 |
| 硬条件匹配 | P0 | 学历/专业/政治面貌/应届/基层经验/地域六维度过滤正确 |
| 专业大类映射 | P0 | "软件工程"能正确映射到"计算机类" |
| 冲稳保推荐 | P0 | 输出包含三档，每档至少1个岗位，有推荐理由 |
| 双模式输出 | P0 | beginner模式字数≥500，advanced模式字数≤200 |
| 多轮追问 | P1 | 支持岗位对比、专业确认、待遇咨询3类追问 |
| 报告收藏 | P1 | 用户可收藏岗位，能查询收藏列表 |

### 7.2 Out of Scope（明确不做）

- ❌ 全国所有省份（MVP先做1-2省+国考）
- ❌ 实时报名数据监控（官方不公开，用历史数据+mock）
- ❌ 手写笔记OCR（仅支持官方印刷体职位表）
- ❌ 行测/申论/面试功能（后续feat迭代）
- ❌ 用户登录/支付系统（MVP用设备ID标识）
- ❌ 前端UI（纯后端API服务）

### 7.3 性能指标

| 指标 | 目标 | 测试方法 |
|------|------|----------|
| 接口响应P99 | < 2s（不含LLM流式生成） | 压测50并发 |
| 首包延迟（SSE） | < 500ms | 前端计时 |
| OCR识别耗时 | < 1s | 单张截图 |
| Agent总迭代 | ≤ 5轮 | 日志监控 |

---

## 八、风险与假设

| 风险 | 影响 | 应对 |
|------|------|------|
| OCR准确率不足 | 用户条件识别错误，导致推荐偏差 | 置信度<0.85时强制降级为表单输入 |
| LLM幻觉（胡说专业匹配规则） | 用户信错信息，报岗失败 | 专业匹配必须走规则引擎，LLM只负责解释，不参与判定 |
| 历史数据不全 | 竞争分析不准确 | MVP明确告知用户"基于历史数据，仅供参考" |
| 政策变动 | 2026年考公政策变化 | 数据版本化管理，支持快速更新 |

---

## 九、变更记录

| 日期 | 版本 | 变更内容 | 变更人 |
|------|------|----------|--------|
| 2026-09-05 | v1.0 | 初始版本创建 | 产品经理 |

