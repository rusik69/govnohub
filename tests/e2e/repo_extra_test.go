//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestWikiCRUD(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "wiki" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+repo+"/wiki/home", token, map[string]string{
		"title": "Home", "content": "# Welcome",
	})
	requireStatus(t, resp, http.StatusOK, "upsert wiki page")
	if out["slug"] != "home" {
		t.Fatalf("unexpected slug: %v", out["slug"])
	}

	resp, out = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/wiki/home", token, nil)
	requireStatus(t, resp, http.StatusOK, "get wiki page")
	if out["content"] != "# Welcome" {
		t.Fatalf("unexpected content: %v", out["content"])
	}

	resp, pages := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/wiki", token, nil)
	requireStatus(t, resp, http.StatusOK, "list wiki pages")
	if len(pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(pages))
	}

	resp, _ = testutil.DoJSON(t, http.MethodDelete, base+"/api/v1/repos/"+repo+"/wiki/home", token, nil)
	requireStatus(t, resp, http.StatusOK, "delete wiki page")

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/wiki/home", token, nil)
	requireStatus(t, resp, http.StatusNotFound, "deleted wiki page")
}

func TestWebhooksAndDeliveries(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "hook" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/webhooks", token, map[string]any{
		"url": "http://127.0.0.1:1/hook", "secret": "s", "events": []string{"push"},
	})
	requireStatus(t, resp, http.StatusOK, "create webhook")
	hookID, _ := out["id"].(string)
	if hookID == "" {
		t.Fatal("missing webhook id")
	}

	resp, hooks := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/webhooks", token, nil)
	requireStatus(t, resp, http.StatusOK, "list webhooks")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(hooks))
	}

	resp, _ = doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/webhooks/"+hookID+"/deliveries", token, nil)
	requireStatus(t, resp, http.StatusOK, "list deliveries")
}

func TestRepoMetadata(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "meta" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo, token, nil)
	requireStatus(t, resp, http.StatusOK, "get repo")

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/contents/README.md", token, nil)
	requireStatus(t, resp, http.StatusOK, "get contents")

	resp, commits := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/commits", token, nil)
	requireStatus(t, resp, http.StatusOK, "list commits")
	if len(commits) == 0 {
		t.Fatal("expected at least one commit")
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/star", token, nil)
	requireStatus(t, resp, http.StatusOK, "star repo")

	resp, _ = testutil.DoJSON(t, http.MethodDelete, base+"/api/v1/repos/"+repo+"/star", token, nil)
	requireStatus(t, resp, http.StatusOK, "unstar repo")

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/watch", token, nil)
	requireStatus(t, resp, http.StatusOK, "watch repo")

	resp, _ = testutil.DoJSON(t, http.MethodDelete, base+"/api/v1/repos/"+repo+"/watch", token, nil)
	requireStatus(t, resp, http.StatusOK, "unwatch repo")

	resp, branches := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/branches", token, nil)
	requireStatus(t, resp, http.StatusOK, "list branches")
	if len(branches) == 0 {
		t.Fatal("expected branches")
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/fork", token, nil)
	requireStatus(t, resp, http.StatusOK, "fork repo")

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/search?q=app", token, nil)
	requireStatus(t, resp, http.StatusOK, "search")
}
