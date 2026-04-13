// import reads 4 historical competitor CSV files and outputs a single normalized CSV
// to stdout. The output can be reviewed, edited, and then uploaded via the admin import UI.
//
// Usage:
//
//	go run ./bin/import glr-2026.csv glr-2025.csv nat-2025.csv nat-2024.csv > normalized.csv
//
// Files can be passed in any order — the CSV type is detected from the header row.
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	eventGLR2026 = "glr-2026"
	eventGLR2025 = "glr-2025"
	eventNAT2025 = "nat-2025"
	eventNAT2024 = "nat-2024"
)

// eventOrder is chronological oldest→newest. Used to sort the events column.
var eventOrder = []string{eventNAT2024, eventGLR2025, eventNAT2025, eventGLR2026}

// processOrder controls which file type is merged first. glr-2026 goes first so it
// provides the studio+teacher base; historical files then fill in DOBs and verification.
var processOrder = []string{eventGLR2026, eventNAT2024, eventGLR2025, eventNAT2025}

type mergedRecord struct {
	NameFirst string
	NameLast  string
	Studio    string
	Teacher   string
	Email     string
	ShirtSize string
	DOB       *time.Time
	Verified  bool          // true if verified="yes" in any historical sheet
	Events    map[string]bool
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: import <csv-file> [<csv-file> ...]")
		fmt.Fprintln(os.Stderr, "example: go run ./bin/import glr-2026.csv glr-2025.csv nat-2025.csv nat-2024.csv > normalized.csv")
		os.Exit(1)
	}

	// Detect the type of each file.
	type fileEntry struct {
		path    string
		csvType string
	}
	var files []fileEntry
	for _, path := range os.Args[1:] {
		ct, err := detectCSVType(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot detect type for %s: %v\n", path, err)
			os.Exit(1)
		}
		files = append(files, fileEntry{path, ct})
		fmt.Fprintf(os.Stderr, "detected %s → %s\n", path, ct)
	}

	// Process in defined order so glr-2026 (studio+teacher) is the base.
	merged := map[string]*mergedRecord{}
	for _, eventID := range processOrder {
		for _, fe := range files {
			if fe.csvType == eventID {
				count, err := processFile(fe.path, eventID, merged)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error processing %s: %v\n", fe.path, err)
					os.Exit(1)
				}
				fmt.Fprintf(os.Stderr, "processed %s: %d rows\n", fe.path, count)
			}
		}
	}

	// Write normalized CSV to stdout.
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{
		"first_name", "last_name", "studio", "teacher", "email",
		"shirt_size", "date_of_birth", "requires_validation", "validated", "events",
	}); err != nil {
		fmt.Fprintln(os.Stderr, "csv write error:", err)
		os.Exit(1)
	}

	written := 0
	for _, rec := range merged {
		var eventList []string
		for _, eid := range eventOrder {
			if rec.Events[eid] {
				eventList = append(eventList, eid)
			}
		}

		dob := ""
		if rec.DOB != nil {
			dob = rec.DOB.Format("2006-01-02")
		}

		// RequiresValidation:
		//   - Previously verified in any historical sheet → false (trust prior verification)
		//   - Brand new (glr-2026 only) or has history but never verified → true
		requiresValidation := !rec.Verified
		validated := rec.Verified

		if err := w.Write([]string{
			rec.NameFirst,
			rec.NameLast,
			rec.Studio,
			rec.Teacher,
			rec.Email,
			rec.ShirtSize,
			dob,
			strconv.FormatBool(requiresValidation),
			strconv.FormatBool(validated),
			strings.Join(eventList, "|"),
		}); err != nil {
			fmt.Fprintln(os.Stderr, "csv write error:", err)
			os.Exit(1)
		}
		written++
	}

	w.Flush()
	if err := w.Error(); err != nil {
		fmt.Fprintln(os.Stderr, "csv flush error:", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "done: %d unique competitors written\n", written)
}

// detectCSVType opens a file and inspects its header row to determine which event it belongs to.
func detectCSVType(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	headers, err := r.Read()
	if err != nil {
		return "", fmt.Errorf("reading header: %w", err)
	}

	hset := map[string]bool{}
	for _, h := range headers {
		hset[strings.TrimSpace(h)] = true
	}

	switch {
	case hset["Studio"] && hset["Teacher"]:
		return eventGLR2026, nil
	case hset["T-Shirt"] && hset["Birthdate"]:
		return eventGLR2025, nil
	case hset["birthdate"] && hset["Shirt Size"]:
		return eventNAT2025, nil
	case hset["STUDIO"] && hset["Birthdate"]:
		return eventNAT2024, nil
	default:
		preview := headers
		if len(preview) > 8 {
			preview = preview[:8]
		}
		return "", fmt.Errorf("unrecognized CSV format (first headers: %v)", preview)
	}
}

