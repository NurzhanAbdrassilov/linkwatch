package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nurzh/linkwatch/internal/core"
	"github.com/nurzh/linkwatch/internal/store"
	"github.com/stretchr/testify/require"
)

func testPool(t *testing.T) *pgxpool.Pool {
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

func TestPerHostSerialization(t *testing.T) {
	pool := testPool(t)
	pg := &store.Postgres{Pool: pool}

	// server 1
	var curA, maxA int
	var muA sync.Mutex
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		muA.Lock()
		curA++
		if curA > maxA {
			maxA = curA
		}
		muA.Unlock()
		time.Sleep(250 * time.Millisecond)
		muA.Lock()
		curA--
		muA.Unlock()
		w.WriteHeader(200)
	}))
	defer srvA.Close()

	// server 2
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(250 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srvB.Close()

	// insert targets
	type tgt struct{ url, host, id string }
	add := func(url string) tgt {
		canon, host, err := core.Canonicalize(url)
		require.NoError(t, err)
		id := core.NewID("t")
		_, err = pool.Exec(context.Background(), `
			INSERT INTO targets (id, url, host, created_at)
			VALUES ($1,$2,$3, now())
		`, id, canon, host)
		require.NoError(t, err)
		return tgt{canon, host, id}
	}

	_ = add(srvB.URL + "/x")

	c := New(pg, 4, 2*time.Second, 1*time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	c.Start(ctx)

	//expected: Server 1 never had >1 in-flight at once
	require.LessOrEqual(t, maxA, 1, "per-host lock should serialize same-host requests")
}
