package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	Pool *pgxpool.Pool
}

// insert or return existing url
func (p *Postgres) CreateOrGetTarget(ctx context.Context, id, canonURL, host string) (Target, bool, error) {
	var t Target
	//try inserting
	ct := time.Now().UTC()
	_, err := p.Pool.Exec(ctx, `
		INSERT INTO targets (id, url, host, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (url) DO NOTHING
	`, id, canonURL, host, ct)
	if err != nil {
		return t, false, err
	}

	//read row
	row := p.Pool.QueryRow(ctx, `
		SELECT id, url, host, created_at
		FROM targets
		WHERE url = $1
	`, canonURL)
	if err := row.Scan(&t.ID, &t.URL, &t.Host, &t.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return t, false, errors.New("failed to read target after insert/select")
		}
		return t, false, err
	}

	created := (t.ID == id)
	return t, created, nil
}
