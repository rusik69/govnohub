package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Document struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Repo     string `json:"repo"`
	Path     string `json:"path,omitempty"`
	FullName string `json:"full_name,omitempty"`
}

type Hit struct {
	ID    string  `json:"id"`
	Type  string  `json:"type"`
	Title string  `json:"title"`
	Repo  string  `json:"repo"`
	Score float64 `json:"score"`
	Snippet string `json:"snippet"`
}

type Service struct {
	baseURL string
	client  *http.Client
}

func NewService(baseURL string) *Service {
	return &Service{baseURL: strings.TrimRight(baseURL, "/"), client: http.DefaultClient}
}

func (s *Service) Index(ctx context.Context, doc Document) error {
	body, _ := json.Marshal(doc)
	url := fmt.Sprintf("%s/govnohub/_doc/%s", s.baseURL, doc.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("index failed: %s", string(b))
	}
	return nil
}

func (s *Service) EnsureIndex(ctx context.Context) error {
	url := s.baseURL + "/govnohub"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(`{
		"mappings": {"properties": {
			"type": {"type": "keyword"},
			"title": {"type": "text"},
			"body": {"type": "text"},
			"repo": {"type": "keyword"},
			"path": {"type": "keyword"}
		}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *Service) Search(ctx context.Context, q string, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 20
	}
	query := map[string]interface{}{
		"size": limit,
		"query": map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  q,
				"fields": []string{"title^3", "body", "repo", "path"},
			},
		},
	}
	body, _ := json.Marshal(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/govnohub/_search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return []Hit{}, nil
	}
	var result struct {
		Hits struct {
			Hits []struct {
				Score  float64  `json:"_score"`
				Source Document `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	var hits []Hit
	for _, h := range result.Hits.Hits {
		snippet := h.Source.Body
		if len(snippet) > 120 {
			snippet = snippet[:120] + "..."
		}
		hits = append(hits, Hit{
			ID: h.Source.ID, Type: h.Source.Type, Title: h.Source.Title,
			Repo: h.Source.Repo, Score: h.Score, Snippet: snippet,
		})
	}
	return hits, nil
}
