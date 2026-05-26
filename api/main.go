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
	"github.com/redis/go-redis/v9"
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

	kgsURL := mustEnv("KGS_URL")
	baseURL := envOr("BASE_URL", "http://localhost:8080")
	port := envOr("PORT", "8080")

	rdb := redis.NewClient(&redis.Options{Addr: mustEnv("REDIS_ADDR")})
	cache := NewCache(rdb)
	producer := NewProducer(rdb)
	store := NewStore(db)
	kgs := NewKGSClient(kgsURL)
	rateLimitRPM := envInt("RATE_LIMIT_RPM", 10)
	rateLimitBurst := envInt("RATE_LIMIT_BURST", 10)
	handler := NewHandler(store, kgs, cache, producer, baseURL)
	mux := NewServer(handler, rateLimitRPM, rateLimitBurst)

	srv := &http.Server{Addr: ":" + port, Handler: mux}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "error", err)
			os.Exit(1)
		}
	}()
	slog.Info("API listening", "port", port)

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	<-sigCtx.Done()

	slog.Info("API shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("API shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("API stopped")
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
