package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	db, err := pgxpool.New(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	// Seed mode: pre-populate keys_available then exit.
	if n := envInt("SEED_COUNT", 0); n > 0 {
		log.Printf("seeding %d keys...", n)
		inserted, err := GenerateKeys(ctx, db, n)
		if err != nil {
			log.Fatalf("seed: %v", err)
		}
		log.Printf("inserted %d keys", inserted)
		return
	}

	batchSize := envInt("BATCH_SIZE", 10_000)
	refillAt := envInt("REFILL_THRESHOLD", 1_000)
	port := envOr("PORT", "8081")

	ks := NewKeyStore(db, batchSize, refillAt)
	if err := ks.Fill(ctx); err != nil {
		log.Fatalf("initial key fill: %v", err)
	}
	log.Printf("key buffer loaded: %d keys", ks.Len())

	srv := NewServer(ks)
	log.Printf("KGS listening on :%s", port)
	if err := srv.ListenAndServe(":" + port); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s not set", key)
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
		log.Fatalf("env var %s must be an integer: %v", key, err)
	}
	return n
}
