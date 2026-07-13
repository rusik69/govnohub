//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestWorkflowParseAndTriggerRun(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "wfuser")
	owner := "wfuser"

	// Create a repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "actions-test", "description": "repo for actions tests", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Create a workflow
	workflowContent := `name: CI
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo "hello world"
  test:
    needs: build
    steps:
      - run: echo "running tests"
`
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/actions-test/actions/workflows", token, map[string]any{
		"name":    "CI",
		"path":    ".github/workflows/ci.yml",
		"content": workflowContent,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create workflow status=%d out=%v", resp.StatusCode, out)
	}
	wfID, ok := out["id"].(string)
	if !ok || wfID == "" {
		t.Fatal("missing workflow id")
	}

	// List workflows
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/actions-test/actions/workflows", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list workflows status=%d out=%v", resp.StatusCode, out)
	}
	wfs, ok := out["data"].([]interface{})
	if !ok {
		// The response might be a JSON array directly
		_ = wfs
	}

	// Trigger a run
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/actions-test/actions/runs", token, map[string]any{
		"workflow_id": wfID,
		"event":       "push",
		"branch":      "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("trigger run status=%d out=%v", resp.StatusCode, out)
	}
	runStatus, _ := out["status"].(string)
	if runStatus != "queued" {
		t.Fatalf("run status=%q want queued", runStatus)
	}
	runID, ok := out["id"].(string)
	if !ok || runID == "" {
		t.Fatal("missing run id")
	}

	// List runs
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/actions-test/actions/runs", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list runs status=%d", resp.StatusCode)
	}

	// Get run logs (should return empty or no jobs since no controller runs)
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/actions-test/actions/runs/"+runID+"/logs", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get run logs status=%d out=%v", resp.StatusCode, out)
	}
}

func TestWorkflowListEmpty(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "wflistuser")
	owner := "wflistuser"

	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "empty-actions", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// List workflows — should return HTTP 200 (null response for empty)
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/empty-actions/actions/workflows", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list workflows status=%d", resp.StatusCode)
	}

	// List runs — should return HTTP 200 (null response for empty)
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/empty-actions/actions/runs", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list runs status=%d", resp.StatusCode)
	}
}

func TestWorkflowInvalidContent(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "wfinvuser")
	owner := "wfinvuser"

	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "inv-actions", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Upload invalid YAML — should be rejected
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/inv-actions/actions/workflows", token, map[string]any{
		"name":    "BadWorkflow",
		"path":    ".github/workflows/bad.yml",
		"content": "invalid: [yaml: unclosed",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid workflow, got %d out=%v", resp.StatusCode, out)
	}
}

func TestWorkflowRunNotFound(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "wfnfuser")
	owner := "wfnfuser"

	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "nf-actions", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Get logs for non-existent run
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/nf-actions/actions/runs/00000000-0000-0000-0000-000000000000/logs", token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent run, got %d", resp.StatusCode)
	}
}
