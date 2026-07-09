package search

import (
	"strings"
	"testing"
)

func TestNewService(t *testing.T) {
	s := NewService("http://localhost:9200/")
	if !strings.HasSuffix(s.baseURL, "9200") {
		t.Fatalf("base=%s", s.baseURL)
	}
}

func TestDocumentFields(t *testing.T) {
	d := Document{ID: "1", Type: "repo", Title: "demo", Repo: "a/b"}
	if d.Type != "repo" {
		t.Fatal()
	}
}
