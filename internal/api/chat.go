package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Chat 多轮追问接口 POST /api/v1/chat（P1，后续迭代接入 Responder Agent）。
// 追问回答将基于长期记忆（三层作用域召回）+ RAG 知识库生成。
func (h *Handler) Chat(c *gin.Context) {
	Fail(c, http.StatusNotImplemented, "多轮追问能力将在后续迭代接入")
}
