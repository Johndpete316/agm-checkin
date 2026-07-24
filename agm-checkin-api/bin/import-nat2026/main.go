// import-nat2026 converts the nat-2026 attendance/shirt CSV into the standard
// normalized import format and writes it to stdout for upload via the import UI.
//
// The source file is a t-shirt export that doubles as the attendance roster —
// every student in it should be registered for nat-2026.
//
// Usage:
//
//	go run ./bin/import-nat2026 student-list-shirt.csv > nat2026-normalized.csv
package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

const eventID = "nat-2026"

var shirtSizeMap = map[string]string{
	"adult extra large": "Adult XL",
	"adult xl":          "Adult XL",
	"adult xxl":         "Adult XXL",
	"adult 2xl":         "Adult XXL",
	"adult 3xl":         "Adult 3XL",
	"adult large":       "Adult L",
	"adult l":           "Adult L",
	"adult medium":      "Adult M",
	"adult m":           "Adult M",
	"adult small":       "Adult S",
	"adult s":           "Adult S",
	"youth extra large": "Youth XL",
	"youth xl":          "Youth XL",
	"youth large":       "Youth L",
	"youth l":           "Youth L",
	"youth medium":      "Youth M",
	"youth m":           "Youth M",
	"youth small":       "Youth S",
	"youth s":           "Youth S",
}

func normalizeShirt(s string) (normalized string, ok bool) {
	if s == "" {
		return "", true
	}
	key := strings.ToLower(strings.TrimSpace(s))
	if v, found := shirtSizeMap[key]; found {
		return v, true
	}
	return "", false
}

// flipTeacher converts "Last, First" → "First Last".
// If the value has no comma it is returned unchanged.
func flipTeacher(s string) string {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(parts[1]) + " " + strings.TrimSpace(parts[0])
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: import-nat2026 <shirt-csv-file>")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error opening file:", err)
		os.Exit(1)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	out := csv.NewWriter(os.Stdout)
	out.Write([]string{"first_name", "last_name", "studio", "teacher", "shirt_size", "events"})

	written := 0
	skipped := 0
	lineNum := 0

	for {
		row, err := r.Read()
		if err != nil {
			break
		}
		lineNum++

		if len(row) < 2 {
			continue
		}

		last := strings.TrimSpace(row[0])
		first := strings.TrimSpace(row[1])

		// Drop blank rows and the mid-file header row.
		if last == "" || first == "" {
			continue
		}
		if strings.EqualFold(last, "last name") || strings.EqualFold(last, "last") {
			continue
		}

		studio := ""
		if len(row) > 2 {
			studio = strings.TrimSpace(row[2])
		}

		teacher := ""
		if len(row) > 3 {
			teacher = flipTeacher(row[3])
		}

		// Column 4 is teacher email — intentionally skipped (not the student's email).

		shirtRaw := ""
		if len(row) > 5 {
			shirtRaw = strings.TrimSpace(row[5])
		}

		shirt, ok := normalizeShirt(shirtRaw)
		if !ok {
			fmt.Fprintf(os.Stderr, "WARN line %d (%s %s): unrecognized shirt size %q — registering without size\n",
				lineNum, first, last, shirtRaw)
			skipped++
		}

		out.Write([]string{first, last, studio, teacher, shirt, eventID})
		written++
	}

	out.Flush()
	fmt.Fprintf(os.Stderr, "Done: %d students written to nat-2026, %d shirt size warnings\n", written, skipped)
}
