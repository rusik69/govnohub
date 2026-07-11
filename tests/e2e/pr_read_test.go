//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestListPulls(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "plist" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/branches", token, map[string]string{
		"name": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create branch")

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls", token, map[string]string{
		"title": "feature PR", "body": "", "head": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create PR")

	resp, pulls := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/pulls", token, nil)
	requireStatus(t, resp, http.StatusOK, "list pulls")
	if len(pulls) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(pulls))
	}
}

func TestGetPR(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "pget" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/branches", token, map[string]string{
		"name": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create branch")

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls", token, map[string]string{
		"title": "my PR", "body": "desc", "head": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create PR")
	prNum := int(out["number"].(float64))

	resp, got := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum), token, nil)
	requireStatus(t, resp, http.StatusOK, "get PR")
	if got["title"] != "my PR" {
		t.Fatalf("unexpected title: %v", got["title"])
	}
}

func TestListPRReviews(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "prev" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/branches", token, map[string]string{
		"name": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create branch")

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls", token, map[string]string{
		"title": "review me", "body": "", "head": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create PR")
	prNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum)+"/reviews", token, map[string]string{
		"state": "approved", "body": "lgtm",
	})
	requireStatus(t, resp, http.StatusOK, "add review")

	resp, reviews := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum)+"/reviews", token, nil)
	requireStatus(t, resp, http.StatusOK, "list reviews")
	assertJSONArrayLen(t, reviews, 1, "PR reviews")
}

func TestSquashMerge(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "psq" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/branches", token, map[string]string{
		"name": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create branch")
	gitCommitOnBranch(t, env, owner, "app", "feature", "feature.txt", "feature change\n", "feature commit")

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls", token, map[string]string{
		"title": "squash me", "body": "", "head": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create PR")
	prNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum)+"/merge", token, map[string]any{
		"squash": true,
	})
	requireStatus(t, resp, http.StatusOK, "squash merge")

	resp, pulls := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/pulls", token, nil)
	requireStatus(t, resp, http.StatusOK, "list pulls")
	if state, _ := pulls[0]["state"].(string); state != "closed" {
		t.Fatalf("expected closed state after merge, got %v", state)
	}
	if sha, _ := pulls[0]["merge_sha"].(string); sha == "" {
		t.Fatal("expected merge_sha after squash merge")
	}
}
