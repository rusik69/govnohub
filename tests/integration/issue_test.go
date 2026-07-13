//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestIssueCRUD(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "issueuser")
	owner := "issueuser"

	// Create a repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "testrepo", "description": "repo for issue tests", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Create an issue
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/testrepo/issues", token, map[string]string{
		"title": "found a bug", "body": "This is a bug description",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create issue status=%d out=%v", resp.StatusCode, out)
	}
	title, _ := out["title"].(string)
	if title != "found a bug" {
		t.Fatalf("issue title=%q want %q", title, "found a bug")
	}
	body, _ := out["body"].(string)
	if body != "This is a bug description" {
		t.Fatalf("issue body=%q", body)
	}
	state, _ := out["state"].(string)
	if state != "open" {
		t.Fatalf("issue state=%q want open", state)
	}
	number, _ := out["number"].(float64)
	if number != 1 {
		t.Fatalf("issue number=%v want 1", number)
	}
	author, _ := out["author"].(string)
	if author != owner {
		t.Fatalf("issue author=%q want %q", author, owner)
	}

	// Get issue by number
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/testrepo/issues/1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get issue status=%d out=%v", resp.StatusCode, out)
	}
	if out["title"] != "found a bug" {
		t.Fatalf("got title=%v", out["title"])
	}

	// Get non-existent issue returns 404
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/testrepo/issues/999", token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent issue, got %d", resp.StatusCode)
	}

	// List issues
	resp, err := http.Get(env.URL + "/api/v1/repos/" + owner + "/testrepo/issues")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list issues status=%d", resp.StatusCode)
	}
	var issues []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0]["number"] != float64(1) {
		t.Fatalf("issue number=%v", issues[0]["number"])
	}

	// Close the issue
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/testrepo/issues/1/close", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("close issue status=%d out=%v", resp.StatusCode, out)
	}
	if out["status"] != "closed" {
		t.Fatalf("close status=%q want closed", out["status"])
	}

	// Verify closed state
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/testrepo/issues/1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get closed issue status=%d", resp.StatusCode)
	}
	if out["state"] != "closed" {
		t.Fatalf("state=%q want closed", out["state"])
	}

	// Close already closed issue should still work
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/testrepo/issues/1/close", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("close already closed status=%d", resp.StatusCode)
	}
}

func TestIssueComments(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "commentuser")
	owner := "commentuser"

	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "repo", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	// Create an issue
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/repo/issues", token, map[string]string{
		"title": "needs discussion",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create issue: %d", resp.StatusCode)
	}

	// Add a comment
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/repo/issues/1/comments", token, map[string]string{
		"body": "I agree, this needs fixing",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add comment status=%d out=%v", resp.StatusCode, out)
	}
	body, _ := out["body"].(string)
	if body != "I agree, this needs fixing" {
		t.Fatalf("comment body=%q", body)
	}
	if _, ok := out["id"]; !ok {
		t.Fatal("missing comment id")
	}

	// Add another comment
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/repo/issues/1/comments", token, map[string]string{
		"body": "Second comment here",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add second comment: %d", resp.StatusCode)
	}

	// List comments
	resp, _ = http.Get(env.URL + "/api/v1/repos/" + owner + "/repo/issues/1/comments")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list comments status=%d", resp.StatusCode)
	}
	defer resp.Body.Close()
	var comments []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		t.Fatal(err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	if comments[0]["body"] != "I agree, this needs fixing" {
		t.Fatalf("first comment body=%q", comments[0]["body"])
	}
	if comments[1]["body"] != "Second comment here" {
		t.Fatalf("second comment body=%q", comments[1]["body"])
	}
}

func TestIssueLabels(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "labeluser")
	owner := "labeluser"

	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "labels-repo", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	// Create a label
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/labels-repo/labels", token, map[string]string{
		"name": "bug", "color": "ff0000",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create label status=%d out=%v", resp.StatusCode, out)
	}
	labelID, ok := out["id"].(string)
	if !ok || labelID == "" {
		t.Fatal("missing label id")
	}
	if out["name"] != "bug" {
		t.Fatalf("label name=%q", out["name"])
	}
	if out["color"] != "ff0000" {
		t.Fatalf("label color=%q", out["color"])
	}

	// Create another label
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/labels-repo/labels", token, map[string]string{
		"name": "enhancement", "color": "00ff00",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create label 2: %d", resp.StatusCode)
	}
	labelID2, _ := out["id"].(string)

	// List labels
	resp, _ = http.Get(env.URL + "/api/v1/repos/" + owner + "/labels-repo/labels")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list labels status=%d", resp.StatusCode)
	}
	defer resp.Body.Close()
	var labels []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&labels); err != nil {
		t.Fatal(err)
	}
	if len(labels) < 2 {
		t.Fatalf("expected at least 2 labels, got %d", len(labels))
	}

	// Create an issue
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/labels-repo/issues", token, map[string]string{
		"title": "label test issue",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create issue: %d", resp.StatusCode)
	}

	// Add label to issue
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/labels-repo/issues/1/labels/"+labelID, token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add label to issue status=%d out=%v", resp.StatusCode, out)
	}
	labelsOnIssue, _ := out["labels"].([]any)
	if len(labelsOnIssue) != 1 {
		t.Fatalf("expected 1 label on issue, got %d", len(labelsOnIssue))
	}

	// Add same label again (idempotent)
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/labels-repo/issues/1/labels/"+labelID, token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add label duplicate status=%d", resp.StatusCode)
	}
	labelsOnIssue, _ = out["labels"].([]any)
	if len(labelsOnIssue) != 1 {
		t.Fatalf("expected still 1 label after duplicate, got %d", len(labelsOnIssue))
	}

	// Add second label
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/labels-repo/issues/1/labels/"+labelID2, token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add second label status=%d", resp.StatusCode)
	}
	labelsOnIssue, _ = out["labels"].([]any)
	if len(labelsOnIssue) != 2 {
		t.Fatalf("expected 2 labels on issue, got %d", len(labelsOnIssue))
	}

	// Remove label from issue
	resp, out = testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/repos/"+owner+"/labels-repo/issues/1/labels/"+labelID, token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove label status=%d out=%v", resp.StatusCode, out)
	}
	labelsOnIssue, _ = out["labels"].([]any)
	if len(labelsOnIssue) != 1 {
		t.Fatalf("expected 1 label after removal, got %d", len(labelsOnIssue))
	}

	// Get issue should reflect labels
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/labels-repo/issues/1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get issue after label ops: %d", resp.StatusCode)
	}
	labelsOnIssue, _ = out["labels"].([]any)
	if len(labelsOnIssue) != 1 {
		t.Fatalf("expected 1 label on get, got %d", len(labelsOnIssue))
	}
}

