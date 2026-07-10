//go:build deploy

package e2e

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func setupGitRepo(t *testing.T) (base, gitURL, gitSSHURL, owner, token string) {
	t.Helper()
	env := testenv.NewLocalDeploy(t)
	t.Cleanup(env.Cleanup)
	owner = "git" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token = testutil.RegisterAndLogin(t, env.URL, owner)
	createRepo(t, env.URL, token, owner, "app")
	return env.URL, env.GitURL, env.GitSSHURL, owner, token
}

func TestGitClone(t *testing.T) {
	_, gitURL, _, owner, _ := setupGitRepo(t)
	requireGit(t)
	work := t.TempDir()
	repoDir := gitClone(t, work, gitRemoteURL(gitURL, owner, "app", owner, "password123"), "repo")
	if _, err := os.Stat(filepath.Join(repoDir, "README.md")); err != nil {
		t.Fatal("expected README.md in clone")
	}
}

func TestGitPushUpdatesCommits(t *testing.T) {
	base, gitURL, _, owner, token := setupGitRepo(t)
	repo := owner + "/app"

	testGitPush(t, gitURL, owner, "app", owner, "password123")

	resp, commits := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/commits", token, nil)
	requireStatus(t, resp, http.StatusOK, "list commits")
	if len(commits) == 0 {
		t.Fatal("expected commits after push")
	}
	msg, _ := commits[0]["message"].(string)
	if msg != "e2e commit" {
		t.Fatalf("unexpected commit message: %q", msg)
	}
}

func TestGitPushContents(t *testing.T) {
	base, gitURL, _, owner, token := setupGitRepo(t)
	repo := owner + "/app"

	testGitPush(t, gitURL, owner, "app", owner, "password123")

	req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/repos/"+repo+"/contents/README.md", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusOK, "get contents")
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("expected content body")
	}
}

