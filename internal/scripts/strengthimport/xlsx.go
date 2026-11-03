package main

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// readXLSXWorkbook reads every sheet of a .xlsx file into a workbook,
// one sheet per tab, in the file's own tab order.
func readXLSXWorkbook(path string) (workbook, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return workbook{}, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	var wb workbook
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			return workbook{}, fmt.Errorf("reading sheet %q from %s: %w", name, path, err)
		}
		wb.sheets = append(wb.sheets, sheet{tabName: name, rows: rows})
	}
	return wb, nil
}
