// 生成测试用职位表 Excel（验证导入脚本用，非项目代码）。
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/xuri/excelize/v2"
)

func main() {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Sheet1"
	// 模拟国考职位表表头（含首行说明的兼容场景）
	headers := []string{"部门名称", "用人司局", "招考职位", "职位代码", "招考人数", "专业", "学历", "政治面貌", "基层工作最低年限", "工作地点", "备注"}
	data := [][]any{
		{"国家税务总局", "广东省税务局", "一级行政执法员", "300110001", 2, "计算机类、电子信息类", "本科及以上", "不限", "无限制", "广州市", "限2025届高校毕业生"},
		{"深圳市市场监督管理局", "信息中心", "信息化管理岗", "SZ2025001", 1, "计算机科学与技术、软件工程", "本科", "中共党员", "二年", "深圳市", "服务期5年"},
		{"汕头市公安局", "龙湖分局", "基层执法岗", "ST2025008", 3, "不限", "大专及以上", "不限", "无限制", "汕头市", ""},
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	for r, row := range data {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			f.SetCellValue(sheet, cell, v)
		}
	}
	// 生成到系统临时目录（Windows 兼容）
	outPath := filepath.Join(os.TempDir(), "test_positions.xlsx")
	if err := f.SaveAs(outPath); err != nil {
		log.Fatal(err)
	}
	log.Println("测试文件已生成:", outPath)
}
