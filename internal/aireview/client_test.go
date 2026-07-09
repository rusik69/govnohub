package aireview

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientReview(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "deepseek-v4-flash" {
			http.Error(w, "bad model", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Role: "assistant", Content: "Looks good. Verdict: APPROVE"}}},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{Enabled: true, APIKey: "test-key", BaseURL: srv.URL, Model: "deepseek-v4-flash"})
	out, err := c.Review(context.Background(), ReviewInput{Title: "Fix bug", Body: "details", Diff: "+fix"})
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty review")
	}
}

func TestClientDisabled(t *testing.T) {
	c := NewClient(Config{Enabled: false})
	_, err := c.Review(context.Background(), ReviewInput{Title: "x", Diff: "y"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClientCustomModel(t *testing.T) {
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotModel = req.Model
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{Enabled: true, APIKey: "k", BaseURL: srv.URL, Model: "deepseek-v4-flash"})
	_, err := c.Review(context.Background(), ReviewInput{Title: "t", Diff: "d", Model: "deepseek-v4-pro"})
	if err != nil {
		t.Fatal(err)
	}
	if gotModel != "deepseek-v4-pro" {
		t.Fatalf("model=%s", gotModel)
	}
}
