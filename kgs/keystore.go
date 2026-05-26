package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type KeyStore struct {
	db        *pgxpool.Pool
	mu        sync.Mutex
	buffer    []string
	batchSize int
	refillAt  int
	refilling bool
}

func NewKeyStore(db *pgxpool.Pool, batchSize, refillAt int) *KeyStore {
	return &KeyStore{
		db:        db,
		batchSize: batchSize,
		refillAt:  refillAt,
	}
}

// Fill claims an initial batch synchronously. Called once at startup.
func (ks *KeyStore) Fill(ctx context.Context) error {
	keys, err := ks.claim(ctx, ks.batchSize)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return fmt.Errorf("keys_available is empty — run with SEED_COUNT=<n> first")
	}
	ks.buffer = keys
	return nil
}

// Next pops a key from the in-memory buffer and triggers an async refill
// when the buffer drops below refillAt.
func (ks *KeyStore) Next(ctx context.Context) (string, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if len(ks.buffer) == 0 {
		return "", fmt.Errorf("key buffer empty")
	}

	key := ks.buffer[len(ks.buffer)-1]
	ks.buffer = ks.buffer[:len(ks.buffer)-1]

	if len(ks.buffer) < ks.refillAt && !ks.refilling {
		ks.refilling = true
		go ks.refill()
	}

	return key, nil
}

// Len returns the current buffer size (for health checks).
func (ks *KeyStore) Len() int {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return len(ks.buffer)
}

func (ks *KeyStore) refill() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	keys, err := ks.claim(ctx, ks.batchSize)

	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.refilling = false

	if err != nil || len(keys) == 0 {
		return
	}
	ks.buffer = append(ks.buffer, keys...)
}

// claim atomically removes up to n keys from keys_available and returns them.
// SKIP LOCKED ensures concurrent replicas never claim the same key.
func (ks *KeyStore) claim(ctx context.Context, n int) ([]string, error) {
	rows, err := ks.db.Query(ctx, `
		DELETE FROM keys_available
		WHERE key_value IN (
			SELECT key_value FROM keys_available
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING key_value
	`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// GenerateKeys bulk-inserts n random keys into keys_available.
// Duplicate keys are silently skipped via ON CONFLICT DO NOTHING.
func GenerateKeys(ctx context.Context, db *pgxpool.Pool, n int) (int64, error) {
	const batchSize = 10_000
	var total int64

	for remaining := n; remaining > 0; {
		batch := min(batchSize, remaining)
		remaining -= batch

		keys := make([]string, batch)
		for i := range keys {
			k, err := randomKey(8)
			if err != nil {
				return total, err
			}
			keys[i] = k
		}

		tag, err := db.Exec(ctx, `
			INSERT INTO keys_available (key_value)
			SELECT unnest($1::varchar[])
			ON CONFLICT DO NOTHING
		`, keys)
		if err != nil {
			return total, err
		}
		total += tag.RowsAffected()
	}
	return total, nil
}

func randomKey(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// Modulo bias is ~3% and acceptable for short URL keys.
	for i, v := range b {
		b[i] = charset[int(v)%len(charset)]
	}
	return string(b), nil
}
