package store

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func tp(t *testing.T) *pgxpool.Pool {
	dsn := getenv("DATABASE_URL")
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

func getenv(k string) string {
	v := ""
	if x, ok := syscallEnv(k); ok {
		v = x
	}
	return v
}
func syscallEnv(k string) (string, bool) { return syscall.Getenv(k) }

func TestListTargets_PaginationStable(t *testing.T) {
	pool := tp(t)
	pg := &Postgres{Pool: pool}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t0 := time.Now().Add(-3 * time.Second).UTC()
	type row struct {
		id, url, host string
		at            time.Time
	}
	rs := []row{
		{"t1", "https://a.test/1", "a.test", t0.Add(1 * time.Second)},
		{"t2", "https://a.test/2", "a.test", t0.Add(2 * time.Second)},
		{"t3", "https://b.test/3", "b.test", t0.Add(3 * time.Second)},
	}
	for _, r := range rs {
		_, err := pool.Exec(ctx, `
			INSERT INTO targets (id, url, host, created_at) VALUES ($1,$2,$3,$4)
			ON CONFLICT (url) DO NOTHING
		`, r.id, r.url, r.host, r.at)
		require.NoError(t, err)
	}

	//page 1
	items1, next, err := pg.ListTargets(ctx, nil, nil, 2)
	require.NoError(t, err)
	require.Len(t, items1, 2)
	require.NotNil(t, next)

	//page 2
	items2, next2, err := pg.ListTargets(ctx, nil, next, 2)
	require.NoError(t, err)
	require.Len(t, items2, 1)
	require.Nil(t, next2)

	//expected: t1, t2, t3
	got := []string{items1[0].ID, items1[1].ID, items2[0].ID}
	require.Equal(t, []string{"t1", "t2", "t3"}, got)
}
