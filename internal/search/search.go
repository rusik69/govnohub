package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/rusik69/govnohub/internal/httputil"
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

// Service provides search indexing and querying against OpenSearch.
// HTTP request failures are retried with exponential backoff.
type Service struct {
	baseURL    string
	client     *http.Client
	maxRetries int
	baseDelay  time.Duration
}

func NewService(baseURL string) *Service {
	return &Service{
		baseURL:    strings.TrimRight(baseURL, "/"),
		client:     httputil.NewClient(),
		maxRetries: 3,
		baseDelay:  500 * time.Millisecond,
	}
}

// doRequest performs an HTTP request with exponential backoff retry on
// network/transport errors (connection refused, DNS failure, timeouts).
// HTTP error responses (4xx, 5xx) are NOT retried — the caller handles them.
func (s *Service) doRequest(ctx context.Context, method, url string, body []byte, contentType string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: baseDelay * 2^(attempt-1) with jitter
			delay := s.baseDelay * (1 << (attempt - 1))
			jitter := time.Duration(rand.Int63n(int64(delay) / 2))
			select {
			case <-time.After(delay + jitter):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err := s.client.Do(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("request failed after %d retries: %w", s.maxRetries, lastErr)
}

func (s *Service) Index(ctx context.Context, doc Document) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal document: %w", err)
	}
	url := fmt.Sprintf("%s/govnohub/_doc/%s", s.baseURL, doc.ID)
	resp, err := s.doRequest(ctx, http.MethodPut, url, body, "application/json")
	if err != nil {
		return fmt.Errorf("index document: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("index failed (status %d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func (s *Service) EnsureIndex(ctx context.Context) error {
	mapping := `{
		"mappings": {"properties": {
			"type": {"type": "keyword"},
			"title": {"type": "text"},
			"body": {"type": "text"},
			"repo": {"type": "keyword"},
			"path": {"type": "keyword"}
		}}
	}`
	resp, err := s.doRequest(ctx, http.MethodPut, s.baseURL+"/govnohub", []byte(mapping), "application/json")
	if err != nil {
		return fmt.Errorf("ensure index: %w", err)
	}
	resp.Body.Close()
	return nil
}

func (s *Service) Search(ctx context.Context, q string, opts SearchOptions) (SearchResult, error) {
	if q == "" {
		return SearchResult{}, nil
	}
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
		"size":             opts.Limit,
		"from":             opts.Offset,
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

	body, err := json.Marshal(queryBody)
	if err != nil {
		return SearchResult{}, fmt.Errorf("marshal query: %w", err)
	}

	resp, err := s.doRequest(ctx, http.MethodPost, s.baseURL+"/govnohub/_search", body, "application/json")
	if err != nil {
		return SearchResult{}, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return SearchResult{}, fmt.Errorf("search failed (status %d): %s", resp.StatusCode, string(b))
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
		return SearchResult{}, fmt.Errorf("decode response: %w", err)
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
