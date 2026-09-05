// Package api 提供对外 HTTP 接口层。
// 基于 Gin 框架，负责请求解析、参数校验、调用 Agent 编排层并返回响应。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ChatRequest 对话请求体。
type ChatRequest struct {
	SessionID string `json:"session_id"` // 会话标识，为空则创建新会话
	Message   string `json:"message"`    // 用户输入文本
}

// ChatResponse 对话响应体。
type ChatResponse struct {
	Reply string `json:"reply"` // Agent 回复内容
}

// ChatHandler 处理对话请求。
// TODO: 接入 agent 编排层，当前为占位实现。
func ChatHandler(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体解析失败"})
		return
	}

	c.JSON(http.StatusOK, ChatResponse{Reply: "服务初始化中，Agent 尚未接入"})
}
