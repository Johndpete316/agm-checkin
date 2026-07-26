package main

import (
	"encoding/csv"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// realCSV loads a file from the directory named by IMPORT_TEST_CSV_DIR. The real
// exports hold competitor PII, so they are never committed; point the variable at
// a copy to exercise this suite.
func realCSV(t *testing.T, name string) []byte {
	t.Helper()

	dir := os.Getenv("IMPORT_TEST_CSV_DIR")
	if dir == "" {
		t.Skip("IMPORT_TEST_CSV_DIR not set; skipping real-file import tests")
	}
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Skipf("%s unavailable: %v", name, err)
	}
	return body
}

// csvStats counts data rows and distinct (first_name, last_name) pairs so a test
// can compare what went in against what the import claims to have created.
func csvStats(t *testing.T, body []byte) (dataRows, distinctNames int) {
	t.Helper()

	cr := csv.NewReader(strings.NewReader(string(body)))
	cr.FieldsPerRecord = -1
	records, err := cr.ReadAll()
	if err != nil {
		t.Fatalf("parsing CSV: %v", err)
	}
	if len(records) == 0 {
		return 0, 0
	}

	first, last := -1, -1
	for i, h := range records[0] {
		switch strings.TrimSpace(strings.ToLower(strings.TrimPrefix(h, "\ufeff"))) {
		case "first_name":
			first = i
		case "last_name":
			last = i
		}
	}

	seen := map[string]bool{}
	for _, rec := range records[1:] {
		if len(rec) == 0 || (len(rec) == 1 && rec[0] == "") {
			continue
		}
		dataRows++
		if first >= 0 && last >= 0 && first < len(rec) && last < len(rec) {
			key := strings.ToLower(strings.TrimSpace(rec[first])) + "|" +
				strings.ToLower(strings.TrimSpace(rec[last]))
			if key != "|" {
				seen[key] = true
			}
		}
	}
	return dataRows, len(seen)
}

// TestImportRealNormalizedRoster runs the genuine nat-2026 normalized export
// through the endpoint and checks nothing is silently dropped: every distinct
// person in the file must become a competitor and a roster row.
func TestImportRealNormalizedRoster(t *testing.T) {
	body := realCSV(t, "nat2026-normalized.csv")
	database, router, token := importFixture(t)

	dataRows, distinctNames := csvStats(t, body)

	rec := uploadImport(t, router, token, "file", "nat2026-normalized.csv", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	out := decodeImport(t, rec)

	competitors, events, ces := tableCounts(t, database)
	t.Logf("nat2026-normalized.csv: dataRows=%d distinctNames=%d -> %+v", dataRows, distinctNames, out)
	t.Logf("resulting rows: competitors=%d events=%d competitor_events=%d", competitors, events, ces)

	if int(competitors) != distinctNames {
		t.Fatalf("competitors = %d, want %d (one per distinct name in the file)", competitors, distinctNames)
	}
	if int(ces) != distinctNames {
		t.Fatalf("competitor_events = %d, want %d — every person should be on the roster", ces, distinctNames)
	}
	if len(out.Errors) != 0 {
		t.Fatalf("errors = %v, want none", out.Errors)
	}

	// Re-importing the same real file must be a no-op.
	dropBackupTables(t, database)
	again := uploadImport(t, router, token, "file", "nat2026-normalized.csv", body)
	if again.Code != http.StatusOK {
		t.Fatalf("re-import status = %d: %s", again.Code, again.Body.String())
	}
	secondOut := decodeImport(t, again)
	competitorsAfter, _, cesAfter := tableCounts(t, database)
	if competitorsAfter != competitors || cesAfter != ces {
		t.Fatalf("re-import changed row counts: competitors %d->%d competitor_events %d->%d",
			competitors, competitorsAfter, ces, cesAfter)
	}
	if secondOut.CompetitorsCreated != 0 || secondOut.EventEntriesAdded != 0 {
		t.Fatalf("re-import created rows: %+v", secondOut)
	}
	t.Logf("re-import: %+v", secondOut)
}

// TestImportRealScheduleExportIsSilentlyIgnored uploads student-list-master.csv,
// which is a schedule export rather than a roster. It has none of the expected
// columns, and the endpoint reports a successful import of nobody.
func TestImportRealScheduleExportIsSilentlyIgnored(t *testing.T) {
	body := realCSV(t, "student-list-master.csv")
	database, router, token := importFixture(t)

	dataRows, distinctNames := csvStats(t, body)

	rec := uploadImport(t, router, token, "file", "student-list-master.csv", body)
	out := decodeImport(t, rec)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	competitors, events, ces := tableCounts(t, database)
	t.Logf("student-list-master.csv: dataRows=%d parsed distinct first/last pairs=%d -> %+v",
		dataRows, distinctNames, out)
	t.Logf("resulting rows: competitors=%d events=%d competitor_events=%d", competitors, events, ces)

	if competitors != 0 {
		t.Fatalf("competitors = %d, want 0 — the file has no first_name/last_name columns", competitors)
	}
	if len(out.Errors) != 0 {
		t.Fatalf("errors = %v, want none — the endpoint reports success", out.Errors)
	}
	t.Logf("KNOWN DEFECT: %d schedule rows uploaded as a roster returned 200 with no warning", dataRows)
}
