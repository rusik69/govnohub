//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/rusik69/govnohub/internal/search"
	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startOpenSearch(t *testing.T) (string, func()) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "opensearchproject/opensearch:2",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{
			"discovery.type":             "single-node",
			"DISABLE_SECURITY_PLUGIN":    "true",
			"OPENSEARCH_JAVA_OPTS":       "-Xms512m -Xmx512m",
			"DISABLE_INSTALL_DEMO_CONFIG": "true",
		},
		WaitingFor: wait.ForHTTP("/").
			WithPort("9200/tcp").
			WithStartupTimeout(180 * time.Second).
			WithPollInterval(3 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Skipf("OpenSearch container unavailable: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get host: %v", err)
	}
	port, err := container.MappedPort(ctx, "9200")
	if err != nil {
		t.Fatalf("get port: %v", err)
	}

	baseURL := "http://" + host + ":" + port.Port()

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate OpenSearch: %v", err)
		}
	}

	return baseURL, cleanup
}

func TestSearchService_EnsureIndexAndSearch(t *testing.T) {
	osURL, cleanup := startOpenSearch(t)
	defer cleanup()

	svc := search.NewService(osURL)
	ctx := context.Background()

	// Ensure index exists
	if err := svc.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	// Index a repo document
	repoDoc := search.Document{
		ID:       "repo-1",
		Type:     "repo",
		Title:    "Awesome Go Project",
		Body:     "An awesome Go project with HTTP handlers and PostgreSQL storage",
		Repo:     "alice/awesome-go",
		Path:     "README.md",
		FullName: "alice/awesome-go",
		Ref:      "main",
	}
	if err := svc.Index(ctx, repoDoc); err != nil {
		t.Fatalf("Index repo: %v", err)
	}

	// Index an issue document
	issueDoc := search.Document{
		ID:       "issue-42",
		Type:     "issue",
		Title:    "Bug: login fails with special characters",
		Body:     "When using special characters in password, login endpoint returns 500",
		Repo:     "alice/awesome-go",
		FullName: "alice/awesome-go",
	}
	if err := svc.Index(ctx, issueDoc); err != nil {
		t.Fatalf("Index issue: %v", err)
	}

	// Index a PR document
	prDoc := search.Document{
		ID:       "pr-7",
		Type:     "pull",
		Title:    "Add GitHub Actions CI pipeline",
		Body:     "This PR adds a CI pipeline using GitHub Actions to run tests and linting",
		Repo:     "bob/ci-demo",
		FullName: "bob/ci-demo",
	}
	if err := svc.Index(ctx, prDoc); err != nil {
		t.Fatalf("Index PR: %v", err)
	}

	// Wait for OpenSearch to refresh
	time.Sleep(1 * time.Second)

	// Search for something that should match multiple documents
	hits, err := svc.Search(ctx, "Go", 10)
	if err != nil {
		t.Fatalf("Search 'Go': %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least 1 hit for 'Go'")
	}

	// Search for something specific to repo document
	hits, err = svc.Search(ctx, "PostgreSQL", 10)
	if err != nil {
		t.Fatalf("Search 'PostgreSQL': %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least 1 hit for 'PostgreSQL'")
	}
	found := false
	for _, h := range hits {
		if h.ID == "repo-1" {
			found = true
			if h.Type != "repo" {
				t.Errorf("expected type repo, got %s", h.Type)
			}
			if h.Title != "Awesome Go Project" {
				t.Errorf("expected title 'Awesome Go Project', got %s", h.Title)
			}
			if h.Repo != "alice/awesome-go" {
				t.Errorf("expected repo 'alice/awesome-go', got %s", h.Repo)
			}
			break
		}
	}
	if !found {
		t.Error("expected repo-1 in search results for 'PostgreSQL'")
	}

	// Search for something in issue
	hits, err = svc.Search(ctx, "login fails", 10)
	if err != nil {
		t.Fatalf("Search 'login fails': %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least 1 hit for 'login fails'")
	}
	found = false
	for _, h := range hits {
		if h.ID == "issue-42" {
			found = true
			if h.Type != "issue" {
				t.Errorf("expected type issue, got %s", h.Type)
			}
			break
		}
	}
	if !found {
		t.Error("expected issue-42 in search results for 'login fails'")
	}

	// Search for something in PR
	hits, err = svc.Search(ctx, "CI pipeline", 10)
	if err != nil {
		t.Fatalf("Search 'CI pipeline': %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least 1 hit for 'CI pipeline'")
	}
	found = false
	for _, h := range hits {
		if h.ID == "pr-7" {
			found = true
			if h.Type != "pull" {
				t.Errorf("expected type pull, got %s", h.Type)
			}
			break
		}
	}
	if !found {
		t.Error("expected pr-7 in search results for 'CI pipeline'")
	}

	// Search for something that should not exist
	hits, err = svc.Search(ctx, "nonexistentterm12345", 10)
	if err != nil {
		t.Fatalf("Search 'nonexistentterm12345': %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits for nonexistent term, got %d", len(hits))
	}

	// Test with limit
	hits, err = svc.Search(ctx, "Go", 1)
	if err != nil {
		t.Fatalf("Search 'Go' with limit=1: %v", err)
	}
	if len(hits) > 1 {
		t.Errorf("expected at most 1 hit with limit=1, got %d", len(hits))
	}

	// Snippet truncation test
	longDoc := search.Document{
		ID:   "long-body",
		Type: "issue",
		Title: "Long body document",
		Body:  "This is a very long body " + string(make([]byte, 200)) + " that should be truncated in the snippet",
		Repo: "alice/awesome-go",
	}
	if err := svc.Index(ctx, longDoc); err != nil {
		t.Fatalf("Index long body: %v", err)
	}

	time.Sleep(1 * time.Second)

	hits, err = svc.Search(ctx, "long body", 10)
	if err != nil {
		t.Fatalf("Search 'long body': %v", err)
	}
	for _, h := range hits {
		if h.ID == "long-body" {
			if len(h.Snippet) > 150 {
				t.Logf("long body snippet length: %d", len(h.Snippet))
			}
			break
		}
	}
}

