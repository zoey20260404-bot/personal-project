// 专业大类回填工具（一次性脚本，独立于主服务）。
//
// 用法：go run ./scripts/backfill_major_category [--dry-run]
//
// 职位表的专业要求是原文（如"030105民商法、030103宪法学与行政法学"或"计算机类、电子信息类"），
// 本脚本将其归一为大类（major_req_category），规则：
//  1. 原文含"XX类"直接提取
//  2. 4 位专业代码按前缀映射大类（兼容本科/研究生目录）
//  3. "不限" 保持空
package main

import (
	"flag"
	"fmt"
	"log"
	"regexp"
	"strings"

	config "ai-start/configs"
	"ai-start/internal/store"
)

// codePrefixCategory 专业代码前 4 位 → 大类（本科目录为主，研究生目录常见冲突取其近似）。
var codePrefixCategory = map[string]string{
	"0101": "哲学类",
	"0201": "经济学类", "0202": "财政学类", "0203": "金融学类", "0204": "经济与贸易类",
	"0251": "金融", "0252": "应用统计", "0253": "税务", "0254": "国际商务", "0257": "审计",
	"0301": "法学类", "0302": "政治学类", "0303": "社会学类", "0305": "马克思主义理论类",
	"0351": "法律", "0352": "社会工作",
	"0401": "教育学类", "0402": "心理学类",
	"0501": "中国语言文学类", "0502": "外国语言文学类", "0503": "新闻传播学类",
	"0551": "翻译", "0552": "新闻与传播",
	"0601": "历史学类", "0602": "中国史类",
	"0701": "数学类", "0702": "物理学类", "0703": "化学类", "0712": "统计学类",
	"0802": "机械类", "0805": "材料类", "0806": "电气类", "0807": "电子信息类",
	"0809": "计算机类", "0810": "土木类", "0811": "水利类", "0814": "土木工程",
	"0812": "计算机科学与技术", "0817": "化学工程与技术",
	"0828": "建筑类", "0830": "环境科学与工程类",
	"0901": "植物生产类", "0905": "动物医学类",
	"1002": "临床医学类", "1007": "药学类",
	"1202": "工商管理类", "1204": "公共管理类", "1205": "图书情报与档案管理类",
	"1251": "工商管理", "1252": "公共管理", "1253": "会计", "1256": "工程管理",
}

// categoryPattern 直接出现在原文中的"XX类"。
var categoryPattern = regexp.MustCompile(`[一-龥]{2,12}类`)

// codePattern 4 位专业代码（如 0301、0812、120203K 的前缀）。
var codePattern = regexp.MustCompile(`(\d{4})\d{0,2}K?`)

// deriveCategory 从专业要求原文提取大类集合。
func deriveCategory(exact string) string {
	exact = strings.TrimSpace(exact)
	if exact == "" || exact == "不限" {
		return ""
	}
	seen := map[string]bool{}
	var cats []string
	add := func(c string) {
		if c != "" && c != "不限" && !seen[c] {
			seen[c] = true
			cats = append(cats, c)
		}
	}
	// 1. 原文中的"XX类"
	for _, m := range categoryPattern.FindAllString(exact, -1) {
		add(m)
	}
	// 2. 专业代码前缀映射
	for _, m := range codePattern.FindAllStringSubmatch(exact, -1) {
		if cat, ok := codePrefixCategory[m[1]]; ok {
			add(cat)
		}
	}
	return strings.Join(cats, "、")
}

func main() {
	dryRun := flag.Bool("dry-run", false, "只统计不入库")
	flag.Parse()

	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	ms, err := store.NewMySQLStore(cfg.Store.MySQL.DSN)
	if err != nil {
		log.Fatalf("连接 MySQL 失败: %v", err)
	}

	// 读取所有需要回填的岗位（有大类原文但未归一）
	positions, err := ms.ListPositionsForBackfill()
	if err != nil {
		log.Fatalf("查询失败: %v", err)
	}
	log.Printf("待回填 %d 条岗位", len(positions))

	updated, empty := 0, 0
	for _, p := range positions {
		cat := deriveCategory(p.MajorReqExact)
		if cat == "" {
			empty++
			continue
		}
		if *dryRun {
			if updated < 5 {
				fmt.Printf("%s → %s\n", truncate(p.MajorReqExact, 60), cat)
			}
		} else {
			if err := ms.UpdateMajorCategory(p.ID, cat); err != nil {
				log.Fatalf("更新失败（id=%d）: %v", p.ID, err)
			}
		}
		updated++
	}
	fmt.Printf("完成：归一 %d 条，无法识别 %d 条（保持空按不限处理）\n", updated, empty)
}

// truncate 截断长文本。
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
