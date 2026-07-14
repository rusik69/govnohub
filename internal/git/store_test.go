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

func bareCommitOn(t *testing.T, repoPath, parentSHA, branch, msg string) string {
	t.Helper()
	args := []string{"--git-dir", repoPath, "commit-tree", "-m", msg}
	if parentSHA != "" {
		args = append(args, "-p", parentSHA)
	}
	args = append(args, emptyTree)
	cmd := exec.Command("git", args...)
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
	return sha
}

func bareCommit(t *testing.T, repoPath, branch, msg string) {
	t.Helper()
	bareCommitOn(t, repoPath, "", branch, msg)
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

func TestSeedMainBranchSetsHEAD(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Init(ctx, "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SeedMainBranch("alice", "demo", "main"); err != nil {
		t.Fatal(err)
	}
	path := store.RepoPath("alice", "demo")
	head, err := exec.Command("git", "--git-dir", path, "symbolic-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("symbolic-ref HEAD: %v", err)
	}
	if strings.TrimSpace(string(head)) != "refs/heads/main" {
		t.Fatalf("HEAD=%q", head)
	}
}

func TestExists(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Non-existent repo
	if store.Exists("nobody", "nope") {
		t.Fatal("expected false for non-existent repo")
	}

	// After Init
	if err := store.Init(ctx, "alice", "myrepo"); err != nil {
		t.Fatal(err)
	}
	if !store.Exists("alice", "myrepo") {
		t.Fatal("expected true after Init")
	}

	// Different owner/name
	if store.Exists("alice", "other") {
		t.Fatal("expected false for different repo name")
	}
	if store.Exists("bob", "myrepo") {
		t.Fatal("expected false for different owner")
	}
}

func TestGetTree(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Init(ctx, "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SeedMainBranch("alice", "demo", "main"); err != nil {
		t.Fatal(err)
	}

	// Get tree at root
	entries, err := store.GetTree("alice", "demo", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected tree entries")
	}
	found := false
	for _, e := range entries {
		if e.Path == "README.md" && e.Type == "file" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected README.md in tree entries")
	}
}

func TestGetBlob(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Init(ctx, "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SeedMainBranch("alice", "demo", "main"); err != nil {
		t.Fatal(err)
	}

	content, err := store.GetBlob("alice", "demo", "main", "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty blob content")
	}
	if !strings.Contains(string(content), "demo") {
		t.Fatalf("expected blob to contain repo name, got: %s", string(content))
	}

	// Non-existent file should error
	_, err = store.GetBlob("alice", "demo", "main", "NONEXISTENT.md")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestCreateBranch(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Init(ctx, "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	sha, err := store.SeedMainBranch("alice", "demo", "main")
	if err != nil {
		t.Fatal(err)
	}

	// Create branch from main
	if err := store.CreateBranch("alice", "demo", "feature-1", "main"); err != nil {
		t.Fatal(err)
	}

	path := store.RepoPath("alice", "demo")
	for _, b := range []string{"main", "feature-1"} {
		out, err := exec.Command("git", "--git-dir", path, "rev-parse", "refs/heads/"+b).Output()
		if err != nil {
			t.Fatalf("rev-parse %s: %v", b, err)
		}
		if strings.TrimSpace(string(out)) != sha {
			t.Fatalf("%s SHA mismatch: got %s, want %s", b, strings.TrimSpace(string(out)), sha)
		}
	}
}

func TestDiff(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Init(ctx, "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	mainSHA, err := store.SeedMainBranch("alice", "demo", "main")
	if err != nil {
		t.Fatal(err)
	}

	// Create a second commit on a feature branch
	path := store.RepoPath("alice", "demo")
	featureSHA := bareCommitOn(t, path, mainSHA, "feature", "second commit")

	diff, err := store.Diff("alice", "demo", mainSHA, featureSHA)
	if err != nil {
		t.Fatal(err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
}

func TestGetCommitsLimited(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Init(ctx, "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	_, err = store.SeedMainBranch("alice", "demo", "main")
	if err != nil {
		t.Fatal(err)
	}

	// Get commits with limit
	commits, err := store.GetCommits("alice", "demo", "main", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if commits[0].Message != "Initial commit" {
		t.Fatalf("expected 'Initial commit', got '%s'", commits[0].Message)
	}
}

func TestGetCommitsEmptyRepo(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Init(ctx, "alice", "empty"); err != nil {
		t.Fatal(err)
	}

	// Empty bare repo — GetCommits should fail
	_, err = store.GetCommits("alice", "empty", "main", 10)
	if err == nil {
		t.Fatal("expected error for empty repo with no refs/heads/main")
	}
}

func TestMergeUpdatesBareRef(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background(), "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	path := store.RepoPath("alice", "demo")
	mainSHA := bareCommitOn(t, path, "", "main", "init")
	bareCommitOn(t, path, mainSHA, "feature", "feature work")

	sha, err := store.Merge("alice", "demo", "main", "feature", false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := exec.Command("git", "--git-dir", path, "rev-parse", "refs/heads/main").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != sha {
		t.Fatalf("bare ref=%s want merge sha=%s", got, sha)
	}
}

func TestCanMerge(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background(), "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	path := store.RepoPath("alice", "demo")
	mainSHA := bareCommitOn(t, path, "", "main", "init")
	bareCommitOn(t, path, mainSHA, "feature", "feature work")

	ok, err := store.CanMerge("alice", "demo", "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected mergeable")
	}
}

func TestGetPRFiles(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background(), "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	path := store.RepoPath("alice", "demo")

	// Seed main with initial commit
	mainSHA := bareCommitOn(t, path, "", "main", "init")

	// Create a second commit on feature that "adds" a file
	// We'll create a real commit with a file change using worktree
	wt, err := os.MkdirTemp("", "govnohub-test-prfiles-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(wt)

	// Use a worktree to make a real file change on the feature branch
	exec.Command("git", "-C", path, "worktree", "add", "--detach", wt, mainSHA).Run()
	defer exec.Command("git", "-C", path, "worktree", "remove", "--force", wt).Run()

	runGit(t, wt, "config", "user.email", "test@test.local")
	runGit(t, wt, "config", "user.name", "test")

	// Create two files and commit
	if err := os.WriteFile(filepath.Join(wt, "new-file.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "README.md"), []byte("# Demo\n\nUpdated.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wt, "add", ".")
	runGit(t, wt, "commit", "-m", "feat: add new-file.go and update README")
	runGit(t, wt, "push", path, "HEAD:refs/heads/feature")

	// Get PR files between main and feature
	files, err := store.GetPRFiles("alice", "demo", "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least 1 changed file")
	}
	// Check we see both files — new-file.go (added) and README.md (modified)
	foundNew := false
	foundReadme := false
	for _, f := range files {
		if f.Filename == "new-file.go" {
			foundNew = true
			if f.Status != "added" && f.Status != "modified" {
				t.Fatalf("expected new-file.go to be added or modified, got %s", f.Status)
			}
		}
		if f.Filename == "README.md" {
			foundReadme = true
		}
	}
	if !foundNew {
		t.Fatal("expected new-file.go in changed files")
	}
	if !foundReadme {
		t.Fatal("expected README.md in changed files")
	}
	// Verify additions/deletions are tracked
	var totalAdds, totalDels int
	for _, f := range files {
		totalAdds += f.Additions
		totalDels += f.Deletions
	}
	if totalAdds == 0 && totalDels == 0 {
		t.Fatal("expected non-zero additions or deletions")
	}
}

func TestGetBlame(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Init(ctx, "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	path := store.RepoPath("alice", "demo")

	// Create a commit with a real file using worktree
	mainSHA := bareCommitOn(t, path, "", "main", "init")

	wt, err := os.MkdirTemp("", "govnohub-test-blame-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(wt)

	exec.Command("git", "-C", path, "worktree", "add", "--detach", wt, mainSHA).Run()
	defer exec.Command("git", "-C", path, "worktree", "remove", "--force", wt).Run()

	runGit(t, wt, "config", "user.email", "blame@test.local")
	runGit(t, wt, "config", "user.name", "Blame Tester")

	if err := os.WriteFile(filepath.Join(wt, "main.go"), []byte("package main\n\nfunc main() {\n	println(\"hello\")\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wt, "add", ".")
	// Commit with env vars to ensure correct author
	commitCmd := exec.Command("git", "commit", "-m", "add main.go")
	commitCmd.Dir = wt
	commitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Blame Tester",
		"GIT_AUTHOR_EMAIL=blame@test.local",
		"GIT_COMMITTER_NAME=Blame Tester",
		"GIT_COMMITTER_EMAIL=blame@test.local",
	)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	runGit(t, wt, "push", path, "HEAD:refs/heads/main")

	// Get blame for main.go
	blameLines, err := store.GetBlame("alice", "demo", "main", "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(blameLines) == 0 {
		t.Fatal("expected blame lines")
	}
	// Should have 5 lines
	if len(blameLines) != 5 {
		t.Fatalf("expected 5 blame lines, got %d", len(blameLines))
	}
	// Check line content
	if blameLines[0].Content != "package main" {
		t.Fatalf("expected 'package main', got '%s'", blameLines[0].Content)
	}
	if blameLines[0].ShortSHA == "" {
		t.Fatal("expected non-empty short SHA")
	}
	if blameLines[0].Author != "Blame Tester" {
		t.Fatalf("expected author 'Blame Tester', got '%s'", blameLines[0].Author)
	}
	if blameLines[0].AuthorDate == "" {
		t.Fatal("expected non-empty author date")
	}

	// Non-existent file should error
	_, err = store.GetBlame("alice", "demo", "main", "NONEXISTENT.go")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestGetBlameEmpty(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Init(ctx, "alice", "empty"); err != nil {
		t.Fatal(err)
	}
	path := store.RepoPath("alice", "empty")
	bareCommitOn(t, path, "", "main", "init")

	// Empty repo with no files — blame should fail
	_, err = store.GetBlame("alice", "empty", "main", "nonexistent.go")
	if err == nil {
		t.Fatal("expected error for non-existent file in empty repo")
	}
}
