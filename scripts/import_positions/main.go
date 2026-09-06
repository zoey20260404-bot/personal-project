// 职位表导入工具（一次性脚本，独立于主服务）。
//
// 用法：
//
//	go run ./scripts/import_positions --file 2026国考职位表.xls --exam-type 国考 --year 2026 --province 国家
//
// 数据源：国考职位表从 bm.scs.gov.cn 年度考录专题下载（招考简章 zip 内）；
// 省考从各省人事考试网下载。支持 .xlsx 与老版 .xls，多 sheet 自动遍历，
// 列名按"包含匹配"识别，兼容国考/省考常见表头变体。
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"

	config "ai-start/configs"
	"ai-start/internal/store"
)

// columnMap 目标字段 → 可能的表头关键词（包含匹配，命中其一即可）。
var columnMap = map[string][]string{
	"department": {"部门名称", "用人司局", "招录单位", "招录机关"},
	"position":   {"招考职位", "职位名称", "岗位名称"},
	"code":       {"职位代码", "岗位代码"},
	"city":       {"工作地点", "考区", "工作城市"},
	"major":      {"专业"},
	"education":  {"学历"},
	"political":  {"政治面貌"},
	"work_years": {"基层工作最低年限", "基层工作经历", "基层工作年限"},
	"fresh":      {"应届生", "限应届"},
	"remarks":    {"备注", "其他要求"},
	"score":      {"进面分", "最低面试分数"},
	"ratio":      {"报录比", "竞争比"},
}

// findColumn 在表头行中按关键词定位列号，找不到返回 -1。
func findColumn(headers []string, keywords []string) int {
	for i, h := range headers {
		for _, kw := range keywords {
			if strings.Contains(strings.TrimSpace(h), kw) {
				return i
			}
		}
	}
	return -1
}

// parseWorkYears 解析基层年限表述（"无限制"/"一年"/"二年"/"三年"）。
func parseWorkYears(s string) int {
	s = strings.TrimSpace(s)
	switch {
	case strings.Contains(s, "无"), s == "":
		return 0
	case strings.Contains(s, "一"):
		return 1
	case strings.Contains(s, "二") || strings.Contains(s, "两"):
		return 2
	case strings.Contains(s, "三"):
		return 3
	case strings.Contains(s, "五"):
		return 5
	}
	return 0
}

// parseFresh 判断是否限应届（备注或专门列中含"应届"且不含"不限"）。
func parseFresh(parts ...string) *bool {
	for _, p := range parts {
		if strings.Contains(p, "应届") && !strings.Contains(p, "不限") {
			t := true
			return &t
		}
	}
	return nil
}

// parseSheet 解析单个 sheet：定位表头行（跳过说明行），逐行解析为岗位。
// 表头判定：岗位列与单位列同时命中且列号不同（国考简章首行是说明文字，需跳过）。
func parseSheet(rows [][]string, examType string, year int, province string) []store.Position {
	// 定位表头行
	headerIdx := -1
	var col map[string]int
	for i, row := range rows[:min(5, len(rows))] {
		posCol := findColumn(row, columnMap["position"])
		deptCol := findColumn(row, columnMap["department"])
		if posCol >= 0 && deptCol >= 0 && posCol != deptCol {
			headerIdx = i
			col = make(map[string]int)
			for field, keywords := range columnMap {
				col[field] = findColumn(row, keywords)
			}
			break
		}
	}
	if headerIdx < 0 {
		return nil // 该 sheet 无职位表结构（如说明页），跳过
	}

	var positions []store.Position
	for i := headerIdx + 1; i < len(rows); i++ {
		row := rows[i]
		cell := func(field string) string {
			if c := col[field]; c >= 0 && c < len(row) {
				return strings.TrimSpace(row[c])
			}
			return ""
		}
		name := cell("position")
		dept := cell("department")
		// 跳过空行与合并单元格产生的残缺行（国考表中"职位属性"等列空值行）
		if name == "" || dept == "" {
			continue
		}
		positions = append(positions, store.Position{
			ExamType:               examType,
			Year:                   year,
			Province:               provinceOf(province, cell("city")), // 国考从工作地点推导实际省份
			City:                   cell("city"),
			Department:             dept,
			PositionName:           name,
			PositionCode:           cell("code"),
			EducationReq:           cell("education"),
			MajorReqExact:          cell("major"), // 专业原文进精确字段；大类由后续规则补充
			PoliticalReq:           cell("political"),
			FreshGraduateReq:       parseFresh(cell("fresh"), cell("remarks")),
			WorkExperienceYearsReq: parseWorkYears(cell("work_years")),
			Remarks:                cell("remarks"),
		})
	}
	return positions
}

