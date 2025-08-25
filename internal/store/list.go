package store

import (
	"context"
	"strconv"
	"strings"

	"github.com/nurzh/linkwatch/internal/api"
)

// returns up to limit targets
func (p *Postgres) ListTargets(ctx context.Context, host *string, after *api.Cursor, limit int) (items []Target, next *api.Cursor, err error) {
	args := []any{}
	q := `SELECT id, url, host, created_at FROM targets`

	conds := []string{}
	//filter
	if host != nil && *host != "" {
		conds = append(conds, "host = $"+strconv.Itoa(len(args)+1))
		args = append(args, *host)
	}
	//add condition
	if after != nil {
		conds = append(conds, "(created_at, id) > ($"+strconv.Itoa(len(args)+1)+", $"+strconv.Itoa(len(args)+2)+")")
		args = append(args, after.CreatedAt, after.ID)
	}
	//where
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}

	//order and limit
	args = append(args, limit+1)
	q += " ORDER BY created_at ASC, id ASC LIMIT $" + strconv.Itoa(len(args))

	rows, err := p.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items = make([]Target, 0, limit+1)

	for rows.Next() {
		var t Target
		if err := rows.Scan(&t.ID, &t.URL, &t.Host, &t.CreatedAt); err != nil {
			return nil, nil, err
		}
		items = append(items, t)
	}
	if rows.Err() != nil {
		return nil, nil, rows.Err()
	}

	// there exists another page
	if len(items) > limit {
		lastKept := items[limit-1]
		items = items[:limit]
		nc := api.Cursor{CreatedAt: lastKept.CreatedAt, ID: lastKept.ID}
		next = &nc
	}
	return items, next, nil
}
