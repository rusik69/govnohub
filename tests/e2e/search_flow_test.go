//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"strings"
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
			"discovery.type":              "single-node",
			"DISABLE_SECURITY_PLUGIN":     "true",
			"OPENSEARCH_JAVA_OPTS":        "-Xms512m -Xmx512m",
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

func TestSearchFlow_RepoIssuePRContent(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "srch" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)

	// Create a repo via the API to verify the full stack
	createRepo(t, base, token, owner, "my-project")
	repoFullName := owner + "/my-project"

	// Start OpenSearch container
	osURL, cleanup := startOpenSearch(t)
	defer cleanup()

	svc := search.NewService(osURL)
	ctx := context.Background()

	// Ensure index exists
	if err := svc.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	// ---- Index documents (simulating what search-indexer does) ----

	// Index a repo document
	repoDoc := search.Document{
		ID:       "repo-" + owner + "_my-project",
		Type:     "repo",
		Title:    repoFullName,
		Body:     "A Go project with REST API and PostgreSQL database for managing tasks and issues",
		Repo:     repoFullName,
		FullName: repoFullName,
		Ref:      "main",
	}
	if err := svc.Index(ctx, repoDoc); err != nil {
		t.Fatalf("Index repo: %v", err)
	}

	// Index an issue document
	issueDoc := search.Document{
		ID:   "issue-42",
		Type: "issue",
		Title: "Bug: login page crashes with empty password",
		Body: "When submitting the login form with an empty password field, " +
			"the server returns a 500 error instead of a proper validation error. " +
			"This affects all users who accidentally submit the form without filling in the password.",
		Repo: repoFullName,
	}
	if err := svc.Index(ctx, issueDoc); err != nil {
		t.Fatalf("Index issue: %v", err)
	}

	// Index a PR document
	prDoc := search.Document{
		ID:    "pr-7",
		Type:  "pull_request",
		Title: "Add GitHub Actions CI workflow",
		Body:  "This PR adds a CI pipeline that runs tests and linting on every push using GitHub Actions",
		Repo:  repoFullName,
	}
	if err := svc.Index(ctx, prDoc); err != nil {
		t.Fatalf("Index PR: %v", err)
	}

	// Wait for OpenSearch to refresh
	time.Sleep(1 * time.Second)

	// ---- Test 1: Search for repo content ----
	t.Run("search repo content", func(t *testing.T) {
		hits, err := svc.Search(ctx, "PostgreSQL", 10)
		if err != nil {
			t.Fatalf("Search 'PostgreSQL': %v", err)
		}
		if len(hits) == 0 {
			t.Fatal("expected at least 1 hit for 'PostgreSQL'")
		}
		var found bool
		for _, h := range hits {
			if h.ID == "repo-"+owner+"_my-project" {
				found = true
				if h.Type != "repo" {
					t.Errorf("expected type 'repo', got %s", h.Type)
				}
				if h.Title != repoFullName {
					t.Errorf("expected title %q, got %q", repoFullName, h.Title)
				}
				if h.Repo != repoFullName {
					t.Errorf("expected repo %q, got %q", repoFullName, h.Repo)
				}
				if h.Ref != "main" {
					t.Errorf("expected ref 'main', got %s", h.Ref)
				}
				break
			}
		}
		if !found {
			t.Error("repo document not found in search results for 'PostgreSQL'")
		}
	})

	// ---- Test 2: Search for issue content ----
	t.Run("search issue content", func(t *testing.T) {
		hits, err := svc.Search(ctx, "login page crashes", 10)
		if err != nil {
			t.Fatalf("Search 'login page crashes': %v", err)
		}
		if len(hits) == 0 {
			t.Fatal("expected at least 1 hit for 'login page crashes'")
		}
		var found bool
		for _, h := range hits {
			if h.ID == "issue-42" {
				found = true
				if h.Type != "issue" {
					t.Errorf("expected type 'issue', got %s", h.Type)
				}
				if h.Title != "Bug: login page crashes with empty password" {
					t.Errorf("expected title about login crash, got %q", h.Title)
				}
				if h.Repo != repoFullName {
					t.Errorf("expected repo %q, got %q", repoFullName, h.Repo)
				}
				break
			}
		}
		if !found {
			t.Error("issue document not found in search results for 'login page crashes'")
		}
	})

	// ---- Test 3: Search for PR content ----
	t.Run("search PR content", func(t *testing.T) {
		hits, err := svc.Search(ctx, "CI pipeline", 10)
		if err != nil {
			t.Fatalf("Search 'CI pipeline': %v", err)
		}
		if len(hits) == 0 {
			t.Fatal("expected at least 1 hit for 'CI pipeline'")
		}
		var found bool
		for _, h := range hits {
			if h.ID == "pr-7" {
				found = true
				if h.Type != "pull_request" {
					t.Errorf("expected type 'pull_request', got %s", h.Type)
				}
				if h.Title != "Add GitHub Actions CI workflow" {
					t.Errorf("expected title about CI workflow, got %q", h.Title)
				}
				if h.Repo != repoFullName {
					t.Errorf("expected repo %q, got %q", repoFullName, h.Repo)
				}
				break
			}
		}
		if !found {
			t.Error("PR document not found in search results for 'CI pipeline'")
		}
	})

	// ---- Test 4: Cross-type search ----
	t.Run("cross-type search matches multiple types", func(t *testing.T) {
		// "Go" should match the repo title
		hits, err := svc.Search(ctx, "Go", 10)
		if err != nil {
			t.Fatalf("Search 'Go': %v", err)
		}
		if len(hits) == 0 {
			t.Fatal("expected at least 1 hit for 'Go'")
		}
		// Should find the repo document
		var foundRepo bool
		for _, h := range hits {
			if h.ID == "repo-"+owner+"_my-project" {
				foundRepo = true
				break
			}
		}
		if !foundRepo {
			t.Error("expected repo in search results for 'Go'")
		}
	})

	// ---- Test 5: Nonexistent search ----
	t.Run("nonexistent term returns empty", func(t *testing.T) {
		hits, err := svc.Search(ctx, "nonexistentterm12345searchtest", 10)
		if err != nil {
			t.Fatalf("Search 'nonexistentterm12345searchtest': %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("expected 0 hits for nonexistent term, got %d", len(hits))
		}
	})

	// ---- Test 6: Result limit ----
	t.Run("search respects limit", func(t *testing.T) {
		hits, err := svc.Search(ctx, "Go", 1)
		if err != nil {
			t.Fatalf("Search 'Go' with limit=1: %v", err)
		}
		if len(hits) > 1 {
			t.Errorf("expected at most 1 hit with limit=1, got %d", len(hits))
		}
	})

	// ---- Test 7: Snippet truncation ----
	t.Run("long body snippet truncated", func(t *testing.T) {
		longBody := strings.Repeat("this is a very long body that should be truncated ", 20)
		longDoc := search.Document{
			ID:    "long-body-doc",
			Type:  "issue",
			Title: "Issue with very long body",
			Body:  longBody,
			Repo:  repoFullName,
		}
		if err := svc.Index(ctx, longDoc); err != nil {
			t.Fatalf("Index long body: %v", err)
		}
		time.Sleep(500 * time.Millisecond)

		hits, err := svc.Search(ctx, "truncated", 10)
		if err != nil {
			t.Fatalf("Search 'truncated': %v", err)
		}
		for _, h := range hits {
			if h.ID == "long-body-doc" {
				if len(h.Snippet) > 150 {
					t.Logf("long body snippet length: %d", len(h.Snippet))
				}
				if len(h.Snippet) > 0 && len(h.Snippet) > 150 {
					t.Errorf("snippet too long: %d chars", len(h.Snippet))
				}
				break
			}
		}
	})

	// ---- Test 8: API search endpoint ----
	t.Run("API search endpoint returns 200", func(t *testing.T) {
		// The API server's search service is wired to a dummy URL in testenv,
		// so results will be empty. But we can verify the endpoint exists.
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/search?q=Go", token, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("search API: status=%d, out=%v", resp.StatusCode, out)
		}
		t.Logf("API search returned %v (expected empty due to dummy OpenSearch in testenv)", out)
	})

	// ---- Test 9: Search for code/repo names ----
	t.Run("search repo by full name", func(t *testing.T) {
		hits, err := svc.Search(ctx, repoFullName, 10)
		if err != nil {
			t.Fatalf("Search by repo name: %v", err)
		}
		if len(hits) == 0 {
			t.Fatal("expected hits when searching by repo full name")
		}
	})
}
