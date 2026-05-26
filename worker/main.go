package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	stream   = "click-events"
	group    = "click-workers"
	consumer = "worker-1"
)

func main() {
	ctx := context.Background()

	db, err := pgxpool.New(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mustEnv("REDIS_ADDR")})

	// Create consumer group. MKSTREAM creates the stream if it doesn't exist yet.
	// "0" means the group starts reading from the beginning of the stream.
	if err := rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil {
		// BUSYGROUP means the group already exists — safe to ignore.
		if err.Error() != "BUSYGROUP Consumer Group name already exists" {
			log.Fatalf("create consumer group: %v", err)
		}
	}

	log.Printf("worker started, consuming %s", stream)

	for {
		msgs, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    10,
			Block:    2 * time.Second,
		}).Result()

		if err == redis.Nil {
			continue
		}
		if err != nil {
			log.Printf("xreadgroup: %v", err)
			continue
		}

		for _, msg := range msgs[0].Messages {
			insertCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := insertClick(insertCtx, db, msg.Values)
			cancel()
			if err != nil {
				log.Printf("insert %s: %v", msg.ID, err)
				continue
			}
			rdb.XAck(ctx, stream, group, msg.ID)
		}
	}
}

func insertClick(ctx context.Context, db *pgxpool.Pool, v map[string]any) error {
	clickedAt, _ := time.Parse(time.RFC3339Nano, str(v["clicked_at"]))

	_, err := db.Exec(ctx, `
		INSERT INTO click_events (short_key, clicked_at, ip_address, user_agent, referrer)
		VALUES ($1, $2, $3, $4, $5)
	`, str(v["short_key"]), clickedAt, str(v["ip_address"]), str(v["user_agent"]), str(v["referrer"]))
	return err
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s not set", key)
	}
	return v
}
