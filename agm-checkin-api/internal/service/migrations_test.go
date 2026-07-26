package service

import (
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"johndpete316/agm-checkin-api/internal/db"
)

// newEmptyDatabase returns a connection to a database whose public schema has
// been dropped and recreated, i.e. the state a brand-new deployment starts from.
// newFixture cannot stand in for this: it truncates but never drops, so after the
// first run every table already exists and the fresh-install path goes untested.
func newEmptyDatabase(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed tests")
	}

	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}

	if err := database.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`).Error; err != nil {
		t.Fatalf("resetting public schema: %v", err)
	}
	return database
}

// A fresh deployment brings the schema up with AutoMigrate and then runs the
// ordered migrations, and that combination has to work with no database behind
// it. It did not: 001 and 002 both read columns (last_registered_event,
// validated) that only an upgrading database has, because the Go model stopped
// declaring them, so AutoMigrate never creates them. Postgres rejected the whole
// migration, db.Migrate returned an error, and main() treats that as fatal — a
// first install could not start at all.
func TestMigrateFromEmptyDatabase(t *testing.T) {
	database := newEmptyDatabase(t)

	db.AutoMigrate(database)
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrating a fresh database: %v", err)
	}

	var applied []string
	if err := database.Raw(`SELECT version FROM schema_migrations ORDER BY version`).
		Scan(&applied).Error; err != nil {
		t.Fatalf("reading schema_migrations: %v", err)
	}
	if len(applied) == 0 {
		t.Fatal("no migrations recorded in schema_migrations")
	}

	// The foreign keys are the point of 001; if they are absent the migration
	// reported success without doing its job.
	var fks int64
	if err := database.Raw(`
		SELECT count(*) FROM pg_constraint
		WHERE contype = 'f' AND connamespace = 'public'::regnamespace`).
		Scan(&fks).Error; err != nil {
		t.Fatalf("counting foreign keys: %v", err)
	}
	if fks != 4 {
		t.Errorf("foreign keys after migration = %d, want 4", fks)
	}
}

// Migrations run on every API start, and the API runs with two replicas, so
// re-running against an already-migrated database has to be a no-op rather than
// a second application of the same DDL.
func TestMigrateIsIdempotent(t *testing.T) {
	database := newEmptyDatabase(t)

	db.AutoMigrate(database)
	if err := db.Migrate(database); err != nil {
		t.Fatalf("first migration run: %v", err)
	}

	var first []string
	if err := database.Raw(`SELECT version FROM schema_migrations ORDER BY version`).
		Scan(&first).Error; err != nil {
		t.Fatalf("reading schema_migrations: %v", err)
	}

	db.AutoMigrate(database)
	if err := db.Migrate(database); err != nil {
		t.Fatalf("second migration run: %v", err)
	}

	var second []string
	if err := database.Raw(`SELECT version FROM schema_migrations ORDER BY version`).
		Scan(&second).Error; err != nil {
		t.Fatalf("reading schema_migrations after second run: %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("schema_migrations changed on re-run: %v then %v", first, second)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("schema_migrations changed on re-run: %v then %v", first, second)
		}
	}
}
