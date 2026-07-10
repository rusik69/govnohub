//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestWorkflowTriggerAndLogs(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "act" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	wf := `name: CI
on: push
jobs:
  test:
    runs-on: linux
    steps:
      - run: echo ok`
	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/actions/workflows", token, map[string]string{
		"name": "CI", "path": ".github/workflows/ci.yml", "content": wf,
	})
	requireStatus(t, resp, http.StatusOK, "upsert workflow")
	workflowID, _ := out["id"].(string)
	if workflowID == "" {
		t.Fatal("missing workflow id")
	}

	resp, workflows := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/actions/workflows", token, nil)
	requireStatus(t, resp, http.StatusOK, "list workflows")
	if len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(workflows))
	}

	resp, patOut := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/user/tokens", token, map[string]any{
		"name": "ci", "scopes": []string{"repo", "repo:write", "workflow"},
	})
	requireStatus(t, resp, http.StatusOK, "create PAT")
	pat, _ := patOut["token"].(string)

	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/actions/runs", pat, map[string]any{
		"workflow_id": workflowID, "event": "push", "branch": "main",
	})
	requireStatus(t, resp, http.StatusOK, "trigger run")
	runID, _ := out["id"].(string)
	if runID == "" {
		t.Fatal("missing run id")
	}

	resp, runs := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/actions/runs", token, nil)
	requireStatus(t, resp, http.StatusOK, "list runs")
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/actions/runs/"+runID+"/logs", token, nil)
	requireStatus(t, resp, http.StatusOK, "run logs")
}
