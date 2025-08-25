package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nurzh/linkwatch/internal/core"
	"github.com/nurzh/linkwatch/internal/store"
	"github.com/stretchr/testify/require"
)

func testPoolRB(t *testing.T) *pgxpool.Pool {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))
	t.Cleanup(func() { pool.Close() })
	_, _ = pool.Exec(ctx, "TRUNCATE idempotency_keys, check_results, targets")
	return pool
}

func TestRetryBackoffEventually200(t *testing.T) {
	pool := testPoolRB(t)
	pg := &store.Postgres{Pool: pool}

	var hits int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n <= 2 {
			http.Error(w, "boom", http.StatusInternalServerError) // 500
			return
		}
		w.WriteHeader(200)
	}))
	defer s.Close()

	// insert
	canon, host, err := core.Canonicalize(s.URL)
	require.NoError(t, err)
	id := core.NewID("t")
	_, err = pool.Exec(context.Background(), `
		INSERT INTO targets (id, url, host, created_at) VALUES ($1,$2,$3, now())
	`, id, canon, host)
	require.NoError(t, err)

	c := New(pg, 1, 2*time.Second, 1*time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	c.Start(ctx)

	//attempted 2 or 3 times
	require.GreaterOrEqual(t, atomic.LoadInt32(&hits), int32(2))
	require.LessOrEqual(t, atomic.LoadInt32(&hits), int32(3))

	//expected: 200
	rows, err := pool.Query(context.Background(), `
		SELECT status_code FROM check_results WHERE target_id = $1 ORDER BY checked_at DESC LIMIT 1
	`, id)
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next(), "expected at least one check result")
	var code *int
	require.NoError(t, rows.Scan(&code))
	require.NotNil(t, code)
	require.Equal(t, 200, *code)
}
