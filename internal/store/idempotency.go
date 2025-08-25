package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrIdemConflict = errors.New("idempotency key conflict")

// checks if hash and key match
func (p *Postgres) UpsertIdempotencyKey(ctx context.Context, key, requestHash, newID, canonURL, host string) (string, bool, error) {
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	//existing key
	var existingHash, existingTarget string
	err = tx.QueryRow(ctx, `SELECT request_hash, target_id FROM idempotency_keys WHERE key = $1`, key).
		Scan(&existingHash, &existingTarget)
	switch {
	case err == nil:
		if existingHash != requestHash {
			return existingTarget, true, ErrIdemConflict
		}
		//true hash
		if err := tx.Commit(ctx); err != nil {
			return "", true, err
		}
		return existingTarget, true, nil
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return "", false, err
	}

	//target exists
	var tid string
	err = tx.QueryRow(ctx, `SELECT id FROM targets WHERE url = $1`, canonURL).Scan(&tid)
	if errors.Is(err, pgx.ErrNoRows) {
		//insert target
		_, err = tx.Exec(ctx, `
			INSERT INTO targets (id, url, host, created_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (url) DO NOTHING
		`, newID, canonURL, host, time.Now().UTC())
		if err != nil {
			return "", false, err
		}
		//read id
		if err = tx.QueryRow(ctx, `SELECT id FROM targets WHERE url = $1`, canonURL).Scan(&tid); err != nil {
			return "", false, err
		}
	} else if err != nil {
		return "", false, err
	}

	//idempotency mapping
	if _, err = tx.Exec(ctx, `
		INSERT INTO idempotency_keys (key, request_hash, target_id)
		VALUES ($1, $2, $3)
	`, key, requestHash, tid); err != nil {

		//23505 if concurrency issues
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			var h2, t2 string
			if err2 := tx.QueryRow(ctx,
				`SELECT request_hash, target_id FROM idempotency_keys WHERE key = $1`, key,
			).Scan(&h2, &t2); err2 == nil {
				if h2 != requestHash {
					return t2, true, ErrIdemConflict
				}
				//OK
				if err := tx.Commit(ctx); err != nil {
					return "", true, err
				}
				return t2, true, nil
			}
		}
		return "", false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	return tid, false, nil
}
