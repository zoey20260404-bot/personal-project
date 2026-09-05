// Package main 是公考参谋部服务的启动入口。
// 职责：加载配置、初始化各模块依赖、启动 Gin HTTP 服务。
package main

import (
	"fmt"
	"log"

	"ai-start/internal/api"
	"ai-start/internal/config"
)

func main() {
	// 加载应用配置（配置文件路径默认为 configs/config.yaml）
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// TODO: 初始化 OCR、RAG、LLM、Agent 等模块并注入路由层

	// 构建 Gin 路由并启动服务
	r := api.NewRouter()
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("服务启动中，监听地址: %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
