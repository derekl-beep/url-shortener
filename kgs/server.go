package main

import (
	"encoding/json"
	"net/http"
)

type Server struct {
	ks  *KeyStore
	mux *http.ServeMux
}

func NewServer(ks *KeyStore) *Server {
	s := &Server{ks: ks, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /key", s.handleKey)
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	return s
}

func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) handleKey(w http.ResponseWriter, r *http.Request) {
	key, err := s.ks.Next(r.Context())
	if err != nil {
		http.Error(w, "no keys available", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"key": key})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":      "ok",
		"buffer_size": s.ks.Len(),
	})
}
