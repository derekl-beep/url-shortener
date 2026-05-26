package main

import "net/http"

func NewServer(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /urls", h.CreateURL)
	mux.HandleFunc("GET /{key}", h.Redirect)
	return mux
}
