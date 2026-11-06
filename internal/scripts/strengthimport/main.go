package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"viv/internal/core/strengthlibrary"
)

func main() {
	in := flag.String("in", "", "path to the .xlsx workbook, or a directory of per-tab .csv files")
	out := flag.String("out", "internal/core/strengthlibrary/data/exercises.json", "where to write the generated exercises JSON")
	needsReviewOut := flag.String("needs-review", "internal/core/strengthlibrary/data/needs_review.json", `where to write rows flagged "Doesn't belong here?"`)
	equipMapPath := flag.String("equipment-map", "internal/scripts/strengthimport/equipment_mapping.json", "path to the free-text -> controlled-tag equipment mapping")
	flag.Parse()

	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: strengthimport -in <workbook.xlsx | csv-dir> [-out path] [-needs-review path] [-equipment-map path]")
		os.Exit(2)
	}

	equipMapping, err := loadEquipmentMapping(*equipMapPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	wb, err := readWorkbook(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	result := importWorkbook(wb, equipMapping)
	if len(result.errs) > 0 {
		fmt.Fprintln(os.Stderr, "import failed — nothing was written:")
		for _, e := range result.errs {
			fmt.Fprintln(os.Stderr, "  -", e)
		}
		os.Exit(1)
	}

	exercises := result.exercises
	if exercises == nil {
		exercises = []strengthlibrary.Exercise{}
	}
	needsReview := result.needsReview
	if needsReview == nil {
		needsReview = []needsReviewEntry{}
	}

	if err := writeJSON(*out, exercises); err != nil {
		fmt.Fprintln(os.Stderr, "error writing", *out, ":", err)
		os.Exit(1)
	}
	if err := writeJSON(*needsReviewOut, needsReview); err != nil {
		fmt.Fprintln(os.Stderr, "error writing", *needsReviewOut, ":", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %d exercises to %s\n", len(exercises), *out)
	fmt.Printf("wrote %d needs-review rows to %s\n\n", len(needsReview), *needsReviewOut)
	printReport(os.Stdout, exercises)

	if len(needsReview) > 0 {
		fmt.Printf("\n=== %d row(s) need human review (\"Doesn't belong here?\") — see %s ===\n", len(needsReview), *needsReviewOut)
		for _, nr := range needsReview {
			fmt.Printf("  %s / %s / %q: %s\n", nr.Zone, nr.MovementGroup, nr.Name, nr.Comment)
		}
	}
}

func readWorkbook(path string) (workbook, error) {
	info, err := os.Stat(path)
	if err != nil {
		return workbook{}, err
	}
	if info.IsDir() {
		return readCSVWorkbook(path)
	}
	if strings.EqualFold(filepath.Ext(path), ".xlsx") {
		return readXLSXWorkbook(path)
	}
	return workbook{}, fmt.Errorf("%s is neither a directory of CSVs nor an .xlsx file", path)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
