// Package testutil provides shared helpers for integration tests that need a
// real PostgreSQL instance.
package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gonews/gonews/internal/store"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var schemaSeq atomic.Int64

// OpenStore creates an isolated PostgreSQL schema and returns a Store bound to
// it. Tests are skipped when PostgreSQL is unavailable.
func OpenStore(t *testing.T) *store.Store {
	t.Helper()
	base := os.Getenv("GONEWS_TEST_DATABASE_URL")
	if base == "" {
		base = "postgres://gonews:gonews@127.0.0.1:5432/gonews?sslmode=disable"
	}
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := admin.PingContext(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres is unavailable (%v); start it with: docker compose up -d postgres", err)
	}

	schema := fmt.Sprintf("test_%d_%d_%d_%s",
		time.Now().UnixNano(), os.Getpid(), schemaSeq.Add(1), randomSuffix())
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}

	dsn, err := withSearchPath(base, schema)
	if err != nil {
		admin.Close()
		t.Fatalf("build test dsn: %v", err)
	}
	st, err := store.Open(context.Background(), dsn)
	if err != nil {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_, _ = admin.ExecContext(dropCtx, `DROP SCHEMA `+schema+` CASCADE`)
		_ = admin.Close()
	})
	return st
}

func withSearchPath(base, schema string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func randomSuffix() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
