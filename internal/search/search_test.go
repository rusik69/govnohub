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
		query, ok := reqBody["query"].(map[string]interface{})
		if !ok {
			t.Fatal("expected query field")
		}
		mm, ok := query["multi_match"].(map[string]interface{})
		if !ok {
			t.Fatal("expected multi_match")
		}
		if mm["query"] != "test" {
			t.Errorf("expected query 'test', got %v", mm["query"])
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"hits": {
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
	hits, err := s.Search(context.Background(), "test", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].ID != "repo-1" || hits[0].Score != 2.5 || hits[0].Title != "Test Repo" {
		t.Errorf("first hit mismatch: %+v", hits[0])
	}
	if hits[0].Ref != "main" {
		t.Errorf("expected ref main, got %s", hits[0].Ref)
	}
	if hits[0].Snippet != "This is a test repository with some content for searching" {
		t.Errorf("unexpected snippet: %s", hits[0].Snippet)
	}
	if hits[1].ID != "issue-42" || hits[1].Type != "issue" {
		t.Errorf("second hit mismatch: %+v", hits[1])
	}
	// Short body should not be truncated
	if hits[1].Snippet != "Short body" {
		t.Errorf("unexpected snippet: %s", hits[1].Snippet)
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"hits": {"hits": []}}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	hits, err := s.Search(context.Background(), "nonexistent", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(hits))
	}
}

func TestSearch_ErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	hits, err := s.Search(context.Background(), "test", 10)
	// The current implementation returns empty hits on error, no error
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits on error, got %d", len(hits))
	}
}

func TestSearch_DefaultLimit(t *testing.T) {
	var gotBody map[string]interface{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"hits": {"hits": []}}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	// Call with limit <= 0, should default to 20
	_, err := s.Search(context.Background(), "test", 0)
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
		w.Write([]byte(`{"hits": {"hits": [{"_score": 1.0, "_source": {"id": "1", "type": "doc", "title": "x", "body": "` + longBody + `", "repo": "a/b"}}]}}`))
	}))
	defer ts.Close()

	s := NewService(ts.URL)
	hits, err := s.Search(context.Background(), "test", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	snippet := hits[0].Snippet
	if len(snippet) != 123 { // 120 + "..."
		t.Errorf("expected snippet length 123, got %d: %q", len(snippet), snippet)
	}
	if !strings.HasSuffix(snippet, "...") {
		t.Errorf("expected snippet to end with ...")
	}
}
