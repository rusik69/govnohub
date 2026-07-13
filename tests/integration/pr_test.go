//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rusik69/govnohub/internal/git"
	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestPRFullLifecycle(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "pruser")
	owner := "pruser"

	// Create a repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "pr-test", "description": "repo for PR tests", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}

	// Create a feature branch
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/pr-test/branches", token, map[string]string{
		"name": "feature-branch", "base": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create branch status=%d out=%v", resp.StatusCode, out)
	}

	// Make a commit on the feature branch so merge has something to do
	repoPath := filepath.Join(env.GitRoot, owner, "pr-test.git")
	wt, err := os.MkdirTemp("", "govnohub-test-commit-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(wt)

	cmd := exec.Command("git", "-C", repoPath, "worktree", "add", "--detach", wt, "feature-branch")
	if outB, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v: %s", err, outB)
	}
	defer exec.Command("git", "-C", repoPath, "worktree", "remove", "--force", wt).Run()

	if err := os.WriteFile(filepath.Join(wt, "new-file.txt"), []byte("hello from feature branch"), 0644); err != nil {
		t.Fatal(err)
	}
	gitEnv := append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@local",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@local",
	)

	cmd = exec.Command("git", "-C", wt, "add", ".")
	cmd.Env = gitEnv
	if outB, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, outB)
	}
	cmd = exec.Command("git", "-C", wt, "commit", "-m", "feat: add new-file.txt")
	cmd.Env = gitEnv
	if outB, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, outB)
	}
	cmd = exec.Command("git", "-C", wt, "push", repoPath, "HEAD:refs/heads/feature-branch")
	cmd.Env = gitEnv
	if outB, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git push: %v: %s", err, outB)
	}

	// Create a PR
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls", token, map[string]string{
		"title": "feat: add new feature",
		"body":  "This PR adds a great new feature",
		"head":  "feature-branch",
		"base":  "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create PR status=%d out=%v", resp.StatusCode, out)
	}
	title, _ := out["title"].(string)
	if title != "feat: add new feature" {
		t.Fatalf("PR title=%q want %q", title, "feat: add new feature")
	}
	body, _ := out["body"].(string)
	if body != "This PR adds a great new feature" {
		t.Fatalf("PR body=%q", body)
	}
	state, _ := out["state"].(string)
	if state != "open" {
		t.Fatalf("PR state=%q want open", state)
	}
	number, _ := out["number"].(float64)
	if number != 1 {
		t.Fatalf("PR number=%v want 1", number)
	}
	headBranch, _ := out["head_branch"].(string)
	if headBranch != "feature-branch" {
		t.Fatalf("PR head_branch=%q", headBranch)
	}
	baseBranch, _ := out["base_branch"].(string)
	if baseBranch != "main" {
		t.Fatalf("PR base_branch=%q", baseBranch)
	}

	// Get PR by number
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls/1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get PR status=%d out=%v", resp.StatusCode, out)
	}
	if out["title"] != "feat: add new feature" {
		t.Fatalf("got title=%v", out["title"])
	}
	mergeable, _ := out["mergeable"].(bool)
	if !mergeable {
		t.Fatal("expected mergeable=true for feature branch with commits against main")
	}

	// Get non-existent PR returns 404
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls/999", token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent PR, got %d", resp.StatusCode)
	}

	// List PRs
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list PRs status=%d", resp.StatusCode)
	}
	var prs []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
	if prs[0]["number"] != float64(1) {
		t.Fatalf("PR number=%v", prs[0]["number"])
	}

	// Add a review
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls/1/reviews", token, map[string]string{
		"state": "approved",
		"body":  "Looks good, ship it!",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add review status=%d out=%v", resp.StatusCode, out)
	}
	if out["state"] != "approved" {
		t.Fatalf("review state=%q want approved", out["state"])
	}
	if out["body"] != "Looks good, ship it!" {
		t.Fatalf("review body=%q", out["body"])
	}

	// List reviews
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls/1/reviews", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list reviews status=%d", resp.StatusCode)
	}
	var reviews []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0]["state"] != "approved" {
		t.Fatalf("review state=%q", reviews[0]["state"])
	}

	// Add a comment
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls/1/comments", token, map[string]string{
		"body": "Great work on this PR!",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add comment status=%d out=%v", resp.StatusCode, out)
	}
	if out["body"] != "Great work on this PR!" {
		t.Fatalf("comment body=%q", out["body"])
	}
	if _, ok := out["id"]; !ok {
		t.Fatal("missing comment id")
	}

	// List comments
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls/1/comments", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list comments status=%d", resp.StatusCode)
	}
	var comments []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0]["body"] != "Great work on this PR!" {
		t.Fatalf("comment body=%q", comments[0]["body"])
	}

	// Get PR commits — should include the commit we made on feature-branch
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls/1/commits", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list PR commits status=%d", resp.StatusCode)
	}
	var commits []gitstore.CommitInfo
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		t.Fatal(err)
	}
	if len(commits) == 0 {
		t.Fatal("expected at least 1 commit in PR")
	}
	found := false
	for _, c := range commits {
		if c.Message == "feat: add new-file.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find commit with message %q among %d commits", "feat: add new-file.txt", len(commits))
	}
	if commits[0].SHA == "" {
		t.Error("commit SHA should not be empty")
	}
	if commits[0].Author == "" {
		t.Error("commit author should not be empty")
	}

	// Get PR files — should include the files changed
	req, _ = http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls/1/files", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list PR files status=%d", resp.StatusCode)
	}
	var files []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least 1 changed file in PR")
	}
	foundNewFile := false
	if len(files) > 0 {
		for _, f := range files {
			if fn, _ := f["filename"].(string); fn == "new-file.txt" {
				foundNewFile = true
				break
			}
		}
	}
	if !foundNewFile {
		t.Fatalf("expected to find new-file.txt among changed files, got %v", files)
	}

	// Merge the PR
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls/1/merge", token, map[string]any{
		"squash": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("merge PR status=%d out=%v", resp.StatusCode, out)
	}
	mergeSHA, _ := out["merge_sha"].(string)
	if mergeSHA == "" {
		t.Fatal("expected merge_sha after merge")
	}

	// Verify PR is closed after merge
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/pr-test/pulls/1", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get merged PR status=%d", resp.StatusCode)
	}
	if out["state"] != "closed" {
		t.Fatalf("PR state=%q want closed after merge", out["state"])
	}
	if out["merge_sha"] != mergeSHA {
		t.Fatalf("merge_sha=%q want %q", out["merge_sha"], mergeSHA)
	}
}

