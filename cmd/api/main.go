package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aeksunone/go-api-test-kit/internal/api"
	"github.com/aeksunone/go-api-test-kit/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var tasks store.Tasks = store.NewMemory()
	if url := os.Getenv("DATABASE_URL"); url != "" {
		connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		pool, err := pgxpool.New(connectCtx, url)
		if err == nil {
			err = pool.Ping(connectCtx)
		}
		cancel()
		if err != nil {
			log.Fatalf("database connection failed: %v", err)
		}
		defer pool.Close()
		tasks = &store.Postgres{Pool: pool}
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	srv := &http.Server{Addr: addr, Handler: api.New(tasks), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Printf("DEMO ONLY: static public bearer tokens; listening on %s", addr)
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}
}
