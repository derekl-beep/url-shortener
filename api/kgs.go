package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type KGSClient struct {
	baseURL string
	http    *http.Client
}

func NewKGSClient(baseURL string) *KGSClient {
	return &KGSClient{baseURL: baseURL, http: &http.Client{}}
}

func (k *KGSClient) NextKey(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.baseURL+"/key", nil)
	if err != nil {
		return "", err
	}
	resp, err := k.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("KGS returned %d", resp.StatusCode)
	}

	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Key, nil
}
