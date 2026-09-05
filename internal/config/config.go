// Package config 负责应用配置的加载与解析。
// 配置来源：configs/config.yaml，后续可扩展环境变量覆盖。
package config

// Config 应用顶层配置结构。
type Config struct {
	Server ServerConfig `yaml:"server"` // HTTP 服务配置
	LLM    LLMConfig    `yaml:"llm"`    // 大模型接入配置
}

// ServerConfig HTTP 服务相关配置。
type ServerConfig struct {
	Port int `yaml:"port"` // 监听端口
}

// LLMConfig 大模型接入配置。
type LLMConfig struct {
	Provider string `yaml:"provider"` // 模型提供方，如 openai、qwen
	Model    string `yaml:"model"`    // 模型名称
	APIKey   string `yaml:"api_key"`  // API 密钥（建议通过环境变量注入）
}

// Load 从指定路径加载配置文件。
// TODO: 引入 yaml 解析库后实现真实加载逻辑。
func Load(path string) (*Config, error) {
	// 当前返回默认配置，保证项目可编译运行
	return &Config{
		Server: ServerConfig{Port: 8080},
	}, nil
}
