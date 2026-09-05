// Package logx 日志组件：基于标准库 slog，按配置输出到控制台/文件/两者。
// 单一实例：在 svc 装配时创建一次，注入到各组件（不使用全局默认 Logger）。
package logx

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Config 日志配置。
type Config struct {
	Level  string `yaml:"level"`  // debug / info / warn / error，默认 info
	Format string `yaml:"format"` // text / json，默认 text
	Output string `yaml:"output"` // stdout / file / both，默认 stdout
	Path   string `yaml:"path"`   // 日志文件路径（output 含 file 时生效），默认 logs/app.log
}

// NewLogger 按配置创建 slog 日志器。
// 返回日志器与文件句柄（可能为 nil，调用方负责在退出时关闭）。
func NewLogger(cfg Config) (*slog.Logger, io.Closer, error) {
	level := parseLevel(cfg.Level)

	// 输出目标：控制台 / 文件 / 两者
	var writer io.Writer
	var closer io.Closer
	switch strings.ToLower(cfg.Output) {
	case "file", "both":
		path := cfg.Path
		if path == "" {
			path = "logs/app.log"
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, nil, fmt.Errorf("创建日志目录失败: %w", err)
		}
		// 追加模式打开（轮转后续可引入 lumberjack 等组件）
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("打开日志文件失败: %w", err)
		}
		closer = f
		if strings.ToLower(cfg.Output) == "both" {
			writer = io.MultiWriter(os.Stdout, f)
		} else {
			writer = f
		}
	default: // stdout
		writer = os.Stdout
	}

	// 格式：text / json
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}
	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}
	return slog.New(handler), closer, nil
}

// parseLevel 解析日志级别字符串，非法值回退 info。
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
