package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nurzh/linkwatch/internal/api"
	"github.com/nurzh/linkwatch/internal/checker"
	"github.com/nurzh/linkwatch/internal/core"
	"github.com/nurzh/linkwatch/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type health struct {
	Liveness string `json:"liveness"`
	DB       string `json:"db"`
	Checker  string `json:"checker"`
}

type createTargetReq struct {
	URL string `json:"url"`
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	var pool *pgxpool.Pool
	if dbURL == "" {
		log.Println("DATABASE_URL is not set")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var err error
		pool, err = pgxpool.New(ctx, dbURL)
		if err != nil {
			log.Printf("pgxpool.New error: %v", err)
		} else if err := pool.Ping(ctx); err != nil {
			log.Printf("DB ping error: %v", err)
		} else {
			log.Println("DB connected")
		}
	}

	pg := &store.Postgres{Pool: pool}
	getDur := func(k string, def time.Duration) time.Duration {
		if s := os.Getenv(k); s != "" {
			if d, err := time.ParseDuration(s); err == nil {
				return d
			}
		}
		return def
	}

	getInt := func(k string, def int) int {
		if s := os.Getenv(k); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				return n
			}
		}
		return def
	}

	//defaults
	checkInterval := getDur("CHECK_INTERVAL", 15*time.Second)
	httpTimeout := getDur("HTTP_TIMEOUT", 5*time.Second)
	maxConc := getInt("MAX_CONCURRENCY", 8)
	grace := getDur("SHUTDOWN_GRACE", 10*time.Second)

	chk := checker.New(pg, maxConc, httpTimeout, checkInterval)

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)

	/*Liveness probe returning `200 OK` once the server is ready.*/
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		resp := health{Liveness: "ok", DB: "down", Checker: chk.State()}
		if pool != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
			defer cancel()
			if err := pool.Ping(ctx); err == nil {
				resp.DB = "ok"
			}
		}
		status := http.StatusOK
		if resp.DB != "ok" {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, resp)
	})

	/*List targets with **cursor pagination**. Stable, deterministic ordering*/
	r.Get("/v1/targets", func(w http.ResponseWriter, r *http.Request) {
		if pool == nil {
			http.Error(w, "DB not configured", http.StatusServiceUnavailable)
			return
		}

		//query
		q := r.URL.Query()
		var host *string
		if h := strings.TrimSpace(q.Get("host")); h != "" {
			lh := strings.ToLower(h)
			host = &lh
		}
		limit := 20
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
				limit = n
			}
		}
		var after *api.Cursor
		if tok := q.Get("page_token"); tok != "" {
			c, err := api.DecodeCursor(tok)
			if err != nil {
				http.Error(w, "bad page_token", http.StatusBadRequest)
				return
			}
			after = &c
		}

		ctx, cancel := api.CtxTimeout(r.Context(), 3*time.Second)
		defer cancel()

		items, next, err := pg.ListTargets(ctx, host, after, limit)
		if err != nil {
			http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		resp := map[string]any{
			"items": items,
		}
		if next != nil {
			resp["next_page_token"] = api.EncodeCursor(*next)
		}

		writeJSON(w, http.StatusOK, resp)
	})

	/*Return recent check results for a target*/
	r.Get("/v1/targets/{id}/results", func(w http.ResponseWriter, r *http.Request) {
		if pool == nil {
			http.Error(w, "DB not configured", http.StatusServiceUnavailable)
			return
		}
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		//query
		q := r.URL.Query()
		limit := 50
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		var since *time.Time
		if s := q.Get("since"); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				since = &t
			} else {
				http.Error(w, "bad since (use RFC3339)", http.StatusBadRequest)
				return
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		items, err := pg.ListResults(ctx, id, since, limit)
		if err != nil {
			http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	/*Validate and **canonicalize** URL, Support **Idempotency-Key** header*/
	r.Post("/v1/targets", func(w http.ResponseWriter, r *http.Request) {
		if pool == nil {
			http.Error(w, "DB not configured", http.StatusServiceUnavailable)
			return
		}

		var body createTargetReq
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		canon, host, err := core.Canonicalize(body.URL)
		if err != nil {
			http.Error(w, "bad url: "+err.Error(), http.StatusBadRequest)
			return
		}

		//Idempotency-Key
		if key := r.Header.Get("Idempotency-Key"); key != "" {
			h := sha256.Sum256([]byte(canon))
			reqHash := hex.EncodeToString(h[:])

			id := core.NewID("t")
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			tid, existed, err := pg.UpsertIdempotencyKey(ctx, key, reqHash, id, canon, host)
			if err != nil {
				if errors.Is(err, store.ErrIdemConflict) {
					http.Error(w, "idempotency key already used", http.StatusConflict)
					return
				}
				http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
				return
			}

			row := pool.QueryRow(ctx, `SELECT id, url, host, created_at FROM targets WHERE id = $1`, tid)
			var t store.Target
			if err := row.Scan(&t.ID, &t.URL, &t.Host, &t.CreatedAt); err != nil {
				http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if existed {
				writeJSON(w, http.StatusOK, t)
			} else {
				writeJSON(w, http.StatusCreated, t)
			}
			return
		}

		//no key
		id := core.NewID("t")
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		t, created, err := pg.CreateOrGetTarget(ctx, id, canon, host)
		if err != nil {
			http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if created {
			writeJSON(w, http.StatusCreated, t)
		} else {
			writeJSON(w, http.StatusOK, t)
		}
	})

	srv := &http.Server{Addr: ":8080", Handler: r}
	go func() {
		log.Println("listening on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	go chk.Start(ctx)

	<-ctx.Done()
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	if pool != nil {
		pool.Close()
	}
	log.Println("shutdown complete")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
