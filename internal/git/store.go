package gitstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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

// Remove deletes the bare git repository for the given owner/name from disk.
func (s *Store) Remove(owner, name string) error {
	path := s.RepoPath(owner, name)
	return os.RemoveAll(path)
}

func (s *Store) Init(ctx context.Context, owner, name string) error {
	if s.Exists(owner, name) {
		return nil
	}
	path := s.RepoPath(owner, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, err := git.PlainInit(path, true)
	return err
}

func (s *Store) SeedMainBranch(owner, repoName, branch string) (string, error) {
	repo, err := s.Open(owner, repoName)
	if err != nil {
		return "", err
	}
	content := []byte("# " + repoName + "\n\nInitial commit.\n")
	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	if err != nil {
		return "", err
	}
	if _, err := w.Write(content); err != nil {
		return "", err
	}
	w.Close()
	blobHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return "", err
	}

	tree := &object.Tree{Entries: []object.TreeEntry{{
		Name: "README.md", Mode: filemode.Regular, Hash: blobHash,
	}}}
	treeObj := repo.Storer.NewEncodedObject()
	if err := tree.Encode(treeObj); err != nil {
		return "", err
	}
	treeHash, err := repo.Storer.SetEncodedObject(treeObj)
	if err != nil {
		return "", err
	}

	now := time.Now()
	sig := object.Signature{Name: "govnohub", Email: "govnohub@local", When: now}
	commit := &object.Commit{
		Message:  "Initial commit",
		TreeHash: treeHash,
		Author:   sig,
		Committer: sig,
	}
	commitObj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(commitObj); err != nil {
		return "", err
	}
	commitHash, err := repo.Storer.SetEncodedObject(commitObj)
	if err != nil {
		return "", err
	}
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), commitHash)
	if err := repo.Storer.SetReference(ref); err != nil {
		return "", err
	}
	head := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(branch))
	if err := repo.Storer.SetReference(head); err != nil {
		return "", err
	}
	return commitHash.String(), nil
}

func (s *Store) Open(owner, name string) (*git.Repository, error) {
	return git.PlainOpen(s.RepoPath(owner, name))
}

func (s *Store) UploadPackSSH(owner, name string, r io.Reader, w io.Writer, extraEnv []string, stateless bool, stderr io.Writer) error {
	return s.runGitBidirectional(owner, name, "upload-pack", r, w, extraEnv, stateless, stderr)
}

func (s *Store) ReceivePackSSH(owner, name string, r io.Reader, w io.Writer, extraEnv []string, stateless bool, stderr io.Writer) error {
	return s.runGitBidirectional(owner, name, "receive-pack", r, w, extraEnv, stateless, stderr)
}

func (s *Store) ReceivePack(owner, name string, r io.Reader, w io.Writer) error {
	return s.runGit(owner, name, "receive-pack", true, r, w)
}

func (s *Store) UploadPack(owner, name string, r io.Reader, w io.Writer) error {
	return s.runGit(owner, name, "upload-pack", true, r, w)
}

func (s *Store) runGit(owner, name, cmd string, stateless bool, r io.Reader, w io.Writer) error {
	path := s.RepoPath(owner, name)
	args := []string{"-c", "safe.directory=*", cmd}
	if stateless {
		args = append(args, "--stateless-rpc")
	}
	args = append(args, path)
	c := exec.Command("git", args...)
	c.Dir = path
	c.Stdin = r
	c.Stdout = w
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("git %s: %w", cmd, err)
	}
	return nil
}

