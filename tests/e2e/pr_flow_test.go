//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestPRReviewAndMerge(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "pr" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/protected-branches", token, map[string]any{
		"branch": "main", "required_checks": []string{}, "require_reviews": 1,
	})
	requireStatus(t, resp, http.StatusOK, "protect branch")

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/branches", token, map[string]string{
		"name": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create branch")

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls", token, map[string]string{
		"title": "feature", "body": "", "head": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create PR")
	prNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum)+"/merge", token, map[string]any{"squash": false})
	requireStatus(t, resp, http.StatusForbidden, "merge without review blocked")

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum)+"/reviews", token, map[string]string{
		"state": "approved", "body": "lgtm",
	})
	requireStatus(t, resp, http.StatusOK, "approve PR")

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum)+"/merge", token, map[string]any{"squash": false})
	requireStatus(t, resp, http.StatusOK, "merge with review")
}

func TestPRCommentsAndDiff(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "prc" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/branches", token, map[string]string{
		"name": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create branch")

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls", token, map[string]string{
		"title": "feature", "body": "", "head": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create PR")
	prNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum)+"/comments", token, map[string]string{
		"body": "nice change",
	})
	requireStatus(t, resp, http.StatusOK, "add PR comment")

	resp, comments := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum)+"/comments", token, nil)
	requireStatus(t, resp, http.StatusOK, "list PR comments")
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum)+"/diff", token, nil)
	requireStatus(t, resp, http.StatusOK, "PR diff")
}
