# linkwatch

A tiny HTTP service that registers URLs, periodically checks them, and exposes their status.

## REQUIREMENTS:
1. Go 1.24+
2. Docker (for PostGres)

## HOW TO RUN:
1. from root 'docker compose up -d'
2. (example )curl.exe -s "http://localhost:8080/v1/targets?limit=1" 

## OR

1. 'docker compose up -d db'
2. $env:DATABASE_URL = "postgres://postgres:postgres@localhost:5432/linkwatch?sslmode=disable"
3. go run ./cmd/linkwatch

If it's a new DB - run migrations

## .env : 
- 'DATABASE_URL' – Postgres DSN 
- 'CHECK_INTERVAL' – how often to schedule checks (default '15sec')
- 'MAX_CONCURRENCY' – max parallel checks (default '8')
- 'HTTP_TIMEOUT' – timeout for a single HTTP check (default '5sec')
- 'SHUTDOWN_GRACE' – graceful shutdown deadline (default '10sec')

## MIGRATIONS: 
For a new DB run this: "Get-Content -Raw migrations\001_init.sql   | docker compose exec -T db psql -U postgres -d linkwatch -v ON_ERROR_STOP=1 -f - "
                       "Get-Content -Raw migrations\002_indexes.sql | docker compose exec -T db psql -U postgres -d linkwatch -v ON_ERROR_STOP=1 -f - "

## API:

1. Health 
    GET /healthz
    200 OK
    {"liveness":"ok","db":"ok","checker":"running"}

2. Create target
    POST /v1/targets
    Content-Type: application/json
    Idempotency-Key: <any string> # optional

    {"url":"https://example.org/"}

    - '201 Created' on first create
    - '200 OK' on repeat with same 'Idempotency-Key' and same URL
    - '409 Conflict' on same key and different URL

3. List targets
    GET /v1/targets?host=<host>&limit=<n>&page_token=<opaque>
    200 OK
    {
    "items":[
        {"id":"...","url":"https://...","host":"example.org","created_at":"..."}
    ],
    "next_page_token":"..." 
    }

    - Stable ordering by '(created_at, id)' ascending
    - 'page_token' is an opaque cursor

4. Target Results
    GET /v1/targets/{id}/results?since=<RFC3339>&limit=<n>
    200 OK
    {
    "items":[
        {"target_id":"...","checked_at":"...","status_code":200,"latency_ms":123,"error":null}
            ]
    }

    - Most recent first.
    - 'since' filters by timestamp (RFC3339)

## TESTING:
go test ./...

made by Nurzhan Abdrassilov. 