// runGitBidirectional runs git upload/receive-pack with separate copy goroutines.
// Required when stdin and stdout are the same SSH channel to avoid deadlocks.
func (s *Store) runGitBidirectional(owner, name, cmd string, in io.Reader, out io.Writer, extraEnv []string, stateless bool, stderr io.Writer) error {
	path := s.RepoPath(owner, name)
	args := []string{"-c", "safe.directory=*", cmd}
	if stateless {
		args = append(args, "--stateless-rpc")
	}
	args = append(args, path)
	c := exec.Command("git", args...)
	c.Env = append(os.Environ(), extraEnv...)
	var stderrBuf bytes.Buffer
	if stderr != nil {
		c.Stderr = io.MultiWriter(stderr, &stderrBuf)
	} else {
		c.Stderr = &stderrBuf
	}

	stdin, err := c.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return err
	}
	if err := c.Start(); err != nil {
		return fmt.Errorf("git %s start: %w", cmd, err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(stdin, in)
		stdin.Close()
	}()
	go func() {
		defer wg.Done()
		io.Copy(out, stdout)
	}()
	wg.Wait()

	if err := c.Wait(); err != nil {
		return fmt.Errorf("git %s: %w: %s", cmd, err, strings.TrimSpace(stderrBuf.String()))
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

// TagInfo represents a git tag with its metadata.
type TagInfo struct {
	Name    string `json:"name"`
	Ref     string `json:"ref"`
	HeadSHA string `json:"head_sha"`
}

// ListTags lists all tags in a repository using git for-each-ref.
func (s *Store) ListTags(owner, name string) ([]TagInfo, error) {
	path := s.RepoPath(owner, name)
	cmd := exec.Command("git", "-C", path, "for-each-ref", "refs/tags",
		"--format=%(refname:short) %(objectname) %(*objectname)")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var tags []TagInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		sha := parts[1]
		ref := "refs/tags/" + name
		// For annotated tags: parts[1] is the tag object SHA, parts[2] is the commit SHA
		// For lightweight tags: parts[1] is the commit SHA, no *objectname
		headSHA := sha
		if len(parts) >= 3 && parts[2] != "" {
			headSHA = parts[2]
		}
		tags = append(tags, TagInfo{Name: name, Ref: ref, HeadSHA: headSHA})
	}
	if tags == nil {
		tags = []TagInfo{}
	}
	return tags, nil
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

// GetAllFiles recursively walks the entire tree at the given ref and returns all file paths.
func (s *Store) GetAllFiles(owner, name, ref string) ([]string, error) {
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
	var files []string
	walker := object.NewTreeWalker(tree, false, nil)
	defer walker.Close()
	for {
		name, entry, err := walker.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if entry.Mode == filemode.Regular || entry.Mode == filemode.Executable || entry.Mode == filemode.Symlink {
			files = append(files, name)
		}
	}
	return files, nil
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

// ChangedFile represents a file changed in a pull request.
type ChangedFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"` // added, modified, removed
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Changes   int    `json:"changes"`
}

// GetPRFiles returns the list of files changed between baseBranch and headBranch.
func (s *Store) GetPRFiles(owner, name, baseBranch, headBranch string) ([]ChangedFile, error) {
	path := s.RepoPath(owner, name)
	cmd := exec.Command("git", "-C", path, "diff", "--numstat", baseBranch, headBranch)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --numstat: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []ChangedFile
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		// Handle binary files: "-" "-" filename
		addsStr := parts[0]
		delsStr := parts[1]
		filename := strings.Join(parts[2:], " ")
		// Determine status: if filename contains " => ", it's a rename
		status := "modified"
		if addsStr == "-" || delsStr == "-" {
			status = "modified"
			if addsStr == "0" && delsStr != "0" {
				status = "removed"
			} else if delsStr == "0" && addsStr != "0" {
				status = "added"
			}
		} else {
			adds, _ := strconv.Atoi(addsStr)
			dels, _ := strconv.Atoi(delsStr)
			if adds > 0 && dels == 0 {
				status = "added"
			} else if adds == 0 && dels > 0 {
				status = "removed"
			}
		}
		files = append(files, ChangedFile{
			Filename:  filename,
			Status:    status,
			Additions: atoiSafe(addsStr),
			Deletions: atoiSafe(delsStr),
			Changes:   atoiSafe(addsStr) + atoiSafe(delsStr),
		})
	}
	if files == nil {
		files = []ChangedFile{}
	}
	return files, nil
}

func atoiSafe(s string) int {
	if s == "-" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

// GetPRCommits returns commits in headBranch that are not reachable from baseBranch.
func (s *Store) GetPRCommits(owner, name, baseBranch, headBranch string, limit int) ([]CommitInfo, error) {
	path := s.RepoPath(owner, name)
	cmd := exec.Command("git", "-C", path, "log",
		"--format=%H||%s||%an||%aI",
		fmt.Sprintf("--max-count=%d", limit),
		baseBranch+".."+headBranch)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "||", 4)
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, CommitInfo{
			SHA:     parts[0],
			Message: parts[1],
			Author:  parts[2],
			Date:    parts[3],
		})
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

// CreateTag creates a lightweight tag pointing to the given ref (branch or SHA).
func (s *Store) CreateTag(owner, name, tagName, ref string) error {
	repo, err := s.Open(owner, name)
	if err != nil {
		return err
	}
	// Try to resolve the ref as a branch first, then as a raw SHA.
	hash, err := s.resolveRef(repo, ref)
	if err != nil {
		return err
	}
	refName := plumbing.NewTagReferenceName(tagName)
	tagRef := plumbing.NewHashReference(refName, hash)
	return repo.Storer.SetReference(tagRef)
}

// resolveRef resolves a branch name or commit SHA to a hash.
func (s *Store) resolveRef(repo *git.Repository, ref string) (plumbing.Hash, error) {
	// Try as a branch reference.
	br, err := repo.Reference(plumbing.ReferenceName("refs/heads/"+ref), true)
	if err == nil {
		return br.Hash(), nil
	}
	// Try as a raw SHA.
	h := plumbing.NewHash(ref)
	if h.IsZero() {
		return plumbing.ZeroHash, fmt.Errorf("cannot resolve ref: %s", ref)
	}
	return h, nil
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

// CompareResult holds the result of comparing two commits/branches.
type CompareResult struct {
	BaseCommit     CommitInfo    `json:"base_commit"`
	HeadCommit     CommitInfo    `json:"head_commit"`
	MergeBaseCommit CommitInfo   `json:"merge_base_commit"`
	Status         string        `json:"status"` // identical, ahead, behind, diverged
	AheadBy        int           `json:"ahead_by"`
	BehindBy       int           `json:"behind_by"`
	TotalCommits   int           `json:"total_commits"`
	Commits        []CommitInfo  `json:"commits"`
	Files          []ChangedFile `json:"files"`
	Diff           string        `json:"diff,omitempty"`
}

// CompareCommits compares two refs (branches, tags, or SHAs) and returns
// a structured result including merge base, ahead/behind counts, commits,
// changed files, and the unified diff.
func (s *Store) CompareCommits(owner, name, baseRef, headRef string) (*CompareResult, error) {
	repo, err := s.Open(owner, name)
	if err != nil {
		return nil, err
	}
	path := s.RepoPath(owner, name)

	baseCommit, err := resolveCommit(repo, baseRef)
	if err != nil {
		return nil, fmt.Errorf("resolve base ref %q: %w", baseRef, err)
	}
	headCommit, err := resolveCommit(repo, headRef)
	if err != nil {
		return nil, fmt.Errorf("resolve head ref %q: %w", headRef, err)
	}

	baseSHA := baseCommit.Hash.String()
	headSHA := headCommit.Hash.String()

	// Merge base
	mergeBaseSHA := ""
	mbOut, mbErr := exec.Command("git", "-C", path, "merge-base", baseSHA, headSHA).Output()
	if mbErr == nil {
		mergeBaseSHA = strings.TrimSpace(string(mbOut))
	}

	// Ahead / behind counts
	ahead, behind := 0, 0
	abOut, abErr := exec.Command("git", "-C", path, "rev-list", "--count", "--left-right", baseSHA+"..."+headSHA).Output()
	if abErr == nil {
		parts := strings.Fields(string(abOut))
		if len(parts) == 2 {
			behind, _ = strconv.Atoi(parts[0])
			ahead, _ = strconv.Atoi(parts[1])
		}
	}

	// Status
	status := "diverged"
	switch {
	case ahead > 0 && behind == 0:
		status = "ahead"
	case behind > 0 && ahead == 0:
		status = "behind"
	case ahead == 0 && behind == 0:
		status = "identical"
	}

	// Commits in head not in base
	var commits []CommitInfo
	cOut, cErr := exec.Command("git", "-C", path, "log", "--format=%H||%s||%an||%aI", "--max-count=50", baseSHA+".."+headSHA).Output()
	if cErr == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(cOut)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "||", 4)
			if len(parts) < 4 {
				continue
			}
			commits = append(commits, CommitInfo{
				SHA: parts[0], Message: parts[1], Author: parts[2], Date: parts[3],
			})
		}
	}
	if commits == nil {
		commits = []CommitInfo{}
	}

	// Changed files via --numstat
	var files []ChangedFile
	fOut, fErr := exec.Command("git", "-C", path, "diff", "--numstat", baseSHA, headSHA).Output()
	if fErr == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(fOut)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			addsStr, delsStr := parts[0], parts[1]
			filename := strings.Join(parts[2:], " ")

			status := "modified"
			adds := atoiSafe(addsStr)
			dels := atoiSafe(delsStr)
			if adds > 0 && dels == 0 {
				status = "added"
			} else if adds == 0 && dels > 0 {
				status = "removed"
			}
			files = append(files, ChangedFile{
				Filename:  filename,
				Status:    status,
				Additions: adds,
				Deletions: dels,
				Changes:   adds + dels,
			})
		}
	}
	if files == nil {
		files = []ChangedFile{}
	}

	// Full diff
	diffBytes, _ := exec.Command("git", "-C", path, "diff", baseSHA, headSHA).Output()
	diffStr := string(diffBytes)

	// Merge base commit info
	var mergeBaseCommitInfo CommitInfo
	if mergeBaseSHA != "" {
		if mbc, mbErr := repo.CommitObject(plumbing.NewHash(mergeBaseSHA)); mbErr == nil {
			mergeBaseCommitInfo = CommitInfo{
				SHA: mbc.Hash.String(), Message: strings.Split(mbc.Message, "\n")[0],
				Author: mbc.Author.Name, Date: mbc.Author.When.Format("2006-01-02T15:04:05Z"),
			}
		}
	}

	makeInfo := func(c *object.Commit) CommitInfo {
		return CommitInfo{
			SHA: c.Hash.String(), Message: strings.Split(c.Message, "\n")[0],
			Author: c.Author.Name, Date: c.Author.When.Format("2006-01-02T15:04:05Z"),
		}
	}

	return &CompareResult{
		BaseCommit:      makeInfo(baseCommit),
		HeadCommit:      makeInfo(headCommit),
		MergeBaseCommit: mergeBaseCommitInfo,
		Status:          status,
		AheadBy:         ahead,
		BehindBy:        behind,
		TotalCommits:    len(commits),
		Commits:         commits,
		Files:           files,
		Diff:            diffStr,
	}, nil
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
	sha := strings.TrimSpace(string(out))
	if err := exec.Command("git", "--git-dir", path, "update-ref", "refs/heads/"+baseBranch, sha).Run(); err != nil {
		return "", fmt.Errorf("update-ref: %w", err)
	}
	return sha, nil
}