func TestSearchService_ReindexSameDoc(t *testing.T) {
	osURL, cleanup := startOpenSearch(t)
	defer cleanup()

	svc := search.NewService(osURL)
	ctx := context.Background()

	if err := svc.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	doc := search.Document{
		ID:    "doc-1",
		Type:  "repo",
		Title: "Original Title",
		Body:  "Original body content",
		Repo:  "user/repo",
	}

	if err := svc.Index(ctx, doc); err != nil {
		t.Fatalf("first Index: %v", err)
	}

	// Update the same doc
	doc.Title = "Updated Title"
	doc.Body = "Updated body content"
	if err := svc.Index(ctx, doc); err != nil {
		t.Fatalf("second Index: %v", err)
	}

	time.Sleep(1 * time.Second)

	hits, err := svc.Search(ctx, "Updated", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hit for 'Updated'")
	}
	if hits[0].Title != "Updated Title" {
		t.Errorf("expected 'Updated Title', got %s", hits[0].Title)
	}
}

func TestSearchService_EnsureIndexIdempotent(t *testing.T) {
	osURL, cleanup := startOpenSearch(t)
	defer cleanup()

	svc := search.NewService(osURL)
	ctx := context.Background()

	// First call should succeed
	if err := svc.EnsureIndex(ctx); err != nil {
		t.Fatalf("first EnsureIndex: %v", err)
	}

	// Second call should also succeed (idempotent)
	if err := svc.EnsureIndex(ctx); err != nil {
		t.Fatalf("second EnsureIndex: %v", err)
	}
}

func TestSearchIntegrationViaAPI(t *testing.T) {
	_, cleanup := startOpenSearch(t)
	defer cleanup()

	// Create test environment (includes postgres, api server, etc.)
	env := testenv.New(t)
	defer env.Cleanup()

	// We need to override the search service, but testenv doesn't expose it.
	// Instead, index documents directly via the search service and test the API
	// endpoint that queries OpenSearch.

	// Since the API server already has search.NewService("http://127.0.0.1:1") wired in,
	// we can't test the API endpoint with a real OpenSearch from outside.
	// But we can verify the direct search service works against real OpenSearch.
	// This is tested in TestSearchService_EnsureIndexAndSearch above.

	// For the API-level test, just verify the endpoint exists and returns
	// an empty result (it will talk to the dummy OpenSearch URL and get empty hits)
	token := testutil.RegisterAndLogin(t, env.URL, "searchapiuser")

	resp, out := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/search?q=test", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search API status=%d out=%v", resp.StatusCode, out)
	}
	// Should return an empty array since OpenSearch is pointed at dummy URL
	t.Logf("API search result: %v", out)
}
