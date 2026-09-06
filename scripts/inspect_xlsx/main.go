package main

import (
	"fmt"
	"github.com/xuri/excelize/v2"
	"os"
)

func main() {
	f, err := excelize.OpenFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	for _, sheet := range f.GetSheetList() {
		rows, _ := f.GetRows(sheet)
		fmt.Printf("sheet %s 共 %d 行\n", sheet, len(rows))
		for i := 0; i < 4 && i < len(rows); i++ {
			fmt.Printf("row%d: %v\n", i, rows[i])
		}
	}
}
