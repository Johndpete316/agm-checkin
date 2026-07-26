package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"johndpete316/agm-checkin-api/internal/db"
	authmw "johndpete316/agm-checkin-api/internal/middleware"
)

// importFixture returns a clean database, the real router, and an admin bearer
// token, which is what every import upload needs.
func importFixture(t *testing.T) (*gorm.DB, *chi.Mux, string) {
	t.Helper()
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	dropBackupTables(t, database)
	admin := mintToken(t, database, "Imp", "Admin", "admin", "beefbeefbeefbeefbeefbeefbeefbeef")
	return database, router, admin.Token
}

// dropBackupTables clears BulkImport snapshot tables. They survive the fixture
// TRUNCATE, and their names are derived from time.Now().Unix(), so leftovers
// from a previous test in the same second collide with the next import.
func dropBackupTables(t *testing.T, database *gorm.DB) {
	t.Helper()
	var names []string
	if err := database.Raw(
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name ~ '_backup_\d+$'`,
	).Scan(&names).Error; err != nil {
		t.Fatalf("listing backup tables: %v", err)
	}
	for _, n := range names {
		if err := database.Exec("DROP TABLE IF EXISTS " + n).Error; err != nil {
			t.Fatalf("dropping %s: %v", n, err)
		}
	}
}

// uploadImport posts body as the named multipart file field to the import endpoint.
func uploadImport(t *testing.T, router *chi.Mux, token, field, filename string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if field != "" {
		part, err := mw.CreateFormFile(field, filename)
		if err != nil {
			t.Fatalf("creating form file: %v", err)
		}
		if _, err := part.Write(body); err != nil {
			t.Fatalf("writing form file: %v", err)
		}
	} else {
		if err := mw.WriteField("notafile", "x"); err != nil {
			t.Fatalf("writing field: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("closing writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/competitors/import", bytes.NewReader(buf.Bytes()))
	req.RemoteAddr = "10.0.0.9:1234"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

type importResponse struct {
	CompetitorsCreated int      `json:"competitorsCreated"`
	CompetitorsMatched int      `json:"competitorsMatched"`
	FieldsUpdated      int      `json:"fieldsUpdated"`
	EventsCreated      int      `json:"eventsCreated"`
	EventEntriesAdded  int      `json:"eventEntriesAdded"`
	Errors             []string `json:"errors"`
	Error              string   `json:"error"`
}

func decodeImport(t *testing.T, rec *httptest.ResponseRecorder) importResponse {
	t.Helper()
	var out importResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding response %q: %v", rec.Body.String(), err)
	}
	return out
}

func tableCounts(t *testing.T, database *gorm.DB) (competitors, events, competitorEvents int64) {
	t.Helper()
	database.Model(&db.Competitor{}).Count(&competitors)
	database.Model(&db.Event{}).Count(&events)
	database.Model(&db.CompetitorEvent{}).Count(&competitorEvents)
	return
}

const goodHeader = "first_name,last_name,studio,teacher,email,shirt_size,date_of_birth,requires_validation,validated,events\n"

// TestImportMalformedUploads records how the endpoint answers each shape of bad
// input. Every subtest asserts the status code and the resulting row counts, so
// a silent partial success is a failure rather than a shrug.
func TestImportMalformedUploads(t *testing.T) {
	cases := []struct {
		name string
		// body is the raw uploaded bytes.
		body []byte
		// field is the multipart field name; "" means no file part at all.
		field string
		// wantStatus is the status code the endpoint currently returns.
		wantStatus int
		// wantCompetitors is how many competitor rows the import should leave behind.
		wantCompetitors int64
		// wantEvents is how many event rows should survive. A rejected import must
		// leave none: stub events are created before the competitor insert.
		wantEvents int64
	}{
		{
			name:            "well formed baseline",
			body:            []byte(goodHeader + "Ada,Byron,Studio A,Tutor T,ada@example.com,Adult M,2005-01-02,false,true,glr-2026\n"),
			field:           "file",
			wantStatus:      http.StatusOK,
			wantCompetitors: 1,
			wantEvents:      1,
		},
		{
			name:            "columns in a different order",
			body:            []byte("events,last_name,first_name\nglr-2026,Byron,Ada\n"),
			field:           "file",
			wantStatus:      http.StatusOK,
			wantCompetitors: 1,
			wantEvents:      1,
		},
		{
			name: "missing header row",
			// First data line is consumed as the header, so it is silently lost
			// and the remaining rows have no recognisable columns.
			body:            []byte("Ada,Byron,Studio A,Tutor T,ada@example.com,Adult M,2005-01-02,false,true,glr-2026\nGrace,Hopper,Studio B,,g@example.com,Adult L,1906-12-09,false,true,glr-2026\n"),
			field:           "file",
			wantStatus:      http.StatusOK,
			wantCompetitors: 0,
			wantEvents:      0,
		},
		{
			name:            "utf-8 BOM before the header",
			body:            append([]byte("\xef\xbb\xbf"), []byte(goodHeader+"Ada,Byron,,,,,,,,glr-2026\n")...),
			field:           "file",
			wantStatus:      http.StatusOK,
			wantCompetitors: 1,
			wantEvents:      1,
		},
		{
			name:            "CRLF line endings",
			body:            []byte(strings.ReplaceAll(goodHeader+"Ada,Byron,,,,,,,,glr-2026\n", "\n", "\r\n")),
			field:           "file",
			wantStatus:      http.StatusOK,
			wantCompetitors: 1,
			wantEvents:      1,
		},
		{
			name:            "quoted comma inside a field",
			body:            []byte(goodHeader + `Ada,Byron,"Studio A, North","Tutor, T",ada@example.com,Adult M,2005-01-02,false,true,glr-2026` + "\n"),
			field:           "file",
			wantStatus:      http.StatusOK,
			wantCompetitors: 1,
			wantEvents:      1,
		},
		{
			name:            "blank rows mid file",
			body:            []byte(goodHeader + "Ada,Byron,,,,,,,,glr-2026\n\n\nGrace,Hopper,,,,,,,,glr-2026\n"),
			field:           "file",
			wantStatus:      http.StatusOK,
			wantCompetitors: 2,
			wantEvents:      1,
		},
		{
			name:            "identical duplicate rows",
			body:            []byte(goodHeader + "Ada,Byron,,,,,,,,glr-2026\nAda,Byron,,,,,,,,glr-2026\n"),
			field:           "file",
			wantStatus:      http.StatusOK,
			wantCompetitors: 2, // BUG: one person, two records
			wantEvents:      1,
		},
		{
			name:            "latin-1 bytes in a name",
			body:            append([]byte(goodHeader), []byte("Ren\xe9,Byron,,,,,,,,glr-2026\n")...),
			field:           "file",
			wantStatus:      http.StatusInternalServerError,
			wantCompetitors: 0,
			wantEvents:      0,
		},
		{
			name:            "completely empty file",
			body:            []byte(""),
			field:           "file",
			wantStatus:      http.StatusBadRequest,
			wantCompetitors: 0,
			wantEvents:      0,
		},
		{
			name:            "png upload",
			body:            []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89"),
			field:           "file",
			wantStatus:      http.StatusOK,
			wantCompetitors: 0,
			wantEvents:      0,
		},
		{
			name:            "no file field at all",
			body:            nil,
			field:           "",
			wantStatus:      http.StatusBadRequest,
			wantCompetitors: 0,
			wantEvents:      0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			database, router, token := importFixture(t)

			rec := uploadImport(t, router, token, tc.field, "import.csv", tc.body)
			out := decodeImport(t, rec)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}

			competitors, events, ces := tableCounts(t, database)
			if competitors != tc.wantCompetitors {
				t.Fatalf("competitors = %d, want %d (response %+v)", competitors, tc.wantCompetitors, out)
			}
			if events != tc.wantEvents {
				t.Fatalf("events = %d, want %d — a rejected import must roll its stub events back",
					events, tc.wantEvents)
			}
			t.Logf("status=%d competitors=%d events=%d competitor_events=%d response=%+v",
				rec.Code, competitors, events, ces, out)
		})
	}
}

// TestImportWrongFileShapeReportsSuccess pins the behaviour that matters
// operationally: uploading a file with none of the expected columns is reported
// as a successful import of zero people rather than as a rejected upload.
func TestImportWrongFileShapeReportsSuccess(t *testing.T) {
	database, router, token := importFixture(t)

	// The real student-list-master.csv is a schedule export with these headers.
	body := "ID,Name,Instrument,Day,Time,Room,Category,Division\n" +
		"1,Someone Here,Piano,Sat,09:00,101,Solo,Junior\n"

	rec := uploadImport(t, router, token, "file", "student-list-master.csv", []byte(body))
	out := decodeImport(t, rec)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(out.Errors) != 0 {
		t.Fatalf("errors = %v, want none — the endpoint reports a clean import", out.Errors)
	}
	competitors, _, _ := tableCounts(t, database)
	if competitors != 0 {
		t.Fatalf("competitors = %d, want 0", competitors)
	}

	// A snapshot was still taken for an import that could never write anything.
	var snapshots int64
	database.Raw(
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name ~ '^competitors_backup_\d+$'`,
	).Scan(&snapshots)
	if snapshots == 0 {
		t.Fatalf("expected a snapshot table to have been created")
	}
	t.Logf("a wholly unrecognised CSV returned 200 %s and created %d snapshot(s)", rec.Body.String(), snapshots)
}

// TestImportOversizedMultipartBody drives a body past the 32 MiB ParseMultipartForm
// budget. ParseMultipartForm spills to disk rather than failing, so this asserts
// the request is not rejected purely on size.
func TestImportOversizedMultipartBody(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping oversized body test in short mode")
	}
	database, router, token := importFixture(t)

	var sb strings.Builder
	sb.WriteString(goodHeader)
	// ~40 MiB of padding in a column the parser ignores, plus one real row.
	pad := strings.Repeat("x", 4096)
	for i := 0; i < 10*1024; i++ {
		fmt.Fprintf(&sb, ",,%s,,,,,,,\n", pad)
	}
	sb.WriteString("Ada,Byron,,,,,,,,glr-2026\n")

	rec := uploadImport(t, router, token, "file", "big.csv", []byte(sb.String()))
	competitors, _, _ := tableCounts(t, database)
	t.Logf("oversized (%d bytes) -> status=%d competitors=%d body=%s",
		sb.Len(), rec.Code, competitors, truncate(rec.Body.String(), 200))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a large but valid CSV should import", rec.Code)
	}
	if competitors != 1 {
		t.Fatalf("competitors = %d, want 1", competitors)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