func TestGitPull(t *testing.T) {
	_, gitURL, _, owner, _ := setupGitRepo(t)
	requireGit(t)
	work := t.TempDir()
	url := gitRemoteURL(gitURL, owner, "app", owner, "password123")

	clone1 := gitClone(t, work, url, "clone1")
	clone2 := gitClone(t, work, url, "clone2")

	newFile := filepath.Join(clone1, "feature.txt")
	if err := os.WriteFile(newFile, []byte("from clone1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, clone1, "add", "feature.txt")
	runGit(t, clone1, "commit", "-m", "add feature.txt")
	runGit(t, clone1, "push", "origin", "main")

	runGit(t, clone2, "pull", "origin", "main")
	got := readFile(t, filepath.Join(clone2, "feature.txt"))
	if got != "from clone1\n" {
		t.Fatalf("pull content=%q", got)
	}
}

func TestGitFetch(t *testing.T) {
	_, gitURL, _, owner, _ := setupGitRepo(t)
	requireGit(t)
	work := t.TempDir()
	url := gitRemoteURL(gitURL, owner, "app", owner, "password123")

	clone1 := gitClone(t, work, url, "clone1")
	clone2 := gitClone(t, work, url, "clone2")

	if err := os.WriteFile(filepath.Join(clone1, "fetch.txt"), []byte("fetch me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, clone1, "add", "fetch.txt")
	runGit(t, clone1, "commit", "-m", "add fetch.txt")
	runGit(t, clone1, "push", "origin", "main")

	runGit(t, clone2, "fetch", "origin")
	runGit(t, clone2, "merge", "origin/main")
	got := readFile(t, filepath.Join(clone2, "fetch.txt"))
	if got != "fetch me\n" {
		t.Fatalf("fetch content=%q", got)
	}
}

func TestGitPushFeatureBranch(t *testing.T) {
	base, gitURL, _, owner, token := setupGitRepo(t)
	repo := owner + "/app"
	requireGit(t)

	work := t.TempDir()
	repoDir := gitClone(t, work, gitRemoteURL(gitURL, owner, "app", owner, "password123"), "repo")
	runGit(t, repoDir, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(repoDir, "branch.txt"), []byte("on feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "branch.txt")
	runGit(t, repoDir, "commit", "-m", "feature branch commit")
	runGit(t, repoDir, "push", "-u", "origin", "feature")

	resp, commits := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/commits?ref=feature", token, nil)
	requireStatus(t, resp, http.StatusOK, "list feature commits")
	if len(commits) == 0 {
		t.Fatal("expected commits on feature branch")
	}
	msg, _ := commits[0]["message"].(string)
	if msg != "feature branch commit" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestGitPushWithPAT(t *testing.T) {
	base, gitURL, _, owner, token := setupGitRepo(t)
	repo := owner + "/app"
	requireGit(t)

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/user/tokens", token, map[string]any{
		"name": "git", "scopes": []string{"repo", "repo:write"},
	})
	requireStatus(t, resp, http.StatusOK, "create PAT")
	pat, _ := out["token"].(string)

	work := t.TempDir()
	repoDir := gitClone(t, work, gitRemoteURL(gitURL, owner, "app", owner, pat), "repo")
	if err := os.WriteFile(filepath.Join(repoDir, "pat.txt"), []byte("via pat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "pat.txt")
	runGit(t, repoDir, "commit", "-m", "pat push")
	runGit(t, repoDir, "push", "origin", "main")

	resp, commits := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/commits", token, nil)
	requireStatus(t, resp, http.StatusOK, "list commits")
	if len(commits) == 0 {
		t.Fatal("expected commits after PAT push")
	}
	msg, _ := commits[0]["message"].(string)
	if msg != "pat push" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestGitUnauthorized(t *testing.T) {
	_, gitURL, _, owner, _ := setupGitRepo(t)
	requireGit(t)
	work := t.TempDir()
	badURL := gitRemoteURL(gitURL, owner, "app", owner, "wrong-password")
	if _, err := runGitAllowFail(t, work, "clone", badURL, "repo"); err == nil {
		t.Fatal("expected clone to fail with bad credentials")
	}
}

func TestGitSSHClone(t *testing.T) {
	base, _, gitSSHURL, owner, token := setupGitRepo(t)
	privKey := registerSSHKey(t, base, token)
	work := t.TempDir()
	repoDir := gitCloneSSH(t, work, gitSSHRemoteURL(gitSSHURL, owner, "app"), privKey, "repo")
	if _, err := os.Stat(filepath.Join(repoDir, "README.md")); err != nil {
		t.Fatal("expected README.md in ssh clone")
	}
}

func TestGitSSHPushPull(t *testing.T) {
	base, _, gitSSHURL, owner, token := setupGitRepo(t)
	privKey := registerSSHKey(t, base, token)
	repo := owner + "/app"
	url := gitSSHRemoteURL(gitSSHURL, owner, "app")

	work := t.TempDir()
	clone1 := gitCloneSSH(t, work, url, privKey, "clone1")
	clone2 := gitCloneSSH(t, work, url, privKey, "clone2")

	if err := os.WriteFile(filepath.Join(clone1, "ssh.txt"), []byte("via ssh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitSSH(t, clone1, privKey, "add", "ssh.txt")
	runGitSSH(t, clone1, privKey, "commit", "-m", "ssh push")
	runGitSSH(t, clone1, privKey, "push", "origin", "main")

	runGitSSH(t, clone2, privKey, "pull", "origin", "main")
	got := readFile(t, filepath.Join(clone2, "ssh.txt"))
	if got != "via ssh\n" {
		t.Fatalf("ssh pull content=%q", got)
	}

	resp, commits := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/commits", token, nil)
	requireStatus(t, resp, http.StatusOK, "list commits")
	if len(commits) == 0 {
		t.Fatal("expected commits after ssh push")
	}
	msg, _ := commits[0]["message"].(string)
	if msg != "ssh push" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestGitSSHUnauthorized(t *testing.T) {
	base, _, gitSSHURL, owner, token := setupGitRepo(t)
	registerSSHKey(t, base, token) // register a key for the owner, but use a different private key
	_, wrongKey := generateSSHKeyPair(t)

	work := t.TempDir()
	url := gitSSHRemoteURL(gitSSHURL, owner, "app")
	if _, err := runGitSSHAllowFail(t, work, wrongKey, "clone", url, "repo"); err == nil {
		t.Fatal("expected ssh clone to fail without registered key")
	}
}
