package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrationLockKey identifies the advisory lock that serialises migration runs.
// The API deploys with replicaCount 2, so without it concurrent runs could race.
const migrationLockKey = 8724531

// Setup brings a database fully up to date: AutoMigrate first, so the tables the
// models describe exist, then the ordered SQL migrations. Both run under one
// advisory lock, which is the point of the function — AutoMigrate is not safe to
// run concurrently, and the API deploys with replicaCount 2, so two pods starting
// together would otherwise race on CREATE TABLE and leave a half-built schema.
// This is what application entrypoints should call.
func Setup(database *gorm.DB) error {
	return withMigrationLock(database, func(ctx context.Context, conn *sql.Conn) error {
		if err := autoMigrate(database); err != nil {
			return fmt.Errorf("automigrate: %w", err)
		}
		return applyMigrations(ctx, conn)
	})
}

// Migrate applies every migration under migrations/ that has not yet run, in
// filename order, recording each in schema_migrations. It is safe to call
// repeatedly and safe to call concurrently. It does not run AutoMigrate — the
// pre-upgrade job uses this, where the tables already exist.
func Migrate(database *gorm.DB) error {
	return withMigrationLock(database, applyMigrations)
}

// withMigrationLock pins a single connection, holds the advisory lock on it for
// the duration of fn, and releases it afterwards. The lock is session scoped, so
// it has to stay on one connection rather than being scattered across the pool.
func withMigrationLock(database *gorm.DB, fn func(context.Context, *sql.Conn) error) error {
	sqlDB, err := database.DB()
	if err != nil {
		return fmt.Errorf("getting sql handle: %w", err)
	}

	ctx := context.Background()

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquiring connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("acquiring migration lock: %w", err)
	}
	defer func() {
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationLockKey); err != nil {
			log.Printf("releasing migration lock: %v", err)
		}
	}()

	return fn(ctx, conn)
}

// applyMigrations runs the pending migrations. The caller must already hold the
// migration lock.
func applyMigrations(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	versions, err := availableVersions()
	if err != nil {
		return err
	}

	pending := 0
	for _, version := range versions {
		if applied[version] {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("reading %s: %w", version, err)
		}
		if err := applyOne(ctx, conn, version, string(body)); err != nil {
			return err
		}
		log.Printf("applied %s", version)
		pending++
	}

	if pending == 0 {
		log.Println("no pending migrations")
	}
	return nil
}

func availableVersions() ([]string, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("reading migrations directory: %w", err)
	}
	var versions []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			versions = append(versions, e.Name())
		}
	}
	sort.Strings(versions)
	return versions, nil
}

func appliedVersions(ctx context.Context, conn *sql.Conn) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("reading applied migrations: %w", err)
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// applyOne runs a migration and records it in the same transaction, so a failure
// leaves no partial state and the migration can simply be re-run.
func applyOne(ctx context.Context, conn *sql.Conn, version, body string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s: %w", version, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("applying %s: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
		return fmt.Errorf("recording %s: %w", version, err)
	}
	return tx.Commit()
}
