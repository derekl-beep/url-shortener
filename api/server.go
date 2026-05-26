package main

import (
	"net/http"
	"time"
)

func NewServer(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /urls", h.CreateURL)
	mux.Handle("GET /", http.FileServer(http.Dir("api/static")))
	mux.HandleFunc("GET /{key}", h.Redirect)
	return http.TimeoutHandler(mux, 5*time.Second, `{"error":"request timeout"}`)
}
