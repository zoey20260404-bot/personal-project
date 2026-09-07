package prompt

// Prompt 名常量：所有 Prompt 的注册名集中在此定义，避免魔法字符串散落各处。
// 新增 Prompt 变体时：先在此定义名字常量，再在 defaults.go 注册。
const (
	NameParser            = "parser"             // 条件解析（文本，默认）
	NameParserDiploma     = "parser_diploma"     // 条件解析（毕业证/学位证图片）
	NameParserImageUser   = "parser_image_user"  // 条件解析-图片用户提示词模板
	NameAssistant         = "assistant"          // 通用问答（默认）
	NameAssistantBeginner = "assistant_beginner" // 通用问答-0 基础模式
	NameAssistantAdvanced = "assistant_advanced" // 通用问答-进阶模式
	NameRouter            = "router"             // 意图路由（feat002）
	NameAdvisor           = "advisor"            // 选岗参谋（feat002，支持 {{.Mode}}/{{.Profile}}/{{.Memories}} 变量）
	NameAnalyzer          = "analyzer"           // 竞争分析（feat004）
	NameStrategist        = "strategist"         // 冲稳保策略（feat004）
	NameResponder         = "responder"          // 报告生成（feat004，{{.Mode}} 变体）
)
