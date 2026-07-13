package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewService(t *testing.T) {
	s := NewService("http://localhost:9200/")
	if !strings.HasSuffix(s.baseURL, "9200") {
		t.Fatalf("expected baseURL to end with 9200, got %s", s.baseURL)
	}
	if s.client != http.DefaultClient {
		t.Fatal("expected default HTTP client")
	}
}

func TestIndex_Success(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]interface{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"result": "created"}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	err := s.Index(context.Background(), Document{
		ID:   "repo-1",
		Type: "repo",
		Title: "test-repo",
		Repo: "user/test-repo",
		Body: "A test repository",
		Path: "README.md",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "/govnohub/_doc/repo-1" {
		t.Errorf("expected /govnohub/_doc/repo-1, got %s", gotPath)
	}
	if gotBody["id"] != "repo-1" {
		t.Errorf("expected id repo-1, got %v", gotBody["id"])
	}
	if gotBody["type"] != "repo" {
		t.Errorf("expected type repo, got %v", gotBody["type"])
	}
}

func TestIndex_ErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "mapper_parsing_exception"}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	err := s.Index(context.Background(), Document{ID: "bad-doc"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mapper_parsing_exception") {
		t.Errorf("expected mapper_parsing_exception in error, got %s", err.Error())
	}
}

func TestIndex_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal_error"}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	err := s.Index(context.Background(), Document{ID: "fail"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEnsureIndex_Success(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"acknowledged": true}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	err := s.EnsureIndex(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "/govnohub" {
		t.Errorf("expected /govnohub, got %s", gotPath)
	}
}

func TestEnsureIndex_AlreadyExists(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 400 with index_already_exists_exception is ignored by the current code
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "index_already_exists_exception"}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	err := s.EnsureIndex(context.Background())
	if err != nil {
		t.Fatalf("expected no error for already exists, got %v", err)
	}
}

func TestSearch_FoundResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/govnohub/_search" {
			t.Errorf("expected /govnohub/_search, got %s", r.URL.Path)
		}
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		// Verify query structure
		q, ok := reqBody["query"].(map[string]interface{})
		if !ok {
			t.Fatal("expected query field")
		}
		boolQ, ok := q["bool"].(map[string]interface{})
		if !ok {
			t.Fatal("expected bool query")
		}
		must, ok := boolQ["must"].([]interface{})
		if !ok || len(must) != 1 {
			t.Fatal("expected must array with one element")
		}
		mm, ok := must[0].(map[string]interface{})
		if !ok {
			t.Fatal("expected multi_match in must[0]")
		}
		mmInner, ok := mm["multi_match"].(map[string]interface{})
		if !ok {
			t.Fatal("expected multi_match map")
		}
		if mmInner["query"] != "test" {
			t.Errorf("expected query 'test', got %v", mmInner["query"])
		}

		// Check track_total_hits is present
		if reqBody["track_total_hits"] != true {
			t.Error("expected track_total_hits to be true")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"hits": {
				"total": {"value": 2},
				"hits": [
					{
						"_score": 2.5,
						"_source": {
							"id": "repo-1",
							"type": "repo",
							"title": "Test Repo",
							"body": "This is a test repository with some content for searching",
							"repo": "user/test-repo",
							"ref": "main"
						}
					},
					{
						"_score": 1.2,
						"_source": {
							"id": "issue-42",
							"type": "issue",
							"title": "Bug: something broken",
							"body": "Short body",
							"repo": "user/test-repo"
						}
					}
				]
			}
		}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	result, err := s.Search(context.Background(), "test", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Fatalf("expected total 2, got %d", result.Total)
	}
	if len(result.Hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(result.Hits))
	}
	if result.Hits[0].ID != "repo-1" || result.Hits[0].Score != 2.5 || result.Hits[0].Title != "Test Repo" {
		t.Errorf("first hit mismatch: %+v", result.Hits[0])
	}
	if result.Hits[0].Ref != "main" {
		t.Errorf("expected ref main, got %s", result.Hits[0].Ref)
	}
	if result.Hits[0].Snippet != "This is a test repository with some content for searching" {
		t.Errorf("unexpected snippet: %s", result.Hits[0].Snippet)
	}
	if result.Hits[1].ID != "issue-42" || result.Hits[1].Type != "issue" {
		t.Errorf("second hit mismatch: %+v", result.Hits[1])
	}
	// Short body should not be truncated
	if result.Hits[1].Snippet != "Short body" {
		t.Errorf("unexpected snippet: %s", result.Hits[1].Snippet)
	}
}

