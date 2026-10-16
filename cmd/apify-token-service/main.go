// apify-token-service keeps the Apify account registry honest: it asks Apify
// what each account's budget, billing cycle and concurrency limit actually
// are, and parks accounts that are nearly spent before a crawl job discovers
// it the expensive way.
//
// It is a control plane, not a gateway. Crawl services lease slots straight
// from Postgres and call Apify directly; nothing here sits in that path. If
// this process is down, crawling continues on data that gradually goes stale
// -- a spent account is then caught by the inline failure path instead of in
// advance, which is exactly the behaviour from before this existed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"

	"github.com/zeromicro/go-zero/core/conf"
)

type Config struct {
	ListenOn string
	Postgres struct {
		DSN string
	}
	Apify struct {
		// Seed value. Accounts live in apify_tokens once seeded; this is how
		// they get there on first boot and how new ones are added.
		ApiToken string
		BaseURL  string
		// Fallback concurrency for an account not yet refreshed. The real
		// value is read from Apify per account.
		DefaultMaxConcurrency int `json:",default=5"`
	}
	Refresh struct {
		// How often the sweep runs.
		Interval string `json:",default=1m"`
		// An account is refreshed if it has not been checked this recently.
		// With 1000 accounts and a 6h window, the sweep touches ~3/min.
		StaleAfter string `json:",default=6h"`
		// Accounts refreshed per sweep. Caps the burst of outbound calls.
		BatchSize int `json:",default=25"`
		// Concurrent Apify calls within a batch.
		Concurrency int `json:",default=5"`
		// Usage fraction at which an account is parked without waiting for
		// it to fail.
		ParkAbove float64 `json:",default=0.95"`
	}
}

var configFile = flag.String("f", "etc/tokens.yaml", "config file")

func main() {
	flag.Parse()

	var c Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	interval := mustDuration("Refresh.Interval", c.Refresh.Interval)
	staleAfter := mustDuration("Refresh.StaleAfter", c.Refresh.StaleAfter)

	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	reg := apify.NewRegistry(conn, apify.NewLimitsClient(c.Apify.BaseURL))
	reg.ParkAbove = c.Refresh.ParkAbove

	// Seeding is idempotent and keeps measured state, so restarting with a
	// longer APIFY_API_TOKEN is all it takes to add accounts.
	if tokens := apify.ParseTokens(c.Apify.ApiToken); len(tokens) > 0 {
		if err := reg.Seed(context.Background(), tokens, c.Apify.DefaultMaxConcurrency); err != nil {
			panic(fmt.Errorf("apify-token-service: seed: %w", err))
		}
		fmt.Printf("seeded %d account(s) from config\n", len(tokens))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runSweeps(ctx, reg, interval, staleAfter, c.Refresh.BatchSize, c.Refresh.Concurrency)
	}()

	srv := &http.Server{Addr: c.ListenOn, Handler: routes(reg)}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("apify-token-service listening on %s, sweeping every %s\n", c.ListenOn, interval)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Println("http server stopped:", err)
	}
	wg.Wait()
}

func mustDuration(name, v string) time.Duration {
	d, err := time.ParseDuration(v)
	if err != nil {
		panic(fmt.Errorf("apify-token-service: %s=%q is not a duration: %w", name, v, err))
	}
	return d
}

// runSweeps refreshes stale accounts and applies the parking policy until ctx
// ends. A sweep that fails is logged and retried next tick rather than killing
// the loop: the registry going stale is survivable, the loop dying is not.
func runSweeps(ctx context.Context, reg *apify.Registry, interval, staleAfter time.Duration, batch, concurrency int) {
	tick := time.NewTicker(interval)
	defer tick.Stop()

	for {
		sweep(ctx, reg, staleAfter, batch, concurrency)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func sweep(ctx context.Context, reg *apify.Registry, staleAfter time.Duration, batch, concurrency int) {
	states, secrets, err := reg.StaleTokens(ctx, staleAfter, batch)
	if err != nil {
		fmt.Println("sweep: list stale:", err)
		return
	}

	if len(states) > 0 {
		sem := make(chan struct{}, max(concurrency, 1))
		var wg sync.WaitGroup
		var mu sync.Mutex
		var okCount, failCount int

		for i := range states {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				_, err := reg.RefreshOne(ctx, states[i].TokenIndex, secrets[i])
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failCount++
					return
				}
				okCount++
			}(i)
		}
		wg.Wait()
		fmt.Printf("sweep: refreshed %d account(s), %d failed\n", okCount, failCount)
	}

	if _, _, err := reg.ApplyPolicy(ctx); err != nil {
		fmt.Println("sweep: apply policy:", err)
	}
}

func routes(reg *apify.Registry) http.Handler {
	mux := http.NewServeMux()

	// Operational answer to "how much crawling can we still do, and when does
	// capacity come back".
	mux.HandleFunc("/tokens/summary", func(w http.ResponseWriter, r *http.Request) {
		s, err := reg.Summary(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s)
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	return mux
}
