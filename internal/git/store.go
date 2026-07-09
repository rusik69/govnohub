package gitstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Store struct {
	root string
}

func NewStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) RepoPath(owner, name string) string {
	return filepath.Join(s.root, owner, name+".git")
}

func (s *Store) Exists(owner, name string) bool {
	_, err := os.Stat(s.RepoPath(owner, name))
	return err == nil
}

func (s *Store) Init(ctx context.Context, owner, name string) error {
	path := s.RepoPath(owner, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, err := git.PlainInit(path, true)
	return err
}

func (s *Store) Open(owner, name string) (*git.Repository, error) {
	return git.PlainOpen(s.RepoPath(owner, name))
}

func (s *Store) ReceivePack(owner, name string, r io.Reader, w io.Writer) error {
	return s.runGit(owner, name, "receive-pack", r, w)
}

func (s *Store) UploadPack(owner, name string, r io.Reader, w io.Writer) error {
	return s.runGit(owner, name, "upload-pack", r, w)
}

func (s *Store) runGit(owner, name, cmd string, r io.Reader, w io.Writer) error {
	path := s.RepoPath(owner, name)
	c := exec.Command("git", cmd, "--stateless-rpc", path)
	c.Dir = path
	c.Stdin = r
	c.Stdout = w
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("git %s: %w", cmd, err)
	}
	return nil
}

func (s *Store) ListBranchSHAs(owner, name string) (map[string]string, error) {
	path := s.RepoPath(owner, name)
	cmd := exec.Command("git", "-C", path, "for-each-ref", "refs/heads", "--format=%(refname:short) %(objectname)")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	refs := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		refs[parts[0]] = parts[1]
	}
	return refs, nil
}

type TreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
	Size int64  `json:"size,omitempty"`
}

type CommitInfo struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

func (s *Store) GetTree(owner, name, ref, path string) ([]TreeEntry, error) {
	repo, err := s.Open(owner, name)
	if err != nil {
		return nil, err
	}
	commit, err := resolveCommit(repo, ref)
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	if path != "" && path != "/" {
		for _, part := range strings.Split(path, "/") {
			entry, err := tree.FindEntry(part)
			if err != nil {
				return nil, err
			}
			tree, err = repo.TreeObject(entry.Hash)
			if err != nil {
				return nil, err
			}
		}
	}
	var entries []TreeEntry
	for _, e := range tree.Entries {
		t := "file"
		if e.Mode == filemode.Dir {
			t = "dir"
		}
		entries = append(entries, TreeEntry{Path: e.Name, Type: t, SHA: e.Hash.String()})
	}
	return entries, nil
}

func (s *Store) GetBlob(owner, name, ref, path string) ([]byte, error) {
	repo, err := s.Open(owner, name)
	if err != nil {
		return nil, err
	}
	commit, err := resolveCommit(repo, ref)
	if err != nil {
		return nil, err
	}
	file, err := commit.File(path)
	if err != nil {
		return nil, err
	}
	reader, err := file.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (s *Store) GetCommits(owner, name, ref string, limit int) ([]CommitInfo, error) {
	repo, err := s.Open(owner, name)
	if err != nil {
		return nil, err
	}
	refObj, err := repo.Reference(plumbing.ReferenceName("refs/heads/"+ref), true)
	if err != nil {
		refObj, err = repo.Head()
		if err != nil {
			return nil, err
		}
	}
	commit, err := repo.CommitObject(refObj.Hash())
	if err != nil {
		return nil, err
	}
	iter, err := repo.Log(&git.LogOptions{From: commit.Hash})
	if err != nil {
		return nil, err
	}
	var commits []CommitInfo
	count := 0
	err = iter.ForEach(func(c *object.Commit) error {
		if count >= limit {
			return fmt.Errorf("done")
		}
		commits = append(commits, CommitInfo{
			SHA:     c.Hash.String(),
			Message: strings.Split(c.Message, "\n")[0],
			Author:  c.Author.Name,
			Date:    c.Author.When.Format("2006-01-02T15:04:05Z"),
		})
		count++
		return nil
	})
	if err != nil && err.Error() != "done" {
		return nil, err
	}
	return commits, nil
}

func (s *Store) UpdateHead(owner, name, branch string) (string, error) {
	repo, err := s.Open(owner, name)
	if err != nil {
		return "", err
	}
	ref, err := repo.Reference(plumbing.ReferenceName("refs/heads/"+branch), true)
	if err != nil {
		return "", err
	}
	return ref.Hash().String(), nil
}

func (s *Store) CreateBranch(owner, name, branch, base string) error {
	repo, err := s.Open(owner, name)
	if err != nil {
		return err
	}
	baseRef, err := repo.Reference(plumbing.ReferenceName("refs/heads/"+base), true)
	if err != nil {
		return err
	}
	refName := plumbing.NewBranchReferenceName(branch)
	ref := plumbing.NewHashReference(refName, baseRef.Hash())
	return repo.Storer.SetReference(ref)
}

func (s *Store) Diff(owner, name, baseSHA, headSHA string) (string, error) {
	path := s.RepoPath(owner, name)
	cmd := exec.Command("git", "diff", baseSHA, headSHA)
	cmd.Dir = path
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}

func (s *Store) Merge(owner, name, baseBranch, headBranch string, squash bool) (string, error) {
	path := s.RepoPath(owner, name)
	wt, err := os.MkdirTemp("", "govnohub-merge-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(wt)

	if err := exec.Command("git", "-C", path, "worktree", "add", "--detach", wt, baseBranch).Run(); err != nil {
		return "", fmt.Errorf("worktree add: %w", err)
	}
	defer exec.Command("git", "-C", path, "worktree", "remove", "--force", wt).Run()

	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=govnohub", "GIT_AUTHOR_EMAIL=govnohub@local",
		"GIT_COMMITTER_NAME=govnohub", "GIT_COMMITTER_EMAIL=govnohub@local",
	)
	mergeArgs := []string{"-C", wt, "merge", headBranch}
	if squash {
		mergeArgs = append(mergeArgs, "--squash")
	}
	cmd := exec.Command("git", mergeArgs...)
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("merge: %w", err)
	}
	if squash {
		cmd = exec.Command("git", "-C", wt, "commit", "-m", "Squash merge "+headBranch)
		cmd.Env = env
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("commit: %w", err)
		}
	}
	out, err := exec.Command("git", "-C", wt, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func resolveCommit(repo *git.Repository, ref string) (*object.Commit, error) {
	if ref == "" {
		head, err := repo.Head()
		if err != nil {
			return nil, err
		}
		return repo.CommitObject(head.Hash())
	}
	refObj, err := repo.Reference(plumbing.ReferenceName("refs/heads/"+ref), true)
	if err != nil {
		hash := plumbing.NewHash(ref)
		return repo.CommitObject(hash)
	}
	return repo.CommitObject(refObj.Hash())
}
