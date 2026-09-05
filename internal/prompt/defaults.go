package prompt

// Defaults 内置 Prompt 注册表：Prompt 名 → 常量原文。
// 新增 Prompt：在 prompts_*.go 中定义常量，并在此登记。
var Defaults = map[string]string{
	"parser":             PromptParser,            // 条件解析（默认）
	"parser_image_user":  PromptParserImageUser,   // 条件解析-图片用户提示词
	"assistant":          PromptAssistant,         // 通用问答（默认）
	"assistant_beginner": PromptAssistantBeginner, // 通用问答-0 基础模式
	"assistant_advanced": PromptAssistantAdvanced, // 通用问答-进阶模式
}
