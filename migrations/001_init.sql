CREATE TABLE IF NOT EXISTS targets (
  id TEXT PRIMARY KEY,
  url TEXT NOT NULL UNIQUE,
  host TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS check_results (
  target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
  checked_at TIMESTAMPTZ NOT NULL,
  status_code INT,
  latency_ms INT,
  error TEXT,
  PRIMARY KEY (target_id, checked_at)
);

-- for POST /v1/targets
CREATE TABLE IF NOT EXISTS idempotency_keys (
  key TEXT PRIMARY KEY,
  request_hash TEXT NOT NULL,
  target_id TEXT NOT NULL REFERENCES targets(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
