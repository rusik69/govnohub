//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestWikiCRUD(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "wikiuser")
	owner := "wikiuser"

	// Create a repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "wikitest", "description": "repo for wiki tests", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// List wiki pages (should be empty initially)
	reqInit, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki", nil)
	reqInit.Header.Set("Authorization", "Bearer "+token)
	respInit, err := http.DefaultClient.Do(reqInit)
	if err != nil {
		t.Fatal(err)
	}
	if respInit.StatusCode != http.StatusOK {
		t.Fatalf("list wiki pages (initial) status=%d", respInit.StatusCode)
	}
	var initialPages []map[string]any
	json.NewDecoder(respInit.Body).Decode(&initialPages)
	respInit.Body.Close()
	if len(initialPages) != 0 {
		t.Errorf("expected empty wiki, got %d pages", len(initialPages))
	}

	// Create a wiki page via PUT with slug
	resp, out = testutil.DoJSON(t, http.MethodPut, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki/home", token, map[string]any{
		"title":   "Home",
		"content": "Welcome to the wiki",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create wiki page status=%d out=%v", resp.StatusCode, out)
	}
	checkWikiPage(t, out, "home", "Home", "Welcome to the wiki")
	firstID, _ := out["id"].(string)
	if firstID == "" {
		t.Fatal("missing id in wiki page response")
	}

	// Create a second wiki page via PUT without slug (slug is in URL)
	resp, out = testutil.DoJSON(t, http.MethodPut, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki/guide", token, map[string]any{
		"title":   "User Guide",
		"content": "How to use this project",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create second wiki page status=%d out=%v", resp.StatusCode, out)
	}
	checkWikiPage(t, out, "guide", "User Guide", "How to use this project")

	// Create a wiki page with PUT using "new" slug (should use slug from body)
	resp, out = testutil.DoJSON(t, http.MethodPut, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki/new", token, map[string]any{
		"slug":    "about",
		"title":   "About",
		"content": "About this project",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create wiki page with new slug status=%d out=%v", resp.StatusCode, out)
	}
	checkWikiPage(t, out, "about", "About", "About this project")

	// Get a specific wiki page by slug
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki/home", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get wiki page status=%d out=%v", resp.StatusCode, out)
	}
	if title, _ := out["title"].(string); title != "Home" {
		t.Errorf("title=%q want Home", title)
	}
	if content, _ := out["content"].(string); content != "Welcome to the wiki" {
		t.Errorf("content=%q want 'Welcome to the wiki'", content)
	}

	// Get a nonexistent wiki page should return 404
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki/nonexistent", token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get nonexistent wiki page: expected 404, got %d out=%v", resp.StatusCode, out)
	}

	// Update an existing wiki page
	resp, out = testutil.DoJSON(t, http.MethodPut, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki/home", token, map[string]any{
		"title":   "Home Updated",
		"content": "Updated welcome content",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update wiki page status=%d out=%v", resp.StatusCode, out)
	}
	checkWikiPage(t, out, "home", "Home Updated", "Updated welcome content")
	if id, _ := out["id"].(string); id != firstID {
		t.Errorf("update should keep same id: got %q want %q", id, firstID)
	}

	// List wiki pages (should have 3 pages)
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	respBody, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer respBody.Body.Close()
	if respBody.StatusCode != http.StatusOK {
		t.Fatalf("list wiki pages status=%d", respBody.StatusCode)
	}
	var pages []map[string]any
	if err := json.NewDecoder(respBody.Body).Decode(&pages); err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 {
		t.Fatalf("expected 3 wiki pages, got %d", len(pages))
	}
	slugs := make(map[string]bool)
	for _, p := range pages {
		s, _ := p["slug"].(string)
		slugs[s] = true
	}
	if !slugs["home"] {
		t.Error("missing home slug in list")
	}
	if !slugs["guide"] {
		t.Error("missing guide slug in list")
	}
	if !slugs["about"] {
		t.Error("missing about slug in list")
	}

	// Delete a wiki page
	resp, out = testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki/home", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete wiki page status=%d out=%v", resp.StatusCode, out)
	}
	if status, _ := out["status"].(string); status != "deleted" {
		t.Errorf("status=%q want 'deleted'", status)
	}

	// Verify it's gone via GET
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki/home", token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get deleted page: expected 404, got %d", resp.StatusCode)
	}

	// Verify list now has 2 pages
	req2, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	respBody2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer respBody2.Body.Close()
	if respBody2.StatusCode != http.StatusOK {
		t.Fatalf("list wiki pages after delete status=%d", respBody2.StatusCode)
	}
	var pages2 []map[string]any
	if err := json.NewDecoder(respBody2.Body).Decode(&pages2); err != nil {
		t.Fatal(err)
	}
	if len(pages2) != 2 {
		t.Fatalf("expected 2 wiki pages after delete, got %d", len(pages2))
	}

	// Delete nonexistent wiki page should return 404
	resp, out = testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/repos/"+owner+"/wikitest/wiki/nonexistent", token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete nonexistent: expected 404, got %d out=%v", resp.StatusCode, out)
	}

	// Wiki CRUD on nonexistent repo should return 404
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/norepo/wiki", token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("list wiki on nonexistent repo: expected 404, got %d", resp.StatusCode)
	}
}

