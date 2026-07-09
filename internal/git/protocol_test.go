package gitstore

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteServicePacket(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteServicePacket(&buf, "git-upload-pack"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "001e# service=git-upload-pack") {
		t.Fatalf("unexpected packet prefix: %q", out)
	}
	if !strings.HasSuffix(out, "0000") {
		t.Fatalf("missing flush packet: %q", out)
	}
}

func TestAdvertiseRefsEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background(), "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	repoPath := store.RepoPath("alice", "demo")
	var buf bytes.Buffer
	if err := WriteServicePacket(&buf, "git-upload-pack"); err != nil {
		t.Fatal(err)
	}
	if err := AdvertiseRefs(repoPath, "git-upload-pack", &buf); err != nil {
		t.Fatal(err)
	}
	if buf.Len() <= 8 {
		t.Fatal("expected ref advertisement output")
	}
}

func TestListBranchSHAs(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)
	store.Init(context.Background(), "bob", "app")
	repoPath := store.RepoPath("bob", "app")
	bareCommit(t, repoPath, "main", "init")

	refs, err := store.ListBranchSHAs("bob", "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 {
		t.Fatal("expected branch refs")
	}
	for branch, sha := range refs {
		if branch == "" || len(sha) != 40 {
			t.Fatalf("invalid ref %s=%s", branch, sha)
		}
	}
}

func TestRepoExists(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)
	if store.Exists("x", "y") {
		t.Fatal("expected missing repo")
	}
	store.Init(context.Background(), "x", "y")
	if !store.Exists("x", "y") {
		t.Fatal("expected repo to exist")
	}
}

func TestAdvertisementContentType(t *testing.T) {
	if got := AdvertisementContentType("git-upload-pack"); got != "application/x-git-upload-pack-advertisement" {
		t.Fatalf("got %s", got)
	}
	if got := AdvertisementContentType("git-receive-pack"); got != "application/x-git-receive-pack-advertisement" {
		t.Fatalf("got %s", got)
	}
}

func TestRepoPathJoin(t *testing.T) {
	store, _ := NewStore(t.TempDir())
	got := store.RepoPath("o", "r")
	if filepath.Base(got) != "r.git" {
		t.Fatalf("path=%s", got)
	}
}
