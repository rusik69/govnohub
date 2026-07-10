//go:build deploy

package e2e

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestLocalDeployHealth(t *testing.T) {
	env := testenv.NewLocalDeploy(t)
	defer env.Cleanup()

	resp, err := http.Get(env.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("api health: %d", resp.StatusCode)
	}

	resp, err = http.Get(env.GitURL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("git health: %d", resp.StatusCode)
	}
}

func TestLocalDeployFullJourney(t *testing.T) {
	env := testenv.NewLocalDeploy(t)
	defer env.Cleanup()
	runFullJourney(t, env.URL, env.GitURL, true)
}

func TestDeployedClusterFullJourney(t *testing.T) {
	base := deployBaseURL()
	if base == "" {
		t.Skip("set GOVNOHUB_BASE_URL or deploy to govnohub.local")
	}
	if os.Getenv("GOVNOHUB_DEPLOY_E2E") == "" {
		t.Skip("set GOVNOHUB_DEPLOY_E2E=1 to run against a live cluster")
	}
	waitDeployed(t, base)
	gitBase := deployGitURL()
	runFullJourney(t, base, gitBase, false)
}

func runFullJourney(t *testing.T, base, gitBase string, allowRegister bool) {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	owner := "e2e" + suffix[len(suffix)-6:]
	collab := owner + "c"

	var token string
	if allowRegister {
		token = testutil.RegisterAndLogin(t, base, owner)
	} else {
		adminToken := testutil.Login(t, base, "admin", "admin")
		testutil.AdminCreateUser(t, base, adminToken, owner)
		token = testutil.Login(t, base, owner, "password123")
	}

	resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/user", token, nil)
	if resp.StatusCode != http.StatusOK || out["username"] != owner {
		t.Fatalf("me: %d %v", resp.StatusCode, out)
	}

	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/user/tokens", token, map[string]any{
		"name": "ci", "scopes": []string{"repo", "repo:write", "workflow", "read:user"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create PAT: %d %v", resp.StatusCode, out)
	}
	pat, _ := out["token"].(string)

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "app", "description": "e2e app", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get repo: %d", resp.StatusCode)
	}

	if gitBase != "" {
		testGitPush(t, gitBase, owner, "app", owner, "password123")
	}

	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/issues", token, map[string]string{
		"title": "first issue", "body": "details",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("issue: %d", resp.StatusCode)
	}
	issueNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/issues/"+itoa(issueNum)+"/comments", token, map[string]string{
		"body": "looks good",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("issue comment: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/labels", token, map[string]string{
		"name": "bug", "color": "ff0000",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("label: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/branches", token, map[string]string{
		"name": "feature", "base": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create branch: %d", resp.StatusCode)
	}

	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/pulls", token, map[string]string{
		"title": "feature", "body": "", "head": "feature", "base": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pr: %d", resp.StatusCode)
	}
	prNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app/pulls/"+itoa(prNum), token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get pr: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app/pulls/"+itoa(prNum)+"/diff", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pr diff: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app/ai-review/config", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ai review config: %d", resp.StatusCode)
	}

	wf := `name: CI
on: push
jobs:
  test:
    runs-on: linux
    steps:
      - run: echo ok`
	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/actions/workflows", token, map[string]string{
		"name": "CI", "path": ".github/workflows/ci.yml", "content": wf,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("workflow: %d %v", resp.StatusCode, out)
	}
	workflowID, _ := out["id"].(string)

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app/actions/workflows", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list workflows: %d", resp.StatusCode)
	}

	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/actions/runs", pat, map[string]any{
		"workflow_id": workflowID, "event": "push", "branch": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("trigger run: %d %v", resp.StatusCode, out)
	}
	runID, _ := out["id"].(string)

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app/actions/runs", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list runs: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app/actions/runs/"+runID+"/logs", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run logs: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/releases", token, map[string]any{
		"tag_name": "v0.1.0", "name": "v0.1", "body": "first",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("release: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoBody(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/packages?name=demo&version=1.0.0&type=generic", token, "application/octet-stream", strings.NewReader("package-data"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish package: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app/packages", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list packages: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/webhooks", token, map[string]any{
		"url": "http://example.com/hook", "secret": "s", "events": []string{"push"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/star", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("star: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/watch", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("watch: %d", resp.StatusCode)
	}

	if allowRegister {
		testutil.RegisterAndLogin(t, base, collab)
	} else {
		adminToken := testutil.Login(t, base, "admin", "admin")
		testutil.AdminCreateUser(t, base, adminToken, collab)
	}
	testutil.Login(t, base, collab, "password123")

	resp, _ = testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+owner+"/app/collaborators/"+collab, token, map[string]string{
		"permission": "read",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add collaborator: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app/collaborators", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list collaborators: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/protected-branches", token, map[string]any{
		"branch": "main", "required_checks": []string{}, "require_reviews": 0,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("protect branch: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app/protected-branches", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list protected branches: %d", resp.StatusCode)
	}

	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs", token, map[string]string{
		"name": "org" + suffix[len(suffix)-4:], "display_name": "E2E Org", "description": "test",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create org: %d %v", resp.StatusCode, out)
	}
	orgName, _ := out["name"].(string)

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/repos", token, map[string]any{
		"name": "team-app", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("org repo: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/orgs", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list orgs: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/issues/"+itoa(issueNum)+"/close", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("close issue: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/fork", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fork: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/search?q=app", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/user/repos", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list repos: %d", resp.StatusCode)
	}

	if !allowRegister {
		adminToken := testutil.Login(t, base, "admin", "admin")
		resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/admin/users", adminToken, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("admin users: %d", resp.StatusCode)
		}
	}
}

func testGitPush(t *testing.T, gitBase, owner, repo, user, password string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	work := t.TempDir()
	remote := fmt.Sprintf("%s/%s/%s.git", strings.TrimRight(gitBase, "/"), owner, repo)
	cloneURL := strings.Replace(remote, "://", "://"+user+":"+password+"@", 1)

	runGit(t, work, "clone", cloneURL, "repo")
	repoDir := filepath.Join(work, "repo")
	setupGit(t, repoDir)
	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("e2e\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "e2e commit")
	runGit(t, repoDir, "branch", "-M", "main")
	runGit(t, repoDir, "push", "--force", "origin", "main")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s %v", args, out, err)
	}
}

func setupGit(t *testing.T, dir string) {
	t.Helper()
	for _, cfg := range [][]string{
		{"config", "user.email", "e2e@test.local"},
		{"config", "user.name", "e2e"},
	} {
		runGit(t, dir, cfg...)
	}
}

func deployBaseURL() string {
	if v := os.Getenv("GOVNOHUB_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://govnohub.local"
}

func deployGitURL() string {
	if v := os.Getenv("GOVNOHUB_GIT_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://git.govnohub.local"
}

func waitDeployed(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("deployment not reachable at %s", base)
}