func TestWikiAccessControl(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	aliceToken := testutil.RegisterAndLogin(t, env.URL, "alice_wiki")
	owner := "alice_wiki"

	// Create a private repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", aliceToken, map[string]any{
		"name": "privwiki", "description": "private wiki repo", "private": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Alice creates a wiki page in her private repo
	resp, out = testutil.DoJSON(t, http.MethodPut, env.URL+"/api/v1/repos/"+owner+"/privwiki/wiki/home", aliceToken, map[string]any{
		"title": "Home", "content": "Private wiki page",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("alice create wiki page: status=%d out=%v", resp.StatusCode, out)
	}

	// Bob should not be able to read wiki pages on Alice's private repo
	bobToken := testutil.RegisterAndLogin(t, env.URL, "bob_wiki")
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/privwiki/wiki", bobToken, nil)
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
		t.Errorf("bob list wiki on private repo: expected 404/403, got %d out=%v", resp.StatusCode, out)
	}

	// Bob should not be able to create wiki pages on Alice's private repo
	resp, out = testutil.DoJSON(t, http.MethodPut, env.URL+"/api/v1/repos/"+owner+"/privwiki/wiki/evil", bobToken, map[string]any{
		"title": "Evil", "content": "hacked",
	})
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bob create wiki on private repo: expected 404/403/401, got %d out=%v", resp.StatusCode, out)
	}

	// Alice should be able to create wiki pages on her own private repo
	resp, out = testutil.DoJSON(t, http.MethodPut, env.URL+"/api/v1/repos/"+owner+"/privwiki/wiki/guide", aliceToken, map[string]any{
		"title": "Guide", "content": "User guide",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("alice create second wiki page: expected 200, got %d out=%v", resp.StatusCode, out)
	}
}

func TestWikiPageWithSpecialContent(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "wikispc")
	owner := "wikispc"

	// Create a repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "spcwiki", "description": "special content", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Create wiki page with empty content
	resp, out = testutil.DoJSON(t, http.MethodPut, env.URL+"/api/v1/repos/"+owner+"/spcwiki/wiki/empty-content", token, map[string]any{
		"title":   "Empty Content",
		"content": "",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create wiki page with empty content status=%d out=%v", resp.StatusCode, out)
	}
	if content, _ := out["content"].(string); content != "" {
		t.Errorf("content=%q want empty", content)
	}

	// Create wiki page with markdown content
	markdown := "# Heading\n\n- List item 1\n- List item 2\n\n**bold** and *italic*"
	resp, out = testutil.DoJSON(t, http.MethodPut, env.URL+"/api/v1/repos/"+owner+"/spcwiki/wiki/markdown", token, map[string]any{
		"title":   "Markdown Test",
		"content": markdown,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create wiki page with markdown status=%d out=%v", resp.StatusCode, out)
	}
	if content, _ := out["content"].(string); content != markdown {
		t.Errorf("content mismatch, got=%q", content)
	}

	// Create wiki page with very long content
	longContent := ""
	for i := 0; i < 200; i++ {
		longContent += "Lorem ipsum dolor sit amet, consectetur adipiscing elit. "
	}
	resp, out = testutil.DoJSON(t, http.MethodPut, env.URL+"/api/v1/repos/"+owner+"/spcwiki/wiki/long", token, map[string]any{
		"title":   "Long Page",
		"content": longContent,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create wiki page with long content status=%d out=%v", resp.StatusCode, out)
	}

	// Timestamps should be present on response
	if _, ok := out["created_at"]; !ok {
		t.Error("missing created_at in wiki page response")
	}
	if _, ok := out["updated_at"]; !ok {
		t.Error("missing updated_at in wiki page response")
	}
	// Parse timestamps
	if createdAt, ok := out["created_at"].(string); ok {
		if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
			t.Errorf("invalid created_at format: %v", err)
		}
	}
	if updatedAt, ok := out["updated_at"].(string); ok {
		if _, err := time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			t.Errorf("invalid updated_at format: %v", err)
		}
	}

	// Verify author info is present
	if authorID, ok := out["author_id"].(string); !ok || authorID == "" {
		t.Error("missing or empty author_id in wiki page response")
	}
}

func checkWikiPage(t *testing.T, out map[string]any, wantSlug, wantTitle, wantContent string) {
	t.Helper()
	if slug, _ := out["slug"].(string); slug != wantSlug {
		t.Errorf("slug=%q want %q", slug, wantSlug)
	}
	if title, _ := out["title"].(string); title != wantTitle {
		t.Errorf("title=%q want %q", title, wantTitle)
	}
	if content, _ := out["content"].(string); content != wantContent {
		t.Errorf("content=%q want %q", content, wantContent)
	}
	if _, ok := out["id"]; !ok {
		t.Error("missing id")
	}
	if _, ok := out["repo_id"]; !ok {
		t.Error("missing repo_id")
	}
	if _, ok := out["created_at"]; !ok {
		t.Error("missing created_at")
	}
	if _, ok := out["updated_at"]; !ok {
		t.Error("missing updated_at")
	}
}
