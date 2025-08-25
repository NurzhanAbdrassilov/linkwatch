package checker

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nurzh/linkwatch/internal/api"
	"github.com/nurzh/linkwatch/internal/store"
)

type job struct {
	ID, URL, Host string
}

type Checker struct {
	db       *store.Postgres
	client   *http.Client
	jobs     chan job
	state    atomic.Value // starting, running, stopped
	hostLock sync.Map
	interval time.Duration
	workers  int
}

func New(db *store.Postgres, workers int, reqTimeout, interval time.Duration) *Checker {
	if workers <= 0 {
		workers = 4
	}
	if reqTimeout <= 0 {
		reqTimeout = 5 * time.Second
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}

	client := &http.Client{
		Timeout: reqTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				//stop, return 3xx
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	c := &Checker{
		db:       db,
		client:   client,
		jobs:     make(chan job, workers*4),
		interval: interval,
		workers:  workers,
	}
	c.state.Store("starting")
	return c
}

func (c *Checker) State() string {
	if s, ok := c.state.Load().(string); ok {
		return s
	}
	return "stopped"
}

func (c *Checker) Start(ctx context.Context) {
	c.state.Store("running")
	defer c.state.Store("stopped")

	// workers
	var wg sync.WaitGroup
	for i := 0; i < c.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range c.jobs {
				c.doCheck(ctx, j)
			}
		}()
	}

	//scheduler
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	enqueueAll := func() {
		var after *api.Cursor
		for {
			items, next, err := c.db.ListTargets(ctx, nil, after, 500)
			if err != nil {
				return
			}
			for _, t := range items {
				select {
				case c.jobs <- job{ID: t.ID, URL: t.URL, Host: t.Host}:
				case <-ctx.Done():
					close(c.jobs)
					wg.Wait()
					return
				}
			}
			if next == nil {
				break
			}
			after = next
		}
	}

	enqueueAll()
	for {
		select {
		case <-ctx.Done():
			close(c.jobs)
			wg.Wait()
			return
		case <-ticker.C:
			enqueueAll()
		}
	}
}

func (c *Checker) doCheck(ctx context.Context, j job) {
	unlock := c.lockHost(j.Host)
	defer unlock()

	var statusPtr *int
	var latencyPtr *int
	var errStrPtr *string

	for attempt := 1; attempt <= 3; attempt++ {
		t0 := time.Now()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, j.URL, nil)
		req.Header.Set("Accept", "*/*")
		req.Header.Set("User-Agent", "linkwatch/1.0 (+https://example)")
		resp, err := c.client.Do(req)
		elapsed := int(time.Since(t0) / time.Millisecond)
		latencyPtr = &elapsed

		if err == nil {
			code := resp.StatusCode
			resp.Body.Close()
			statusPtr = &code
			// retry on 5xx only
			if code >= 500 && code <= 599 && attempt < 3 {
				time.Sleep(time.Duration(200*(1<<(attempt-1))) * time.Millisecond)
				continue
			}
			break
		}

		s := err.Error()
		errStrPtr = &s
		statusPtr = nil
		if attempt < 3 {
			time.Sleep(time.Duration(200*(1<<(attempt-1))) * time.Millisecond)
			continue
		}
		break
	}

	_ = c.db.AppendCheckResult(context.Background(), store.CheckResult{
		TargetID:   j.ID,
		CheckedAt:  time.Now(),
		StatusCode: statusPtr,
		LatencyMS:  latencyPtr,
		Error:      errStrPtr,
	})
}

func (c *Checker) lockHost(host string) func() {
	v, _ := c.hostLock.LoadOrStore(host, make(chan struct{}, 1))
	ch := v.(chan struct{})
	ch <- struct{}{}
	return func() { <-ch }
}