func TestPRCreateWithEmptyBody(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "prempty")
	owner := "prempty"

	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "pr-emptytest", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	// Create a branch
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/pr-emptytest/branches", token, map[string]string{
		"name": "feature", "base": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create branch: %d", resp.StatusCode)
	}

	// Create PR with empty body (no body field)
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/pr-emptytest/pulls", token, map[string]string{
		"title": "empty body PR",
		"head":  "feature",
		"base":  "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create PR status=%d out=%v", resp.StatusCode, out)
	}
	if out["title"] != "empty body PR" {
		t.Fatalf("title=%q", out["title"])
	}
}

func TestPRListEmpty(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "prlistempty")
	owner := "prlistempty"

	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "no-prs", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/no-prs/pulls", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list PRs status=%d", resp.StatusCode)
	}
	var prs []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		t.Fatal(err)
	}
	if len(prs) != 0 {
		t.Fatalf("expected 0 PRs, got %d", len(prs))
	}
}

func TestPRAccessControl(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	owner := "prprivateowner"
	other := "prprivateother"
	ownerToken := testutil.RegisterAndLogin(t, env.URL, owner)
	otherToken := testutil.RegisterAndLogin(t, env.URL, other)

	// Create private repo
	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", ownerToken, map[string]any{
		"name": "private", "private": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	// Create branch
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/private/branches", ownerToken, map[string]string{
		"name": "feature", "base": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create branch: %d", resp.StatusCode)
	}

	// Create PR as owner
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/private/pulls", ownerToken, map[string]string{
		"title": "private PR", "head": "feature", "base": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create PR as owner: %d", resp.StatusCode)
	}

	// Other user should be forbidden from accessing PRs in private repo
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/private/pulls/1", otherToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("other user get PR should be 403, got %d", resp.StatusCode)
	}

	// Other user should get 403 on listing PRs too
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/private/pulls", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("other user list PRs should be 403, got %d", resp.StatusCode)
	}

	// Owner can still access
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/private/pulls/1", ownerToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner get PR should be 200, got %d", resp.StatusCode)
	}
}
