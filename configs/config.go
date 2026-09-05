// Package config 负责应用配置的加载与解析。
//
// 设计原则：一切可变皆有默认——模型渠道、Prompt、执行流程（flows）、
// 存储均在 configs/config.yaml 配置，代码只提供骨架与默认值；
// 密钥类敏感配置支持环境变量覆盖，不写入仓库。
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"ai-start/internal/logx"
)

// Config 应用顶层配置结构。
type Config struct {
	Server  ServerConfig           `yaml:"server"`  // HTTP 服务配置
	JWT     JWTConfig              `yaml:"jwt"`     // JWT 鉴权配置
	Log     logx.Config            `yaml:"log"`     // 日志配置
	Models  map[string]ModelConfig `yaml:"models"`  // 模型渠道表（key 为渠道名，Agent 按名引用）
	Agents  map[string]AgentConfig `yaml:"agents"`  // Agent 定义表（key 为 Agent 名/节点名）
	Flows   map[string][]string    `yaml:"flows"`   // 执行流程编排：流程名 → 节点名列表（按序执行）
	Store   StoreConfig            `yaml:"store"`   // 存储配置（MySQL/pgvector/Redis）
	Runtime RuntimeConfig          `yaml:"runtime"` // Agent 运行时参数
}

// ServerConfig HTTP 服务相关配置。
type ServerConfig struct {
	Port int `yaml:"port"` // 监听端口，默认 8080
}

// JWTConfig JWT 鉴权配置。
type JWTConfig struct {
	Secret      string `yaml:"secret"`       // 签名密钥，建议用环境变量 JWT_SECRET 注入
	ExpireHours int    `yaml:"expire_hours"` // Token 有效期（小时），默认 72
}

// ModelConfig 模型渠道配置（OpenAI 兼容协议，可指向任意服务商/本地模型）。
// 约定渠道名：chat（文本）、vision（视觉，密钥为空时复用 chat）、embedding（向量化）。
type ModelConfig struct {
	BaseURL     string   `yaml:"base_url"`    // API 地址
	Model       string   `yaml:"model"`       // 模型名
	APIKey      string   `yaml:"api_key"`     // API 密钥，建议用环境变量注入
	Dim         int      `yaml:"dim"`         // 向量维度（仅 embedding 渠道使用）
	Temperature *float64 `yaml:"temperature"` // 采样温度（指针：nil 表示用服务商默认）
	MaxTokens   int      `yaml:"max_tokens"`  // 最大输出 token 数（0 表示不限制）
	Timeout     int      `yaml:"timeout"`     // 单次调用超时（秒），默认 60
}

// AgentConfig 单个 Agent 的定义（配置化装配，新增 Agent 只需加配置）。
type AgentConfig struct {
	Type        string   `yaml:"type"`        // Agent 类型：parser / react（内置可扩展）
	Model       string   `yaml:"model"`       // 引用的模型渠道名（models 中的 key）
	Prompt      string   `yaml:"prompt"`      // 引用的 Prompt 名（prompts 目录下的文件名，不含扩展名）
	Tools       []string `yaml:"tools"`       // 可用工具名列表（react 类型使用）
	MaxSteps    int      `yaml:"max_steps"`   // ReAct 最大推理步数（react 类型使用，默认 5）
	Temperature *float64 `yaml:"temperature"` // 覆盖渠道的采样温度（可选）
}

// StoreConfig 存储配置。
type StoreConfig struct {
	MySQL    DSNConfig   `yaml:"mysql"`    // 结构化业务数据（用户/收藏/会话）
	Postgres DSNConfig   `yaml:"postgres"` // 向量存储（长期记忆/RAG）
	Redis    RedisConfig `yaml:"redis"`    // 通用缓存（预留给后续业务）
}

// DSNConfig 通用数据源连接配置。
type DSNConfig struct {
	DSN string `yaml:"dsn"`
}

// RedisConfig Redis 连接配置。
type RedisConfig struct {
	Addr     string `yaml:"addr"`     // 地址，如 127.0.0.1:6379
	Password string `yaml:"password"` // 密码，可为空
	DB       int    `yaml:"db"`       // 数据库编号
}

// RuntimeConfig Agent 运行时参数。
type RuntimeConfig struct {
	BusBuffer   int `yaml:"bus_buffer"`   // 消息总线缓冲容量，默认 256
	NodeInbox   int `yaml:"node_inbox"`   // 节点收件箱缓冲容量，默认 32
	CallTimeout int `yaml:"call_timeout"` // 节点调用超时（秒），默认 30
}

// Load 从指定路径加载 YAML 配置文件，应用默认值与环境变量覆盖。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败（请从 configs/config.example.yaml 复制一份为 config.yaml 并填入密钥）: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	cfg.applyDefaults()
	cfg.applyEnvOverrides()
	return &cfg, nil
}

// applyDefaults 为未配置项填充默认值。
func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.JWT.ExpireHours == 0 {
		c.JWT.ExpireHours = 72
	}
	if c.Runtime.BusBuffer == 0 {
		c.Runtime.BusBuffer = 256
	}
	if c.Runtime.NodeInbox == 0 {
		c.Runtime.NodeInbox = 32
	}
	if c.Runtime.CallTimeout == 0 {
		c.Runtime.CallTimeout = 30
	}
	// embedding 渠道向量维度默认值（bge-m3）
	if ch, ok := c.Models["embedding"]; ok && ch.Dim == 0 {
		ch.Dim = 1024
		c.Models["embedding"] = ch
	}
}

// applyEnvOverrides 环境变量优先于配置文件，避免密钥入库。
func (c *Config) applyEnvOverrides() {
	if key := os.Getenv("LLM_API_KEY"); key != "" {
		ch := c.Models["chat"]
		ch.APIKey = key
		c.Models["chat"] = ch
	}
	if key := os.Getenv("EMBEDDING_API_KEY"); key != "" {
		ch := c.Models["embedding"]
		ch.APIKey = key
		c.Models["embedding"] = ch
	}
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		c.JWT.Secret = secret
	}
}