// MergeBaseIntoHead merges the base branch into the head branch (updates head ref).
// This is used to update a PR branch with the latest changes from the base branch.
// Returns the new head SHA after merge.
func (s *Store) MergeBaseIntoHead(owner, name, headBranch, baseBranch string) (string, error) {
	path := s.RepoPath(owner, name)
	wt, err := os.MkdirTemp("", "govnohub-merge-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(wt)

	if err := exec.Command("git", "-C", path, "worktree", "add", "--detach", wt, headBranch).Run(); err != nil {
		return "", fmt.Errorf("worktree add: %w", err)
	}
	defer exec.Command("git", "-C", path, "worktree", "remove", "--force", wt).Run()

	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=govnohub", "GIT_AUTHOR_EMAIL=govnohub@local",
		"GIT_COMMITTER_NAME=govnohub", "GIT_COMMITTER_EMAIL=govnohub@local",
	)

	// Merge base into head
	cmd := exec.Command("git", "-C", wt, "merge", baseBranch)
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("merge base into head: %w", err)
	}

	out, err := exec.Command("git", "-C", wt, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(out))

	// Update the head branch ref
	if err := exec.Command("git", "--git-dir", path, "update-ref", "refs/heads/"+headBranch, sha).Run(); err != nil {
		return "", fmt.Errorf("update-ref head: %w", err)
	}
	return sha, nil
}

