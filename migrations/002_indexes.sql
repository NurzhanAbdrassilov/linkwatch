CREATE INDEX IF NOT EXISTS targets_host_created_idx ON targets (host, created_at, id);
CREATE INDEX IF NOT EXISTS results_target_checked_idx ON check_results (target_id, checked_at DESC);