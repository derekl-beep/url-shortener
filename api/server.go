package main

import (
	"net/http"
)

func NewServer(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /urls", h.CreateURL)
	mux.Handle("GET /", http.FileServer(http.Dir("api/static")))
	mux.HandleFunc("GET /{key}", h.Redirect)
	return mux
}
