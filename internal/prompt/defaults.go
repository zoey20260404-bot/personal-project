package prompt

// Defaults 内置 Prompt 注册表：Prompt 名 → 常量原文。
// 新增 Prompt：在 prompts_*.go 中定义常量，并在此登记（名字常量见 names.go）。
var Defaults = map[string]string{
	NameParser:            PromptParser,            // 条件解析（文本，默认）
	NameParserDiploma:     PromptParserDiploma,     // 条件解析（毕业证/学位证图片）
	NameParserImageUser:   PromptParserImageUser,   // 条件解析-图片用户提示词
	NameAssistant:         PromptAssistant,         // 通用问答（默认）
	NameAssistantBeginner: PromptAssistantBeginner, // 通用问答-0 基础模式
	NameAssistantAdvanced: PromptAssistantAdvanced, // 通用问答-进阶模式
	NameRouter:            PromptRouter,            // 意图路由（feat002）
	NameInterviewer:       PromptInterviewer,       // 结构化面试考官（feat002 P3）
	NameAdvisor:           PromptAdvisor,           // 选岗参谋（feat002）
	NameAnalyzer:          PromptAnalyzer,          // 竞争分析（feat004）
	NameStrategist:        PromptStrategist,        // 冲稳保策略（feat004）
	NameResponder:         PromptResponder,         // 报告生成（feat004）
}
