//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestAIReviewConfigDisabled(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "ai" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/ai-review/config", token, nil)
	requireStatus(t, resp, http.StatusOK, "ai review config")
	if enabled, _ := out["enabled"].(bool); enabled {
		t.Fatal("expected ai review disabled in testenv")
	}
}

func TestCreateAIReviewUnavailable(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "ai2" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/branches", token, map[string]string{
		"name": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create branch")

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls", token, map[string]string{
		"title": "ai review", "body": "", "head": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create PR")
	prNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum)+"/ai-reviews", token, nil)
	requireStatus(t, resp, http.StatusServiceUnavailable, "create ai review disabled")
}
