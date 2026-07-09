package gitstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func TestInitAndTree(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background(), "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	path := store.RepoPath("alice", "demo")
	bareCommit(t, path, "main", "init")

	commits, err := store.GetCommits("alice", "demo", "main", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) == 0 {
		t.Fatal("expected commits")
	}
}

func bareCommit(t *testing.T, repoPath, branch, msg string) {
	t.Helper()
	cmd := exec.Command("git", "--git-dir", repoPath, "commit-tree", "-m", msg, emptyTree)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.local",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.local",
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("commit-tree: %v", err)
	}
	sha := strings.TrimSpace(string(out))
	if err := exec.Command("git", "--git-dir", repoPath, "update-ref", "refs/heads/"+branch, sha).Run(); err != nil {
		t.Fatalf("update-ref: %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	for _, cfg := range [][]string{
		{"config", "user.email", "test@test.local"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", cfg...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git config: %v", err)
		}
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s %v", args, out, err)
	}
}

func TestRepoPath(t *testing.T) {
	store, _ := NewStore(t.TempDir())
	got := store.RepoPath("o", "r")
	if filepath.Base(got) != "r.git" {
		t.Fatalf("path=%s", got)
	}
}
