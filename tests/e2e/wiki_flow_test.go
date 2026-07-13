//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestWikiFlow_BasicLifecycle(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "wfl" + short

	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "mywiki")

	// ---- List wiki pages (should be empty initially) ----
	t.Run("list wiki pages initially empty", func(t *testing.T) {
		resp, pages := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/mywiki/wiki", token, nil)
		requireStatus(t, resp, http.StatusOK, "list wiki pages")
		if len(pages) != 0 {
			t.Fatalf("expected empty wiki, got %d pages", len(pages))
		}
	})

	// ---- Create a wiki page ----
	t.Run("create wiki page", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+owner+"/mywiki/wiki/home", token, map[string]any{
			"title":   "Home",
			"content": "Welcome to my wiki",
		})
		requireStatus(t, resp, http.StatusOK, "create wiki page")
		if slug, _ := out["slug"].(string); slug != "home" {
			t.Fatalf("slug=%q want 'home'", slug)
		}
		if title, _ := out["title"].(string); title != "Home" {
			t.Fatalf("title=%q want 'Home'", title)
		}
		if content, _ := out["content"].(string); content != "Welcome to my wiki" {
			t.Fatalf("content=%q want 'Welcome to my wiki'", content)
		}
		if id, _ := out["id"].(string); id == "" {
			t.Fatal("missing id")
		}
		if _, ok := out["created_at"]; !ok {
			t.Fatal("missing created_at")
		}
		if _, ok := out["updated_at"]; !ok {
			t.Fatal("missing updated_at")
		}
		if aid, _ := out["author_id"].(string); aid == "" {
			t.Fatal("missing author_id")
		}
	})

	// ---- Get the wiki page ----
	t.Run("get wiki page by slug", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/mywiki/wiki/home", token, nil)
		requireStatus(t, resp, http.StatusOK, "get wiki page")
		if title, _ := out["title"].(string); title != "Home" {
			t.Fatalf("title=%q want 'Home'", title)
		}
		if content, _ := out["content"].(string); content != "Welcome to my wiki" {
			t.Fatalf("content=%q want 'Welcome to my wiki'", content)
		}
	})

	// ---- Update the wiki page ----
	t.Run("update wiki page", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+owner+"/mywiki/wiki/home", token, map[string]any{
			"title":   "Home Updated",
			"content": "Updated welcome content",
		})
		requireStatus(t, resp, http.StatusOK, "update wiki page")
		if title, _ := out["title"].(string); title != "Home Updated" {
			t.Fatalf("title=%q want 'Home Updated'", title)
		}
		if content, _ := out["content"].(string); content != "Updated welcome content" {
			t.Fatalf("content=%q want 'Updated welcome content'", content)
		}
	})

	// ---- List wiki pages (should have 1 page) ----
	t.Run("list wiki pages after create", func(t *testing.T) {
		resp, pages := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/mywiki/wiki", token, nil)
		requireStatus(t, resp, http.StatusOK, "list wiki pages")
		if len(pages) != 1 {
			t.Fatalf("expected 1 page, got %d", len(pages))
		}
		if slug, _ := pages[0]["slug"].(string); slug != "home" {
			t.Fatalf("slug=%q want 'home'", slug)
		}
	})

	// ---- Create a second wiki page ----
	t.Run("create second wiki page", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+owner+"/mywiki/wiki/guide", token, map[string]any{
			"title":   "User Guide",
			"content": "How to use this project",
		})
		requireStatus(t, resp, http.StatusOK, "create second wiki page")
		if slug, _ := out["slug"].(string); slug != "guide" {
			t.Fatalf("slug=%q want 'guide'", slug)
		}
	})

	// ---- List wiki pages (should have 2 pages) ----
	t.Run("list wiki pages with 2 pages", func(t *testing.T) {
		resp, pages := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/mywiki/wiki", token, nil)
		requireStatus(t, resp, http.StatusOK, "list wiki pages")
		if len(pages) != 2 {
			t.Fatalf("expected 2 pages, got %d", len(pages))
		}
		slugs := make(map[string]bool)
		for _, p := range pages {
			s, _ := p["slug"].(string)
			slugs[s] = true
		}
		if !slugs["home"] {
			t.Error("missing 'home' slug")
		}
		if !slugs["guide"] {
			t.Error("missing 'guide' slug")
		}
	})

	// ---- Delete a wiki page ----
	t.Run("delete wiki page", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodDelete, base+"/api/v1/repos/"+owner+"/mywiki/wiki/home", token, nil)
		requireStatus(t, resp, http.StatusOK, "delete wiki page")
		if status, _ := out["status"].(string); status != "deleted" {
			t.Fatalf("status=%q want 'deleted'", status)
		}
	})

	// ---- Verify deleted page returns 404 ----
	t.Run("get deleted page returns 404", func(t *testing.T) {
		resp, _ := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/mywiki/wiki/home", token, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for deleted page, got %d", resp.StatusCode)
		}
	})

	// ---- Verify remaining page still exists ----
	t.Run("remaining page still exists after delete", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/mywiki/wiki/guide", token, nil)
		requireStatus(t, resp, http.StatusOK, "get remaining wiki page")
		if title, _ := out["title"].(string); title != "User Guide" {
			t.Fatalf("title=%q want 'User Guide'", title)
		}
	})

	// ---- List wiki pages after delete (should have 1 page) ----
	t.Run("list after delete has 1 page", func(t *testing.T) {
		resp, pages := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/mywiki/wiki", token, nil)
		requireStatus(t, resp, http.StatusOK, "list wiki pages after delete")
		if len(pages) != 1 {
			t.Fatalf("expected 1 page after delete, got %d", len(pages))
		}
		if slug, _ := pages[0]["slug"].(string); slug != "guide" {
			t.Fatalf("slug=%q want 'guide'", slug)
		}
	})
}