func TestIssueMilestones(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "milestoneuser")
	owner := "milestoneuser"

	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "ms-repo", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	// Create a milestone
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/ms-repo/milestones", token, map[string]string{
		"title": "v1.0", "description": "First release",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create milestone status=%d out=%v", resp.StatusCode, out)
	}
	msID, ok := out["id"].(string)
	if !ok || msID == "" {
		t.Fatal("missing milestone id")
	}
	if out["title"] != "v1.0" {
		t.Fatalf("milestone title=%q", out["title"])
	}
	if out["state"] != "open" {
		t.Fatalf("milestone state=%q", out["state"])
	}

	// Create another milestone
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/ms-repo/milestones", token, map[string]string{
		"title": "v2.0",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create milestone 2: %d", resp.StatusCode)
	}

	// List milestones
	resp, _ = http.Get(env.URL + "/api/v1/repos/" + owner + "/ms-repo/milestones")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list milestones status=%d", resp.StatusCode)
	}
	defer resp.Body.Close()
	var milestones []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&milestones); err != nil {
		t.Fatal(err)
	}
	if len(milestones) < 2 {
		t.Fatalf("expected at least 2 milestones, got %d", len(milestones))
	}

	// Close a milestone
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/ms-repo/milestones/"+msID+"/close", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("close milestone status=%d out=%v", resp.StatusCode, out)
	}
	if out["status"] != "closed" {
		t.Fatalf("close status=%q", out["status"])
	}

	// Verify milestone is closed
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/ms-repo/milestones", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list milestones after close: %d", resp.StatusCode)
	}
	// The response is an array - need to handle differently
	resp2, _ := http.Get(env.URL + "/api/v1/repos/" + owner + "/ms-repo/milestones")
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("list milestones get: %d", resp2.StatusCode)
	}
	defer resp2.Body.Close()
	var msAfterClose []map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&msAfterClose); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ms := range msAfterClose {
		if ms["title"] == "v1.0" {
			if ms["state"] != "closed" {
				t.Fatalf("milestone state=%q want closed", ms["state"])
			}
			found = true
		}
	}
	if !found {
		t.Fatal("v1.0 milestone not found in list after close")
	}
}

func TestIssuePatch(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "patchuser")
	owner := "patchuser"

	// Create repo and issue
	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "patch-repo", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/patch-repo/issues", token, map[string]string{
		"title": "patchable issue",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create issue: %d", resp.StatusCode)
	}

	// Create a milestone
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/patch-repo/milestones", token, map[string]string{
		"title": "sprint-1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create milestone: %d", resp.StatusCode)
	}
	msID, _ := out["id"].(string)

	// Patch issue: set assignee and milestone
	resp, out = testutil.DoJSON(t, http.MethodPatch, env.URL+"/api/v1/repos/"+owner+"/patch-repo/issues/1", token, map[string]any{
		"assignee":     owner,
		"milestone_id": msID,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch issue status=%d out=%v", resp.StatusCode, out)
	}
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/patch-repo/issues/1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get patched issue: %d", resp.StatusCode)
	}
	if out["assignee"] != owner {
		t.Fatalf("assignee=%q want %q", out["assignee"], owner)
	}
	if out["milestone"] != "sprint-1" {
		t.Fatalf("milestone=%q want sprint-1", out["milestone"])
	}

	// Clear assignee and milestone via patch
	resp, out = testutil.DoJSON(t, http.MethodPatch, env.URL+"/api/v1/repos/"+owner+"/patch-repo/issues/1", token, map[string]any{
		"assignee":       "",
		"clear_milestone": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear milestone status=%d out=%v", resp.StatusCode, out)
	}
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/patch-repo/issues/1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get after clear: %d", resp.StatusCode)
	}
	assignee, _ := out["assignee"].(string)
	if assignee != "" {
		t.Fatalf("expected empty assignee, got %q", assignee)
	}
	milestone, _ := out["milestone"].(string)
	if milestone != "" {
		t.Fatalf("expected empty milestone, got %q", milestone)
	}
}