func (s *Store) CanMerge(owner, name, baseBranch, headBranch string) (bool, error) {
	path := s.RepoPath(owner, name)
	cmd := exec.Command("git", "-C", path, "merge-tree", "--write-tree", baseBranch, headBranch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// merge-tree exits non-zero on conflicts
		if strings.Contains(string(out), "CONFLICT") || strings.Contains(string(out), "conflict") {
			return false, nil
		}
		return false, fmt.Errorf("merge-tree: %w: %s", err, out)
	}
	return true, nil
}

// Archive creates a tar.gz archive of the repository at the given ref (branch, tag, or SHA).
// It returns an io.ReadCloser that streams the archive data. The caller must close it.
func (s *Store) Archive(owner, name, ref string) (io.ReadCloser, error) {
	path := s.RepoPath(owner, name)
	cmd := exec.Command("git", "-C", path, "archive", "--format=tar.gz", ref)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("git archive stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("git archive start: %w", err)
	}
	return &archiveReader{rc: stdout, cmd: cmd}, nil
}

// archiveReader wraps a pipe to wait for the command to finish on Close.
type archiveReader struct {
	rc  io.ReadCloser
	cmd *exec.Cmd
}

func (a *archiveReader) Read(p []byte) (int, error) {
	return a.rc.Read(p)
}

