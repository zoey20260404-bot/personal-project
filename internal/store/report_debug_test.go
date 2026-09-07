package store

import (
	"fmt"
	"testing"
)

func TestReportInsertDebug(t *testing.T) {
	s, err := NewMySQLStore("root:root123@tcp(127.0.0.1:3306)/kaogong?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		t.Skip(err)
	}
	r := &Report{ReportID: "rep_debug1", UserID: 4, Status: "processing", Profile: `{"education":"本科"}`}
	err = s.CreateReport(r)
	fmt.Println("CreateReport err:", err, "id:", r.ID)
	err = s.CompleteReport("rep_debug1", "completed", "# 报告内容", "")
	fmt.Println("CompleteReport err:", err)
}