// processFile reads one CSV and merges its records into the shared map.
// Returns the number of data rows processed.
func processFile(path, eventID string, merged map[string]*mergedRecord) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	headers, err := r.Read()
	if err != nil {
		return 0, fmt.Errorf("reading header: %w", err)
	}

	// Build a column-name → index map.
	cols := map[string]int{}
	for i, h := range headers {
		cols[strings.TrimSpace(h)] = i
	}

	col := func(row []string, name string) string {
		idx, ok := cols[name]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	count := 0
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping malformed row in %s: %v\n", path, err)
			continue
		}

		var (
			first    string
			last     string
			studio   string
			teacher  string
			email    string
			shirt    string
			dobStr   string
			verified bool
		)

		switch eventID {
		case eventGLR2026:
			first = col(row, "First Name")
			last = col(row, "Last Name")
			studio = col(row, "Studio")
			teacher = col(row, "Teacher")
			// Teacher email is intentionally not stored as the competitor email.
			// It will be used for a teachers table in the future.
			shirt = col(row, "Shirt Size")
			// No DOB or Verified column in this file.

		case eventGLR2025:
			first = col(row, "First Name")
			last = col(row, "Last Name")
			shirt = col(row, "T-Shirt")
			dobStr = col(row, "Birthdate")
			email = col(row, "Student/Parent Email")
			verified = strings.EqualFold(col(row, "Verified"), "yes")

		case eventNAT2025:
			first = col(row, "First Name")
			last = col(row, "Last Name")
			dobStr = col(row, "birthdate")
			email = col(row, "email")
			shirt = col(row, "Shirt Size")
			verified = strings.EqualFold(col(row, "Verified?"), "yes")

		case eventNAT2024:
			first = col(row, "First Name")
			last = col(row, "LAST NAME")
			shirt = col(row, "TSHIRT SIZE")
			dobStr = col(row, "Birthdate")
			studio = col(row, "STUDIO")
			email = col(row, "e-mail")
			verified = strings.EqualFold(col(row, "verified?"), "yes")
		}

		// Skip rows with no name or apparent summary/total rows.
		if first == "" || last == "" {
			continue
		}
		if strings.EqualFold(first, "totals") || strings.EqualFold(last, "totals") {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(first) + " " + strings.TrimSpace(last))

		var dob *time.Time
		if dobStr != "" {
			if eventID == eventGLR2025 {
				dob, err = parseDOBTwoDigit(dobStr)
			} else {
				dob, err = parseDOBFourDigit(dobStr)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not parse DOB %q for %s %s: %v\n", dobStr, first, last, err)
			}
		}

		shirt = normalizeShirtSize(shirt)

		if rec, ok := merged[key]; ok {
			// Merge into the existing record.
			if rec.DOB == nil && dob != nil {
				rec.DOB = dob
			}
			if rec.Studio == "" && studio != "" {
				rec.Studio = studio
			}
			if rec.Teacher == "" && teacher != "" {
				rec.Teacher = teacher
			}
			// Prefer student/parent email over teacher email (teacher email only comes from glr-2026).
			if eventID != eventGLR2026 && email != "" {
				rec.Email = email
			}
			if rec.ShirtSize == "" && shirt != "" {
				rec.ShirtSize = shirt
			}
			if verified {
				rec.Verified = true
			}
			rec.Events[eventID] = true
		} else {
			merged[key] = &mergedRecord{
				NameFirst: strings.TrimSpace(first),
				NameLast:  strings.TrimSpace(last),
				Studio:    studio,
				Teacher:   teacher,
				Email:     email,
				ShirtSize: shirt,
				DOB:       dob,
				Verified:  verified,
				Events:    map[string]bool{eventID: true},
			}
		}

		count++
	}

	return count, nil
}

// parseDOBTwoDigit parses dates in M/D/YY format (used in glr-2025).
// Go's "06" year token maps 00–68 → 2000–2068, 69–99 → 1969–1999.
// After parsing, any year > 2026 (the current event year) is adjusted back 100 years
// to handle cases like "66" being parsed as 2066 rather than 1966.
func parseDOBTwoDigit(s string) (*time.Time, error) {
	t, err := time.Parse("1/2/06", s)
	if err != nil {
		return nil, err
	}
	if t.Year() > 2026 {
		t = t.AddDate(-100, 0, 0)
	}
	return &t, nil
}

// parseDOBFourDigit parses dates in M/D/YYYY format (used in nat-2024, nat-2025).
func parseDOBFourDigit(s string) (*time.Time, error) {
	t, err := time.Parse("1/2/2006", s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

var shirtSizeMap = map[string]string{
	"adult extra large": "Adult XL",
	"adult xl":          "Adult XL",
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

func normalizeShirtSize(s string) string {
	if s == "" {
		return ""
	}
	key := strings.ToLower(strings.TrimSpace(s))
	if normalized, ok := shirtSizeMap[key]; ok {
		return normalized
	}
	return s
}