func (a *archiveReader) Close() error {
	err := a.rc.Close()
	if werr := a.cmd.Wait(); werr != nil && err == nil {
		err = werr
	}
	return err
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

// CreateBlob creates a git blob object with the given content and returns its SHA.
func (s *Store) CreateBlob(owner, name string, content []byte) (string, error) {
	repo, err := s.Open(owner, name)
	if err != nil {
		return "", err
	}
	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	if err != nil {
		return "", err
	}
	if _, err := w.Write(content); err != nil {
		return "", err
	}
	w.Close()
	hash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

// TreeEntryInput represents a tree entry in a create-tree request.
type TreeEntryInput struct {
	Path string `json:"path"`
	Mode string `json:"mode"` // "100644", "100755", "040000", "120000"
	Type string `json:"type"` // "blob", "tree", "commit"
	SHA  string `json:"sha"`
}

// CreateTree creates a git tree object from the given entries and returns its SHA.
func (s *Store) CreateTree(owner, name string, entries []TreeEntryInput) (string, error) {
	repo, err := s.Open(owner, name)
	if err != nil {
		return "", err
	}
	var treeEntries []object.TreeEntry
	for _, e := range entries {
		mode, err := parseFileMode(e.Mode)
		if err != nil {
			return "", fmt.Errorf("invalid mode %q: %w", e.Mode, err)
		}
		hash := plumbing.NewHash(e.SHA)
		if hash.IsZero() {
			return "", fmt.Errorf("invalid sha: %s", e.SHA)
		}
		treeEntries = append(treeEntries, object.TreeEntry{
			Name: e.Path,
			Mode: mode,
			Hash: hash,
		})
	}
	tree := &object.Tree{Entries: treeEntries}
	treeObj := repo.Storer.NewEncodedObject()
	if err := tree.Encode(treeObj); err != nil {
		return "", err
	}
	hash, err := repo.Storer.SetEncodedObject(treeObj)
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

func parseFileMode(mode string) (filemode.FileMode, error) {
	switch mode {
	case "100644":
		return filemode.Regular, nil
	case "100755":
		return filemode.Executable, nil
	case "040000":
		return filemode.Dir, nil
	case "120000":
		return filemode.Symlink, nil
	case "160000":
		return filemode.Submodule, nil
	default:
		return filemode.Regular, fmt.Errorf("unknown mode %q", mode)
	}
}

// CommitAuthor represents a commit author or committer.
type CommitAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date,omitempty"`
}

// CreateCommitRequest holds parameters for creating a commit.
type CreateCommitRequest struct {
	Message   string         `json:"message"`
	Tree      string         `json:"tree"`
	Parents   []string       `json:"parents"`
	Author    *CommitAuthor  `json:"author,omitempty"`
	Committer *CommitAuthor  `json:"committer,omitempty"`
}

// CreateCommit creates a git commit object and returns its SHA.
func (s *Store) CreateCommit(owner, name string, req CreateCommitRequest) (string, error) {
	repo, err := s.Open(owner, name)
	if err != nil {
		return "", err
	}

	treeHash := plumbing.NewHash(req.Tree)
	if treeHash.IsZero() {
		return "", fmt.Errorf("invalid tree sha: %s", req.Tree)
	}

	now := time.Now()
	author := object.Signature{Name: "govnohub", Email: "govnohub@local", When: now}
	committer := object.Signature{Name: "govnohub", Email: "govnohub@local", When: now}

	if req.Author != nil {
		author.Name = req.Author.Name
		author.Email = req.Author.Email
		if req.Author.Date != "" {
			if t, err := time.Parse(time.RFC3339, req.Author.Date); err == nil {
				author.When = t
			}
		}
	}
	if req.Committer != nil {
		committer.Name = req.Committer.Name
		committer.Email = req.Committer.Email
		if req.Committer.Date != "" {
			if t, err := time.Parse(time.RFC3339, req.Committer.Date); err == nil {
				committer.When = t
			}
		}
	}

	var parentHashes []plumbing.Hash
	for _, p := range req.Parents {
		h := plumbing.NewHash(p)
		if !h.IsZero() {
			parentHashes = append(parentHashes, h)
		}
	}

	commit := &object.Commit{
		Message:      req.Message,
		TreeHash:     treeHash,
		ParentHashes: parentHashes,
		Author:       author,
		Committer:    committer,
	}
	commitObj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(commitObj); err != nil {
		return "", err
	}
	hash, err := repo.Storer.SetEncodedObject(commitObj)
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

// CommitDetail holds full information about a single commit for the detail view.
type CommitDetail struct {
	SHA            string        `json:"sha"`
	ShortSHA       string        `json:"short_sha"`
	Message        string        `json:"message"`
	AuthorName     string        `json:"author_name"`
	AuthorEmail    string        `json:"author_email"`
	AuthorDate     string        `json:"author_date"`
	CommitterName  string        `json:"committer_name"`
	CommitterEmail string        `json:"committer_email"`
	CommitterDate  string        `json:"committer_date"`
	ParentSHAs     []string      `json:"parent_shas"`
	TreeSHA        string        `json:"tree_sha"`
	FilesChanged   int           `json:"files_changed"`
	Additions      int           `json:"additions"`
	Deletions      int           `json:"deletions"`
	Files          []ChangedFile `json:"files"`
	Diff           string        `json:"diff,omitempty"`
}

// GetCommitDetail returns full details for a single commit identified by ref (branch name or SHA).
func (s *Store) GetCommitDetail(owner, name, ref string) (*CommitDetail, error) {
	repo, err := s.Open(owner, name)
	if err != nil {
		return nil, err
	}
	commit, err := resolveCommit(repo, ref)
	if err != nil {
		return nil, err
	}

	path := s.RepoPath(owner, name)
	sha := commit.Hash.String()

	// Get parent SHAs
	var parentSHAs []string
	for _, p := range commit.ParentHashes {
		parentSHAs = append(parentSHAs, p.String())
	}

	// Stat against first parent (or initial commit stats)
	filesChanged, additions, deletions := 0, 0, 0
	var files []ChangedFile
	diffStr := ""

	if len(commit.ParentHashes) > 0 {
		parentSHA := commit.ParentHashes[0].String()

		// Changed files via --numstat
		fOut, fErr := exec.Command("git", "-C", path, "diff", "--numstat", parentSHA, sha).Output()
		if fErr == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(fOut)), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				parts := strings.Fields(line)
				if len(parts) < 3 {
					continue
				}
				addsStr, delsStr := parts[0], parts[1]
				filename := strings.Join(parts[2:], " ")

				status := "modified"
				adds := atoiSafe(addsStr)
				dels := atoiSafe(delsStr)
				if adds > 0 && dels == 0 {
					status = "added"
				} else if adds == 0 && dels > 0 {
					status = "removed"
				}
				files = append(files, ChangedFile{
					Filename:  filename,
					Status:    status,
					Additions: adds,
					Deletions: dels,
					Changes:   adds + dels,
				})
				filesChanged++
				additions += adds
				deletions += dels
			}
		}

		// Full diff
		dBytes, _ := exec.Command("git", "-C", path, "diff", parentSHA, sha).Output()
		diffStr = string(dBytes)
	} else {
		// Initial commit: count all files in the tree
		tree, err := commit.Tree()
		if err == nil {
			_ = tree.Files().ForEach(func(f *object.File) error {
				files = append(files, ChangedFile{
					Filename:  f.Name,
					Status:    "added",
					Additions: 1,
					Deletions: 0,
					Changes:   1,
				})
				filesChanged++
				additions++
				return nil
			})
		}
	}

	if files == nil {
		files = []ChangedFile{}
	}

	return &CommitDetail{
		SHA:            sha,
		ShortSHA:       sha[:12],
		Message:        commit.Message,
		AuthorName:     commit.Author.Name,
		AuthorEmail:    commit.Author.Email,
		AuthorDate:     commit.Author.When.Format(time.RFC3339),
		CommitterName:  commit.Committer.Name,
		CommitterEmail: commit.Committer.Email,
		CommitterDate:  commit.Committer.When.Format(time.RFC3339),
		ParentSHAs:     parentSHAs,
		TreeSHA:        commit.TreeHash.String(),
		FilesChanged:   filesChanged,
		Additions:      additions,
		Deletions:      deletions,
		Files:          files,
		Diff:           diffStr,
	}, nil
}

// CreateRefParams holds parameters for creating or updating a git reference.
type CreateRefParams struct {
	Ref string `json:"ref"` // e.g. "refs/heads/new-branch" or "refs/tags/v1.0"
	SHA string `json:"sha"`
}

// CreateRef creates or updates a git reference to point to the given SHA.
func (s *Store) CreateRef(owner, name string, params CreateRefParams) error {
	repo, err := s.Open(owner, name)
	if err != nil {
		return err
	}
	hash := plumbing.NewHash(params.SHA)
	if hash.IsZero() {
		return fmt.Errorf("invalid sha: %s", params.SHA)
	}
	refName := plumbing.ReferenceName(params.Ref)
	ref := plumbing.NewHashReference(refName, hash)
	return repo.Storer.SetReference(ref)
}
