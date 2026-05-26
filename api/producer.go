package main

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const clickStream = "click-events"

type Producer struct {
	client *redis.Client
}

func NewProducer(client *redis.Client) *Producer {
	return &Producer{client: client}
}

// PublishClick fires a click event onto the Redis Stream asynchronously.
// It does not block the HTTP handler.
func (p *Producer) PublishClick(shortKey, ipAddress, userAgent, referrer string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		p.client.XAdd(ctx, &redis.XAddArgs{
			Stream: clickStream,
			Values: map[string]any{
				"short_key":  shortKey,
				"clicked_at": time.Now().UTC().Format(time.RFC3339Nano),
				"ip_address": ipAddress,
				"user_agent": userAgent,
				"referrer":   referrer,
			},
		})
	}()
}

func ipFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i != -1 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}