func TestSearch_FilterByType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		q, ok := reqBody["query"].(map[string]interface{})
		if !ok {
			t.Fatal("expected query field")
		}
		boolQ, ok := q["bool"].(map[string]interface{})
		if !ok {
			t.Fatal("expected bool query")
		}
		filter, ok := boolQ["filter"].([]interface{})
		if !ok {
			t.Fatal("expected filter array")
		}
		if len(filter) != 1 {
			t.Fatalf("expected 1 filter, got %d", len(filter))
		}
		term, ok := filter[0].(map[string]interface{})
		if !ok {
			t.Fatal("expected term filter")
		}
		termInner, ok := term["term"].(map[string]interface{})
		if !ok {
			t.Fatal("expected term inner")
		}
		if termInner["type"] != "issue" {
			t.Errorf("expected type=issue, got %v", termInner["type"])
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"hits": {"total": {"value": 1}, "hits": [{"_score": 1.0, "_source": {"id": "issue-1", "type": "issue", "title": "Bug", "body": "", "repo": "a/b"}}]}}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	result, err := s.Search(context.Background(), "test", SearchOptions{Limit: 10, Type: "issue"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected total 1, got %d", result.Total)
	}
	if len(result.Hits) != 1 {
		t.Errorf("expected 1 hit, got %d", len(result.Hits))
	}
	if result.Hits[0].Type != "issue" {
		t.Errorf("expected type issue, got %s", result.Hits[0].Type)
	}
}

func TestSearch_WithOffset(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		if reqBody["from"] != float64(20) {
			t.Errorf("expected from=20, got %v", reqBody["from"])
		}
		if reqBody["size"] != float64(10) {
			t.Errorf("expected size=10, got %v", reqBody["size"])
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"hits": {"total": {"value": 0}, "hits": []}}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	_, err := s.Search(context.Background(), "test", SearchOptions{Limit: 10, Offset: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"hits": {"total": {"value": 0}, "hits": []}}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	result, err := s.Search(context.Background(), "nonexistent", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected total 0, got %d", result.Total)
	}
	if len(result.Hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(result.Hits))
	}
}

func TestSearch_ErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	result, err := s.Search(context.Background(), "test", SearchOptions{Limit: 10})
	// The current implementation returns empty hits on error, no error
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Hits) != 0 {
		t.Errorf("expected 0 hits on error, got %d", len(result.Hits))
	}
}

func TestSearch_DefaultLimit(t *testing.T) {
	var gotBody map[string]interface{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"hits": {"total": {"value": 0}, "hits": []}}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	// Call with limit <= 0, should default to 20
	_, err := s.Search(context.Background(), "test", SearchOptions{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody["size"] != float64(20) {
		t.Errorf("expected size 20, got %v", gotBody["size"])
	}
}

func TestSearch_TruncatesLongSnippet(t *testing.T) {
	longBody := strings.Repeat("a", 200)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"hits": {"total": {"value": 1}, "hits": [{"_score": 1.0, "_source": {"id": "1", "type": "doc", "title": "x", "body": "` + longBody + `", "repo": "a/b"}}]}}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	result, err := s.Search(context.Background(), "test", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(result.Hits))
	}
	snippet := result.Hits[0].Snippet
	if len(snippet) != 123 { // 120 + "..."
		t.Errorf("expected snippet length 123, got %d: %q", len(snippet), snippet)
	}
	if !strings.HasSuffix(snippet, "...") {
		t.Errorf("expected snippet to end with ...")
	}
}
