package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-start/internal/runtime"
)

// ChatRequest 多轮对话请求体（feat002）。
type ChatRequest struct {
	SessionID string `json:"session_id"`                  // 会话 ID（空则创建新会话）
	Question  string `json:"question" binding:"required"` // 用户问题
	Mode      string `json:"mode"`                        // beginner / advanced（影响表达风格）
}

// Chat 多轮追问接口 POST /api/v1/chat（SSE 流式）。
// 事件流：status（状态）→ tool（工具调用）→ delta（逐 token 回答）→ done（最终结果）。
func (h *Handler) Chat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}

	// SSE 响应头
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		Fail(c, http.StatusInternalServerError, "当前环境不支持流式响应")
		return
	}

	// 事件 sink：ReAct/节点产生的事件实时写入 SSE
	streamed := false // 是否已有增量流出（决定 done 事件要不要带完整答案）
	sink := func(ev runtime.StreamEvent) {
		if ev.Type == runtime.EventDelta && ev.Content != "" {
			streamed = true
		}
		writeSSE(c, ev)
		flusher.Flush()
	}

	reply, err := h.svc.Chat.Chat(c.Request.Context(), CurrentUserID(c), req.SessionID, req.Question, req.Mode, sink)
	if err != nil {
		writeSSE(c, runtime.StreamEvent{Type: runtime.EventError, Content: err.Error()})
		flusher.Flush()
		return
	}
	// 完成事件：元信息必带；答案仅在未流式输出时携带（兜底回复等场景），避免重复传输
	donePayload := map[string]any{
		"session_id": reply.SessionID,
		"agent":      reply.Agent,
		"trace_id":   reply.TraceID,
	}
	if !streamed {
		donePayload["answer"] = reply.Answer
	}
	data, _ := json.Marshal(donePayload)
	writeSSE(c, runtime.StreamEvent{Type: runtime.EventDone, Content: string(data), Agent: reply.Agent})
	flusher.Flush()
}

// writeSSE 写入一条 SSE 事件（data: <json>\n\n）。
func writeSSE(c *gin.Context, ev runtime.StreamEvent) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
}
