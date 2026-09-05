// Package main 是公考参谋部服务的启动入口。
// 职责：加载配置 → 构建 ServiceContext（统一依赖装配）→ 启动 HTTP 服务与 Agent 运行时。
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"ai-start/configs"
	"ai-start/internal/api"
	"ai-start/internal/svc"
)

func main() {
	// 加载应用配置（配置文件路径默认为 configs/config.yaml）
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 构建服务上下文：所有依赖在此统一装配（日志器/模型渠道/存储/工具/Agent 运行时）
	serviceCtx, err := svc.NewServiceContext(cfg)
	if err != nil {
		log.Fatalf("装配服务上下文失败: %v", err)
	}
	defer serviceCtx.Close()
	logger := serviceCtx.Logger

	// 构建 Gin 路由与 HTTP 服务（手动管理生命周期，支持优雅退出）
	handler := api.NewHandler(serviceCtx)
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{Addr: addr, Handler: api.NewRouter(handler)}

	// 捕获终止信号（Ctrl+C / kill），实现优雅退出
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 启动 Agent 运行时（所有节点 + 消息分发协程）
	serviceCtx.Runtime.Start(ctx)

	go func() {
		logger.Info("服务启动中", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("服务启动失败", "err", err)
			stop() // 启动失败时触发整体退出
		}
	}()

	// 等待终止信号，优雅退出：先停 HTTP（处理完在途请求），再停 Agent 节点
	<-ctx.Done()
	logger.Info("收到退出信号，开始优雅退出...")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP 服务退出异常", "err", err)
	}
	serviceCtx.Runtime.Shutdown()
	logger.Info("服务已退出")
}
