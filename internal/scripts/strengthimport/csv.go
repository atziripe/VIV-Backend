package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// readCSVWorkbook reads every *.csv file directly inside dir as one
// tab, named after the file (extension stripped) — e.g. "Lower
// body.csv" becomes the tab name "Lower body", which zoneForTabName
// then maps to a zone exactly like it would for an .xlsx tab of that
// name. Files are read in sorted-name order so the resulting workbook
// (and therefore the generated JSON) doesn't depend on directory
// listing order, which isn't guaranteed stable across filesystems.
func readCSVWorkbook(dir string) (workbook, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return workbook{}, fmt.Errorf("reading csv directory %s: %w", dir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".csv") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var wb workbook
	for _, name := range names {
		path := filepath.Join(dir, name)
		rows, err := readCSVFile(path)
		if err != nil {
			return workbook{}, fmt.Errorf("reading %s: %w", path, err)
		}
		tabName := strings.TrimSuffix(name, filepath.Ext(name))
		wb.sheets = append(wb.sheets, sheet{tabName: tabName, rows: rows})
	}
	return wb, nil
}

func readCSVFile(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // a row may have fewer trailing cells than the header
	return r.ReadAll()
}