func main() {
	file := flag.String("file", "", "职位表 Excel 文件路径（必填）")
	examType := flag.String("exam-type", "国考", "考试类型：国考 | 省考")
	year := flag.Int("year", 2025, "年度")
	province := flag.String("province", "国家", "省份（国考填 国家）")
	dryRun := flag.Bool("dry-run", false, "只解析不入库（预览前 5 条）")
	flag.Parse()

	if *file == "" {
		log.Fatal("缺少 --file 参数")
	}

	// 打开 Excel（支持 .xlsx 与老版 .xls，读取所有 sheet）
	sheetRows, err := readExcel(*file)
	if err != nil {
		log.Fatalf("打开 Excel 失败: %v", err)
	}

	// 逐 sheet 解析（国考简章按机构性质分多个 sheet）
	var positions []store.Position
	for i, rows := range sheetRows {
		parsed := parseSheet(rows, *examType, *year, *province)
		log.Printf("sheet[%d] 解析 %d 条", i, len(parsed))
		positions = append(positions, parsed...)
	}
	if len(positions) == 0 {
		log.Fatal("未解析到任何岗位，请检查文件格式")
	}
	log.Printf("共解析到 %d 条岗位", len(positions))

	if *dryRun {
		for i, p := range positions[:min(5, len(positions))] {
			fmt.Printf("[%d] %s | %s | %s | %s | 应届:%v | 基层%d年\n",
				i+1, p.PositionName, p.Department, p.EducationReq, p.MajorReqExact,
				p.FreshGraduateReq != nil, p.WorkExperienceYearsReq)
		}
		fmt.Println("dry-run 模式，未写入数据库")
		return
	}

	// 入库（复用主项目配置与模型）
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	ms, err := store.NewMySQLStore(cfg.Store.MySQL.DSN)
	if err != nil {
		log.Fatalf("连接 MySQL 失败: %v", err)
	}
	const batch = 500
	for i := 0; i < len(positions); i += batch {
		end := min(i+batch, len(positions))
		if err := ms.CreatePositions(positions[i:end]); err != nil {
			log.Fatalf("写入失败（第 %d 批）: %v", i/batch+1, err)
		}
		log.Printf("已写入 %d/%d", end, len(positions))
	}
	log.Println("导入完成 ✅")
}

// provinceOf 确定岗位省份：省考用参数；国考从工作地点提取
// （如"广东省深圳市"→广东，"北京市"→北京），提取不到时保持原值。
func provinceOf(def, city string) string {
	if def != "国家" || city == "" {
		return def
	}
	// 直辖市
	for _, m := range []string{"北京", "上海", "天津", "重庆"} {
		if strings.HasPrefix(city, m) {
			return m
		}
	}
	// "XX省XX市" / "XX自治区"
	if idx := strings.Index(city, "省"); idx > 0 {
		return city[:idx]
	}
	if idx := strings.Index(city, "自治区"); idx > 0 {
		return city[:idx]
	}
	return def
}

// readExcel 读取 Excel 所有 sheet（按扩展名支持 .xlsx 与老版 .xls）。
func readExcel(path string) ([][][]string, error) {
	if strings.HasSuffix(strings.ToLower(path), ".xls") {
		return readXLS(path)
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out [][][]string
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, err
		}
		out = append(out, rows)
	}
	return out, nil
}

// readXLS 读取老版 .xls 格式（extrame/xls），返回所有 sheet。
func readXLS(path string) ([][][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	wb, err := xls.OpenReader(f, "utf-8")
	if err != nil {
		return nil, err
	}
	var out [][][]string
	for i := 0; i < wb.NumSheets(); i++ {
		sheet := wb.GetSheet(i)
		if sheet == nil {
			continue
		}
		var rows [][]string
		for r := 0; r <= int(sheet.MaxRow); r++ {
			row := sheet.Row(r)
			if row == nil {
				continue
			}
			var cells []string
			for c := row.FirstCol(); c < row.LastCol(); c++ {
				cells = append(cells, row.Col(c))
			}
			rows = append(rows, cells)
		}
		out = append(out, rows)
	}
	return out, nil
}
