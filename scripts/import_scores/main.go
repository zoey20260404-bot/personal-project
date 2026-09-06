// 进面分数线导入工具（一次性脚本，独立于主服务）。
//
// 用法：
//
//	go run ./scripts/import_scores --file 面试人员名单.xlsx --year 2026
//
// 数据源：国考面试人员名单（bm.scs.gov.cn 考录专题下载）。
// 名单每行一名进面人员，按职位代码聚合取最低面试分数，回填 positions 表。
package main

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	config "ai-start/configs"
	"ai-start/internal/store"
)

func main() {
	file := flag.String("file", "", "面试人员名单 Excel 文件路径（必填）")
	year := flag.Int("year", 2026, "名单年度")
	dryRun := flag.Bool("dry-run", false, "只聚合不入库（预览前 5 条）")
	flag.Parse()

	if *file == "" {
		log.Fatal("缺少 --file 参数")
	}

	f, err := excelize.OpenFile(*file)
	if err != nil {
		log.Fatalf("打开 Excel 失败: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		log.Fatalf("读取失败: %v", err)
	}
	if len(rows) < 2 {
		log.Fatal("名单内容为空")
	}

	// 表头定位：准考证号 姓名 招录机关 部门代码 用人司局 招考职位 职位代码 最低面试分数
	header := rows[0]
	colCode, colScore := -1, -1
	for i, h := range header {
		h = strings.TrimSpace(h)
		if strings.Contains(h, "职位代码") {
			colCode = i
		}
		if strings.Contains(h, "最低面试分数") {
			colScore = i
		}
	}
	if colCode < 0 || colScore < 0 {
		log.Fatalf("无法识别 职位代码/最低面试分数 列，表头: %v", header)
	}

	// 按职位代码聚合最低分
	type agg struct {
		min   float64
		count int
	}
	scores := make(map[string]*agg)
	for _, row := range rows[1:] {
		if colCode >= len(row) || colScore >= len(row) {
			continue
		}
		code := strings.TrimSpace(row[colCode])
		scoreStr := strings.TrimSpace(row[colScore])
		if code == "" || scoreStr == "" {
			continue
		}
		score, err := strconv.ParseFloat(scoreStr, 64)
		if err != nil {
			continue
		}
		a, ok := scores[code]
		if !ok {
			a = &agg{min: score}
			scores[code] = a
		}
		a.count++
		if score < a.min {
			a.min = score
		}
	}
	log.Printf("聚合出 %d 个职位的进面分数线", len(scores))

	if *dryRun {
		i := 0
		for code, a := range scores {
			fmt.Printf("%s → 最低 %.1f 分（进面 %d 人）\n", code, a.min, a.count)
			if i++; i >= 5 {
				break
			}
		}
		fmt.Println("dry-run 模式，未写入数据库")
		return
	}

	// 入库：按职位代码更新 positions
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	ms, err := store.NewMySQLStore(cfg.Store.MySQL.DSN)
	if err != nil {
		log.Fatalf("连接 MySQL 失败: %v", err)
	}
	matched, unmatched := 0, 0
	for code, a := range scores {
		// 分数线保留一位小数转 int（如 113.9 → 114，展示友好）
		affected, err := ms.UpdatePositionScore(code, int(a.min+0.5), *year)
		if err != nil {
			log.Fatalf("更新失败（%s）: %v", code, err)
		}
		if affected > 0 {
			matched++
		} else {
			unmatched++
		}
	}
	log.Printf("导入完成 ✅ 匹配职位 %d 个，未匹配 %d 个（名单含补录/调剂岗，职位表中可能不存在）", matched, unmatched)
}
