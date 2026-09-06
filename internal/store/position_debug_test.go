package store

import (
	"fmt"
	"testing"
)

// TestQueryPositionsDebug 集成测试：直连本地 MySQL 验证查询过滤规则（无库时跳过）。
// 运行：go test ./internal/store/ -run TestQueryPositionsDebug -v
func TestQueryPositionsDebug(t *testing.T) {
	s, err := NewMySQLStore("root:root123@tcp(127.0.0.1:3306)/kaogong?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		t.Skip("MySQL 不可用，跳过:", err)
	}
	for _, f := range []PositionFilter{
		{Province: "广东"},
		{Education: "本科"},
		{MajorCategory: "计算机类"},
		{Province: "广东", Education: "本科", MajorCategory: "计算机类"},
		{Province: "国家"},
	} {
		positions, total, err := s.QueryPositions(f)
		fmt.Printf("filter=%+v total=%d err=%v\n", f, total, err)
		for _, p := range positions {
			fmt.Printf("  - %s | %s | %s | %s\n", p.PositionName, p.Province, p.EducationReq, p.MajorReqCategory)
		}
	}
}
