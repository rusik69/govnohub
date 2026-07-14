package jobqueue

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	gitstore "github.com/rusik69/govnohub/internal/git"
)

// ArchiveJob creates a tar.gz archive of a repository ref and writes it to a
// temporary file. The caller is responsible for cleaning up the result path.
type ArchiveJob struct {
	Store    *gitstore.Store
	Owner    string
	RepoName string
	Ref      string
	Result   string // populated after execution with the archive file path
	err      error
	done     chan struct{}
}

// NewArchiveJob creates a new archive generation job.
func NewArchiveJob(store *gitstore.Store, owner, repoName, ref string) *ArchiveJob {
	return &ArchiveJob{
		Store:    store,
		Owner:    owner,
		RepoName: repoName,
		Ref:      ref,
		done:     make(chan struct{}),
	}
}

func (j *ArchiveJob) Name() string {
	return fmt.Sprintf("archive/%s/%s@%s", j.Owner, j.RepoName, j.Ref)
}

func (j *ArchiveJob) Execute(ctx context.Context) error {
	defer close(j.done)

	reader, err := j.Store.Archive(j.Owner, j.RepoName, j.Ref)
	if err != nil {
		j.err = fmt.Errorf("git archive: %w", err)
		return j.err
	}
	defer reader.Close()

	// Write to a temp file
	tmpDir := os.TempDir()
	outPath := filepath.Join(tmpDir, fmt.Sprintf("%s-%s-%d.tar.gz",
		j.RepoName, sanitizeRef(j.Ref), time.Now().UnixNano()))

	f, err := os.Create(outPath)
	if err != nil {
		j.err = fmt.Errorf("create temp file: %w", err)
		return j.err
	}
	defer f.Close()

	written, err := io.Copy(f, reader)
	if err != nil {
		os.Remove(outPath)
		j.err = fmt.Errorf("write archive: %w", err)
		return j.err
	}

	log.Printf("[jobqueue] archive %s/%s@%s: wrote %d bytes to %s", j.Owner, j.RepoName, j.Ref, written, outPath)
	j.Result = outPath
	return nil
}

// Wait blocks until the job completes and returns the result path.
func (j *ArchiveJob) Wait() (string, error) {
	<-j.done
	return j.Result, j.err
}

// MergeJob performs a git merge in the background.
type MergeJob struct {
	Store      *gitstore.Store
	Owner      string
	RepoName   string
	BaseBranch string
	HeadBranch string
	Squash     bool
	SHA        string // populated after execution
	err        error
	done       chan struct{}
}

// NewMergeJob creates a new merge job.
func NewMergeJob(store *gitstore.Store, owner, repoName, baseBranch, headBranch string, squash bool) *MergeJob {
	return &MergeJob{
		Store:      store,
		Owner:      owner,
		RepoName:   repoName,
		BaseBranch: baseBranch,
		HeadBranch: headBranch,
		Squash:     squash,
		done:       make(chan struct{}),
	}
}

func (j *MergeJob) Name() string {
	return fmt.Sprintf("merge/%s/%s:%s→%s", j.Owner, j.RepoName, j.HeadBranch, j.BaseBranch)
}

func (j *MergeJob) Execute(ctx context.Context) error {
	defer close(j.done)

	sha, err := j.Store.Merge(j.Owner, j.RepoName, j.BaseBranch, j.HeadBranch, j.Squash)
	if err != nil {
		j.err = err
		return err
	}
	j.SHA = sha
	return nil
}

// Wait blocks until the merge completes and returns the resulting SHA.
func (j *MergeJob) Wait() (string, error) {
	<-j.done
	return j.SHA, j.err
}

func sanitizeRef(ref string) string {
	result := make([]byte, 0, len(ref))
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			result = append(result, c)
		} else {
			result = append(result, '_')
		}
	}
	return string(result)
}
