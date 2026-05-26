package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	ctx := context.Background()

	db, err := pgxpool.New(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		slog.Error("connect db", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Seed mode: pre-populate keys_available then exit.
	if n := envInt("SEED_COUNT", 0); n > 0 {
		slog.Info("seeding keys", "count", n)
		inserted, err := GenerateKeys(ctx, db, n)
		if err != nil {
			slog.Error("seed", "error", err)
			os.Exit(1)
		}
		slog.Info("seed complete", "inserted", inserted)
		return
	}

	batchSize := envInt("BATCH_SIZE", 10_000)
	refillAt := envInt("REFILL_THRESHOLD", 1_000)
	port := envOr("PORT", "8081")

	ks := NewKeyStore(db, batchSize, refillAt)
	if err := ks.Fill(ctx); err != nil {
		slog.Error("initial key fill", "error", err)
		os.Exit(1)
	}
	slog.Info("key buffer loaded", "size", ks.Len())

	httpSrv := &http.Server{Addr: ":" + port, Handler: NewServer(ks).Handler()}

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "error", err)
			os.Exit(1)
		}
	}()
	slog.Info("KGS listening", "port", port)

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	<-sigCtx.Done()

	slog.Info("KGS shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutCtx); err != nil {
		slog.Error("KGS shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("KGS stopped")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required env var not set", "key", key)
		os.Exit(1)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		slog.Error("env var must be an integer", "key", key, "error", err)
		os.Exit(1)
	}
	return n
}
