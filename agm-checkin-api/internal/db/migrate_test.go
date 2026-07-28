package db

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The schema this test builds and tears down. It deliberately does not use
// public: `go test ./...` runs packages in parallel, and the service tests are
// working in public against the same scratch database.
const testSchema = "migrate_concurrency_test"

// scratchDB returns a connection scoped to testSchema, freshly emptied.
func scratchDB(t *testing.T) (base *gorm.DB, scopedDSN string) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed tests")
	}

	base, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}

	reset := func() {
		if err := base.Exec("DROP SCHEMA IF EXISTS " + testSchema + " CASCADE").Error; err != nil {
			t.Fatalf("dropping schema: %v", err)
		}
	}
	reset()
	if err := base.Exec("CREATE SCHEMA " + testSchema).Error; err != nil {
		t.Fatalf("creating schema: %v", err)
	}
	t.Cleanup(reset)

	return base, dsn + " search_path=" + testSchema
}

// Setup runs AutoMigrate, which is not concurrency safe on its own: two replicas
// starting together race on CREATE TABLE. The advisory lock inside Setup is what
// makes that safe, and this is the test that would fail without it.
func TestSetupIsSafeWhenReplicasStartTogether(t *testing.T) {
	base, scopedDSN := scratchDB(t)

	const replicas = 4

	// Each replica gets its own pool, the way separate pods would.
	pools := make([]*gorm.DB, replicas)
	for i := range pools {
		d, err := gorm.Open(postgres.Open(scopedDSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			t.Fatalf("replica %d connecting: %v", i, err)
		}
		pools[i] = d
	}

	// Release them all at once to maximise the overlap.
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, replicas)
	for i, pool := range pools {
		wg.Add(1)
		go func(i int, pool *gorm.DB) {
			defer wg.Done()
			<-start
			errs[i] = Setup(pool)
		}(i, pool)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("replica %d: %v", i, err)
		}
	}

	// The schema has to come out exactly once, not partially built or doubled.
	var fks int64
	if err := base.Raw(`
		SELECT count(*) FROM pg_constraint c
		JOIN pg_namespace n ON n.oid = c.connamespace
		WHERE c.contype = 'f' AND n.nspname = ?`, testSchema).Scan(&fks).Error; err != nil {
		t.Fatalf("counting foreign keys: %v", err)
	}
	if fks != 4 {
		t.Errorf("expected 4 foreign keys, got %d", fks)
	}

	var applied int64
	if err := base.Raw(
		fmt.Sprintf("SELECT count(*) FROM %s.schema_migrations", testSchema),
	).Scan(&applied).Error; err != nil {
		t.Fatalf("counting applied migrations: %v", err)
	}
	if applied != 1 {
		t.Errorf("expected 1 migration recorded, got %d", applied)
	}
}

// Setup has to be safe to run again on an already-migrated database, since every
// pod restart calls it.
func TestSetupIsIdempotent(t *testing.T) {
	base, scopedDSN := scratchDB(t)

	d, err := gorm.Open(postgres.Open(scopedDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}

	for i := range 3 {
		if err := Setup(d); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var fks int64
	if err := base.Raw(`
		SELECT count(*) FROM pg_constraint c
		JOIN pg_namespace n ON n.oid = c.connamespace
		WHERE c.contype = 'f' AND n.nspname = ?`, testSchema).Scan(&fks).Error; err != nil {
		t.Fatalf("counting foreign keys: %v", err)
	}
	if fks != 4 {
		t.Errorf("expected 4 foreign keys after 3 runs, got %d", fks)
	}
}
