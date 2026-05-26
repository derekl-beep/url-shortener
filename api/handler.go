package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Handler struct {
	store    *Store
	kgs      *KGSClient
	cache    *Cache
	producer *Producer
	baseURL  string
}

func NewHandler(store *Store, kgs *KGSClient, cache *Cache, producer *Producer, baseURL string) *Handler {
	return &Handler{store: store, kgs: kgs, cache: cache, producer: producer, baseURL: baseURL}
}

type createRequest struct {
	URL string `json:"url"`
}

type createResponse struct {
	ShortURL string `json:"short_url"`
	Key      string `json:"key"`
}

func (h *Handler) CreateURL(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if _, err := url.ParseRequestURI(req.URL); err != nil || req.URL == "" {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}

	if err := isBlockedURL(req.URL); err != nil {
		http.Error(w, "url not allowed", http.StatusUnprocessableEntity)
		return
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.URL)))

	// Dedup: return existing key if this URL was already shortened.
	existing, err := h.store.FindByHash(r.Context(), hash)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if existing != "" {
		writeJSON(w, http.StatusOK, createResponse{
			ShortURL: h.baseURL + "/" + existing,
			Key:      existing,
		})
		return
	}

	key, err := h.kgs.NextKey(r.Context())
	if err != nil {
		http.Error(w, "could not generate key", http.StatusServiceUnavailable)
		return
	}

	if err := h.store.Insert(r.Context(), key, req.URL, hash); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Write to cache immediately so redirects work before read replicas catch up.
	h.cache.Set(r.Context(), key, req.URL)

	writeJSON(w, http.StatusCreated, createResponse{
		ShortURL: h.baseURL + "/" + key,
		Key:      key,
	})
}

func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	// Cache hit — skip DB entirely.
	if url, err := h.cache.Get(r.Context(), key); err == nil && url != "" {
		http.Redirect(w, r, url, http.StatusFound)
		h.producer.PublishClick(key, ipFromRequest(r), r.UserAgent(), r.Referer())
		return
	}

	originalURL, err := h.store.FindByKey(r.Context(), key)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if originalURL == "" {
		http.NotFound(w, r)
		return
	}

	// Populate cache for subsequent redirects.
	h.cache.Set(r.Context(), key, originalURL)

	http.Redirect(w, r, originalURL, http.StatusFound)
	h.producer.PublishClick(key, ipFromRequest(r), r.UserAgent(), r.Referer())
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dbErr := h.store.Ping(ctx)
	redisErr := h.cache.Ping(ctx)

	status := http.StatusOK
	if dbErr != nil || redisErr != nil {
		status = http.StatusServiceUnavailable
	}

	resp := map[string]any{"status": "ok"}
	if dbErr != nil {
		resp["status"] = "degraded"
		resp["db"] = dbErr.Error()
	}
	if redisErr != nil {
		resp["status"] = "degraded"
		resp["redis"] = redisErr.Error()
	}

	writeJSON(w, status, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
