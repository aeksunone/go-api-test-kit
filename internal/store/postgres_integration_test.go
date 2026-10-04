//go:build integration

package store_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aeksunone/go-api-test-kit/internal/store"
	"github.com/aeksunone/go-api-test-kit/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// isolatedPostgres never truncates shared tables or drops a pre-existing schema.
// A per-pool startup search_path applies to every connection, not just whichever
// connection happened to execute a SET statement during setup.
func isolatedPostgres(t *testing.T) *store.Postgres {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if strings.TrimSpace(dsn) == "" {
		t.Fatal("integration tests require TEST_DATABASE_URL pointing to a disposable PostgreSQL database; tests are never skipped")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid TEST_DATABASE_URL")
	}
	adminCfg := cfg.Copy()
	adminCfg.MaxConns = 1
	adminCfg.MinConns = 0
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		t.Fatalf("create schema-admin pool: %v", err)
	}
	t.Cleanup(admin.Close)
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatalf("generate schema name: %v", err)
	}
	schema := "test_" + hex.EncodeToString(random[:])
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatalf("create isolated schema (test role needs CREATE on the database): %v", err)
	}
	// Register only after successful CREATE. A name collision must never grant
	// this fixture permission to remove a schema it did not create.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("remove owned test schema %s: %v", schema, err)
		}
	})
	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.ConnConfig.RuntimeParams["search_path"] = quoted
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("create isolated repository pool: %v", err)
	}
	t.Cleanup(pool.Close) // LIFO: close connections, drop owned schema, close admin.
	if _, err := pool.Exec(ctx, migrations.Initial); err != nil {
		t.Fatalf("apply embedded migration: %v", err)
	}
	for id, want := range map[int64]string{aliceID: "Alice", bobID: "Bob"} {
		var name string
		if err := pool.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", id).Scan(&name); err != nil || name != want {
			t.Fatalf("migration fixture %d = %q, %v; want %q", id, name, err, want)
		}
	}
	return &store.Postgres{Pool: pool}
}

func TestPostgresContract(t *testing.T) {
	t.Parallel()
	repositoryContract(t, func(t *testing.T) store.Tasks { return isolatedPostgres(t) })
}

func TestPostgresSchemaIsolation(t *testing.T) {
	t.Parallel()
	left, right := isolatedPostgres(t), isolatedPostgres(t)
	ctx := context.Background()
	created, err := left.Create(ctx, aliceID, "left schema only")
	if err != nil {
		t.Fatal(err)
	}
	assertList(t, right, aliceID, []store.Task{})
	_, err = right.Get(ctx, aliceID, created.ID)
	wantNotFound(t, err)
	other, err := right.Create(ctx, aliceID, "right schema only")
	if err != nil {
		t.Fatal(err)
	}
	assertList(t, left, aliceID, []store.Task{created})
	assertList(t, right, aliceID, []store.Task{other})

	// Hold distinct connections simultaneously so startup search_path is checked
	// on every connection the pool can open, rather than repeatedly reusing one.
	for range int(left.Pool.Config().MaxConns) {
		conn, err := left.Pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		var title string
		if err := conn.QueryRow(ctx, "SELECT title FROM tasks WHERE id=$1", created.ID).Scan(&title); err != nil || title != created.Title {
			t.Fatalf("connection escaped isolated search_path: title=%q, error=%v", title, err)
		}
	}
}

func TestPostgresConstraints(t *testing.T) {
	t.Parallel()
	repo := isolatedPostgres(t)
	for _, tc := range []struct {
		name  string
		owner int64
		title string
		code  string
	}{
		{"empty title", aliceID, "", "23514"},
		{"overlong title", aliceID, strings.Repeat("x", 201), "23514"},
		{"unknown owner", 999, "orphan", "23503"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.Create(context.Background(), tc.owner, tc.title)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tc.code {
				t.Fatalf("Create error = %v; want PostgreSQL SQLSTATE %s", err, tc.code)
			}
		})
	}
	assertList(t, repo, aliceID, []store.Task{})
}
