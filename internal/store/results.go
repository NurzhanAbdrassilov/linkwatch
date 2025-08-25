package store

import (
	"context"
	"strconv"
	"time"
)

type CheckResult struct {
	TargetID   string    `json:"target_id"`
	CheckedAt  time.Time `json:"checked_at"`
	StatusCode *int      `json:"status_code,omitempty"`
	LatencyMS  *int      `json:"latency_ms,omitempty"`
	Error      *string   `json:"error,omitempty"`
}

// store check result
func (p *Postgres) AppendCheckResult(ctx context.Context, r CheckResult) error {
	_, err := p.Pool.Exec(ctx, `
		INSERT INTO check_results (target_id, checked_at, status_code, latency_ms, error)
		VALUES ($1, $2, $3, $4, $5)
	`, r.TargetID, r.CheckedAt, r.StatusCode, r.LatencyMS, r.Error)
	return err
}

// most recent results for a target
func (p *Postgres) ListResults(ctx context.Context, targetID string, since *time.Time, limit int) ([]CheckResult, error) {
	args := []any{targetID}
	q := `
		SELECT target_id, checked_at, status_code, latency_ms, error
		FROM check_results
		WHERE target_id = $1
	`
	if since != nil {
		q += " AND checked_at >= $" + strconv.Itoa(len(args)+1)
		args = append(args, *since)
	}
	//sort
	q += " ORDER BY checked_at DESC LIMIT $" + strconv.Itoa(len(args)+1)
	args = append(args, limit)

	rows, err := p.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]CheckResult, 0, limit)
	for rows.Next() {
		var r CheckResult
		if err := rows.Scan(&r.TargetID, &r.CheckedAt, &r.StatusCode, &r.LatencyMS, &r.Error); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
