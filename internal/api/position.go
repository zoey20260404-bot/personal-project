package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ai-start/internal/store"
)

// ListPositions 岗位查询接口 GET /api/v1/positions。
// 按条件分页查询历年岗位数据（纯规则过滤，无 AI），供页面展示。
func (h *Handler) ListPositions(c *gin.Context) {
	filter := store.PositionFilter{
		ExamType:      c.Query("exam_type"),
		Province:      c.Query("province"),
		City:          c.Query("city"),
		Keyword:       c.Query("keyword"),
		Education:     c.Query("education"),
		MajorCategory: c.Query("major_category"),
		Political:     c.Query("political"),
		Page:          queryInt(c, "page", 1),
		PageSize:      queryInt(c, "page_size", 20),
	}
	// 多目标省份（档案条件，IN 查询）：?provinces=广东&provinces=湖南
	filter.Provinces = c.QueryArray("provinces")
	// 应届身份过滤：fresh=false 时排除限应届岗
	if freshStr := c.Query("is_fresh"); freshStr != "" {
		fresh := freshStr == "true"
		filter.IsFresh = &fresh
	}
	// 基层年限过滤：岗位要求 ≤ 用户年限
	if wyStr := c.Query("work_years"); wyStr != "" {
		if wy, err := strconv.Atoi(wyStr); err == nil {
			filter.WorkYears = &wy
		}
	}

	positions, total, err := h.svc.Services.Position.Query(filter)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "查询岗位失败: "+err.Error())
		return
	}
	OK(c, "success", gin.H{
		"positions": positions,
		"total":     total,
		"page":      filter.Page,
		"page_size": filter.PageSize,
	})
}

// queryInt 解析整数查询参数，缺省/非法时用默认值。
func queryInt(c *gin.Context, key string, defaultVal int) int {
	if v, err := strconv.Atoi(c.Query(key)); err == nil {
		return v
	}
	return defaultVal
}
