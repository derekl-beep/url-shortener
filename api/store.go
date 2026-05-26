package main

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// FindByHash returns the short_key for a URL that was already shortened,
// or an empty string if not found.
func (s *Store) FindByHash(ctx context.Context, hash string) (string, error) {
	var key string
	err := s.db.QueryRow(ctx, `SELECT short_key FROM urls WHERE url_hash = $1`, hash).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return key, err
}

// FindByKey returns the original URL for a given short key, or empty string if not found.
func (s *Store) FindByKey(ctx context.Context, key string) (string, error) {
	var originalURL string
	err := s.db.QueryRow(ctx, `
		SELECT original_url FROM urls
		WHERE short_key = $1 AND (expires_at IS NULL OR expires_at > NOW())
	`, key).Scan(&originalURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return originalURL, err
}

// Insert atomically records the URL mapping and marks the key as used.
func (s *Store) Insert(ctx context.Context, key, originalURL, hash string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO urls (short_key, original_url, url_hash)
		VALUES ($1, $2, $3)
	`, key, originalURL, hash)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `INSERT INTO keys_used (key_value) VALUES ($1)`, key)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
