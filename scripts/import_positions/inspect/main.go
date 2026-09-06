// xls 结构探查工具（调试用）：打印 sheet 列表与前几行原始内容。
package main

import (
	"fmt"
	"os"

	"github.com/extrame/xls"
)

func main() {
	f, _ := os.Open(os.Args[1])
	defer f.Close()
	wb, err := xls.OpenReader(f, "utf-8")
	if err != nil {
		panic(err)
	}
	fmt.Println("sheet 数量:", wb.NumSheets())
	for i := 0; i < wb.NumSheets(); i++ {
		sheet := wb.GetSheet(i)
		fmt.Printf("--- sheet[%d] %s, 最大行 %d ---\n", i, sheet.Name, sheet.MaxRow)
		for r := 9; r <= 60 && r <= int(sheet.MaxRow); r++ {
			row := sheet.Row(r)
			if row == nil {
				continue
			}
			var cells []string
			for c := row.FirstCol(); c < row.LastCol() && c < 12; c++ {
				v := row.Col(c)
				if len(v) > 12 {
					v = v[:12] + "…"
				}
				cells = append(cells, fmt.Sprintf("[%d]%s", c, v))
			}
			fmt.Printf("  row%d: %v\n", r, cells)
		}
	}
}
