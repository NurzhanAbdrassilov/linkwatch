package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func testPool(t *testing.T) *pgxpool.Pool {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
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

func TestUpsertIdempotencyKey(t *testing.T) {
	pool := testPool(t)
	pg := &Postgres{Pool: pool}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url1, host1 := "https://example.org/", "example.org"
	url2, host2 := "https://different.org/", "different.org"
	key := "abc123"

	tid1, existed, err := pg.UpsertIdempotencyKey(ctx, key, sha(url1), "t_new_1", url1, host1)
	require.NoError(t, err)
	require.False(t, existed)
	require.NotEmpty(t, tid1)

	tidAgain, existed, err := pg.UpsertIdempotencyKey(ctx, key, sha(url1), "ignored", url1, host1)
	require.NoError(t, err)
	require.True(t, existed)
	require.Equal(t, tid1, tidAgain)

	_, _, err = pg.UpsertIdempotencyKey(ctx, key, sha(url2), "t_new_2", url2, host2)
	require.ErrorIs(t, err, ErrIdemConflict)
}
