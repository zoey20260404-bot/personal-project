package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListReports 报告列表接口 GET /api/v1/reports（当前登录用户）。
func (h *Handler) ListReports(c *gin.Context) {
	reports, err := h.svc.Services.Advise.ListReports(CurrentUserID(c))
	if err != nil {
		Fail(c, http.StatusInternalServerError, "查询报告列表失败: "+err.Error())
		return
	}
	OK(c, "success", reports)
}

// GetReport 报告详情接口 GET /api/v1/reports/:id（限定当前用户）。
func (h *Handler) GetReport(c *gin.Context) {
	report, err := h.svc.Services.Advise.GetReport(c.Param("id"), CurrentUserID(c))
	if err != nil {
		Fail(c, http.StatusInternalServerError, "查询报告失败: "+err.Error())
		return
	}
	if report == nil {
		Fail(c, http.StatusNotFound, "报告不存在")
		return
	}
	OK(c, "success", report)
}
