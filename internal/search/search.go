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
	Ref     string `json:"ref,omitempty"`
}

type Hit struct {
	ID      string  `json:"id"`
	Type    string  `json:"type"`
	Title   string  `json:"title"`
	Repo    string  `json:"repo"`
	Ref     string  `json:"ref,omitempty"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

type SearchOptions struct {
	Limit  int
	Offset int
	Type   string // "repo", "issue", "pull_request", "wiki", or "" for all
	Sort   string // "best_match" (default) or "recently_updated"
}

type SearchResult struct {
	Hits  []Hit
	Total int
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

func (s *Service) Search(ctx context.Context, q string, opts SearchOptions) (SearchResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	if opts.Sort == "" {
		opts.Sort = "best_match"
	}

	// Build the OpenSearch query
	must := []map[string]interface{}{
		{
			"multi_match": map[string]interface{}{
				"query":  q,
				"fields": []string{"title^3", "body", "repo", "path"},
			},
		},
	}

	filter := []map[string]interface{}{}
	if opts.Type != "" {
		filter = append(filter, map[string]interface{}{
			"term": map[string]interface{}{
				"type": opts.Type,
			},
		})
	}

	queryBody := map[string]interface{}{
		"size": opts.Limit,
		"from": opts.Offset,
		"track_total_hits": true,
	}

	// Build bool query
	boolQuery := map[string]interface{}{
		"must": must,
	}
	if len(filter) > 0 {
		boolQuery["filter"] = filter
	}
	queryBody["query"] = map[string]interface{}{
		"bool": boolQuery,
	}

	// Sort
	if opts.Sort == "recently_updated" {
		queryBody["sort"] = []map[string]interface{}{
			{"updated_at": map[string]string{"order": "desc"}},
		}
	}
	// best_match = default scoring (no explicit sort needed, ES uses _score by default)

	body, _ := json.Marshal(queryBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/govnohub/_search", bytes.NewReader(body))
	if err != nil {
		return SearchResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return SearchResult{}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return SearchResult{}, nil
	}
	var result struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []struct {
				Score  float64  `json:"_score"`
				Source Document `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return SearchResult{}, nil
	}
	var hits []Hit
	for _, h := range result.Hits.Hits {
		snippet := h.Source.Body
		if len(snippet) > 120 {
			snippet = snippet[:120] + "..."
		}
		hits = append(hits, Hit{
			ID: h.Source.ID, Type: h.Source.Type, Title: h.Source.Title,
			Repo: h.Source.Repo, Ref: h.Source.Ref, Score: h.Score, Snippet: snippet,
		})
	}
	return SearchResult{Hits: hits, Total: result.Hits.Total.Value}, nil
}