func TestWikiFlow_NonexistentPage(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "wnp" + short

	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "checkwiki")

	// ---- Get nonexistent page returns 404 ----
	t.Run("get nonexistent page returns 404", func(t *testing.T) {
		resp, _ := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/checkwiki/wiki/nonexistent", token, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for nonexistent page, got %d", resp.StatusCode)
		}
	})

	// ---- Delete nonexistent page returns 404 ----
	t.Run("delete nonexistent page returns 404", func(t *testing.T) {
		resp, _ := testutil.DoJSON(t, http.MethodDelete, base+"/api/v1/repos/"+owner+"/checkwiki/wiki/nonexistent", token, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for delete nonexistent page, got %d", resp.StatusCode)
		}
	})

	// ---- List wiki on nonexistent repo returns 404 ----
	t.Run("list wiki on nonexistent repo returns 404", func(t *testing.T) {
		resp, _ := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/norepo/wiki", token, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for nonexistent repo, got %d", resp.StatusCode)
		}
	})
}

func TestWikiFlow_AccessControl(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "wac" + short
	other := "wco" + short

	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	otherToken := testutil.RegisterAndLogin(t, base, other)

	// Create a private repo
	t.Run("create private repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/users/"+owner+"/repos", ownerToken, map[string]any{
			"name": "privwiki", "private": true,
		})
		requireStatus(t, resp, http.StatusOK, "create private repo")
		if out["name"] != "privwiki" {
			t.Fatalf("expected repo name 'privwiki', got %q", out["name"])
		}
	})

	// Owner creates wiki page in private repo
	t.Run("owner creates wiki page in private repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+owner+"/privwiki/wiki/home", ownerToken, map[string]any{
			"title": "Home", "content": "Private content",
		})
		requireStatus(t, resp, http.StatusOK, "owner create wiki page")
		if slug, _ := out["slug"].(string); slug != "home" {
			t.Fatalf("slug=%q want 'home'", slug)
		}
	})

	// Other user cannot read wiki on private repo
	t.Run("other user cannot read wiki on private repo", func(t *testing.T) {
		resp, _ := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/privwiki/wiki", otherToken, nil)
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 404/403 for other user, got %d", resp.StatusCode)
		}
	})

	// Other user cannot create wiki page on private repo
	t.Run("other user cannot create wiki page on private repo", func(t *testing.T) {
		resp, _ := testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+owner+"/privwiki/wiki/evil", otherToken, map[string]any{
			"title": "Evil", "content": "hacked",
		})
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 404/403/401 for other user creating wiki page, got %d", resp.StatusCode)
		}
	})

	// Owner can still access their own wiki
	t.Run("owner can still access own wiki", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/privwiki/wiki/home", ownerToken, nil)
		requireStatus(t, resp, http.StatusOK, "owner get own wiki page")
		if content, _ := out["content"].(string); content != "Private content" {
			t.Fatalf("content=%q want 'Private content'", content)
		}
	})

	// Owner can create additional pages
	t.Run("owner can create additional pages in private repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+owner+"/privwiki/wiki/guide", ownerToken, map[string]any{
			"title": "Guide", "content": "User guide",
		})
		requireStatus(t, resp, http.StatusOK, "owner create second wiki page")
		if slug, _ := out["slug"].(string); slug != "guide" {
			t.Fatalf("slug=%q want 'guide'", slug)
		}
	})
}

func TestWikiFlow_MultipleUsers(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "wmu" + short
	contributor := "wmc" + short

	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	contribToken := testutil.RegisterAndLogin(t, base, contributor)

	// Create a public repo
	createRepo(t, base, ownerToken, owner, "sharedwiki")

	// Owner creates a wiki page
	t.Run("owner creates wiki page in public repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+owner+"/sharedwiki/wiki/home", ownerToken, map[string]any{
			"title": "Home", "content": "Welcome",
		})
		requireStatus(t, resp, http.StatusOK, "owner create wiki page")
		if slug, _ := out["slug"].(string); slug != "home" {
			t.Fatalf("slug=%q want 'home'", slug)
		}
	})

	// Contributor (other user) can read wiki on public repo
	t.Run("contributor can read wiki on public repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/sharedwiki/wiki/home", contribToken, nil)
		requireStatus(t, resp, http.StatusOK, "contributor get wiki page")
		if content, _ := out["content"].(string); content != "Welcome" {
			t.Fatalf("content=%q want 'Welcome'", content)
		}
	})

	// Contributor can read wiki list on public repo
	t.Run("contributor can list wiki on public repo", func(t *testing.T) {
		resp, pages := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/sharedwiki/wiki", contribToken, nil)
		requireStatus(t, resp, http.StatusOK, "contributor list wiki")
		if len(pages) < 1 {
			t.Fatalf("expected at least 1 page, got %d", len(pages))
		}
	})
}
