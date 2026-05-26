package main

import (
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

func NewServer(h *Handler, rateLimitRPM, rateLimitBurst int) http.Handler {
	rl := newRateLimiter(rate.Every(time.Minute/time.Duration(rateLimitRPM)), rateLimitBurst)

	mux := http.NewServeMux()
	mux.Handle("POST /urls", rl.Middleware(http.HandlerFunc(h.CreateURL)))
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.Handle("GET /", http.FileServer(http.Dir("api/static")))
	mux.HandleFunc("GET /{key}", h.Redirect)
	return http.TimeoutHandler(mux, 5*time.Second, `{"error":"request timeout"}`)
}
