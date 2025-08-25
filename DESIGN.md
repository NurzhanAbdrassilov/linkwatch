## DATA MODEL:
1. 
   ```
   'targets(id TEXT PK, url TEXT UNIQUE, host TEXT, created_at TIMESTAMPTZ DEFAULT now())'
   ```
2. 
   ```
   'check_results(target_id TEXT FK → targets(id) ON DELETE CASCADE,
       checked_at TIMESTAMPTZ, status_code INT NULL, latency_ms INT NULL, error TEXT NULL,
       PRIMARY KEY (target_id, checked_at))'
   ```
3. 
   ```
   `idempotency_keys(key TEXT PK, request_hash TEXT, target_id TEXT FK → targets(id),
       created_at TIMESTAMPTZ DEFAULT now())'
   ```

## API:
1. 'POST /v1/targets'  
  - Validate + canonicalize URL (lower-case host, strip default ports, drop fragments, trim trailing slash except root)  
  - If 'Idempotency-Key' is present, compute 'request_hash = sha256(canonical_url)' and use a transaction:  
    1) If key exists:  
       - same hash → return existing target (200 OK)  
       - different hash → 409 Conflict  
    2) If key not found: ensure target exists, insert '(key, request_hash, target_id)', return 201 Created  
    - A possible race where another request inserts the same key is handled by catching 'unique_violation (23505)' and re-reading to decide same vs conflict
2. 'GET /v1/targets'  
  - Cursor pagination ordered by '(created_at, id)' ascending  
  - Cursor encodes '{created_at,id}' (opaque); query uses '(created_at,id) > (cursor.created_at,cursor.id)'  
  - Check 'limit+1' to find next page; return 'next_page_token' if present
3. 'GET /v1/targets/{id}/results'  
  - Newest-first  
  - Returns 'status_code', 'latency_ms', and 'error'

## BACKGROUND CHECKER: 
1. Schedules all targets every 'CHECK_INTERVAL'  
2. Workers count is at most 'MAX_CONCURRENCY'  
3. Maximum of 1 in-flight request per host  
4. Retries on network error or '5xx' (up to 3 attempts total)  
5. Persists '{status_code, latency_ms, error}' rows

## ADDITIONAL:
1. Graceful shutdown: on SIGINT/SIGTERM, stop scheduling, drain workers up to 'SHUTDOWN_GRACE', then close DB and HTTP server  
2. Configuration via env: 'DATABASE_URL', 'CHECK_INTERVAL', 'MAX_CONCURRENCY', 'HTTP_TIMEOUT', 'SHUTDOWN_GRACE'  
3. Logging: request IDs and access logs via chi middleware; concise startup/health logs
