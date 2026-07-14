package gitstore

import (
	"bytes"
	"context"
	"os"
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

func TestParseRefUpdates(t *testing.T) {
	oldSHA := "0000000000000000000000000000000000000000"
	newSHA := "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	ref := "refs/heads/main"
	// Build a pkt-line: 4 hex chars for length + "<old> <new> <ref>\n"
	// Length = len("<old> <new> <ref>\n") + 4
	line := oldSHA + " " + newSHA + " " + ref + "\n"
	pktLine := formatPktLine(line)
	flush := []byte("0000")

	data := append(pktLine, flush...)

	refs, err := ParseRefUpdates(data)
	if err != nil {
		t.Fatalf("ParseRefUpdates error: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].OldSHA != oldSHA {
		t.Fatalf("old SHA: got %q, want %q", refs[0].OldSHA, oldSHA)
	}
	if refs[0].NewSHA != newSHA {
		t.Fatalf("new SHA: got %q, want %q", refs[0].NewSHA, newSHA)
	}
	if refs[0].Ref != ref {
		t.Fatalf("ref: got %q, want %q", refs[0].Ref, ref)
	}
}

func TestParseRefUpdatesMultiple(t *testing.T) {
	old := "0000000000000000000000000000000000000000"
	n1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	n2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	line1 := old + " " + n1 + " refs/heads/main\n"
	line2 := old + " " + n2 + " refs/heads/feature\n"
	pkt1 := formatPktLine(line1)
	pkt2 := formatPktLine(line2)
	flush := []byte("0000")

	data := append(append(pkt1, pkt2...), flush...)

	refs, err := ParseRefUpdates(data)
	if err != nil {
		t.Fatalf("ParseRefUpdates error: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[1].Ref != "refs/heads/feature" {
		t.Fatalf("second ref: got %q, want refs/heads/feature", refs[1].Ref)
	}
}

func TestParseRefUpdatesWithCapabilities(t *testing.T) {
	old := "0000000000000000000000000000000000000000"
	newSHA := "cccccccccccccccccccccccccccccccccccccccc"
	ref := "refs/heads/main"

	// Capabilities are after a null byte in the ref line
	line := old + " " + newSHA + " " + ref + "\x00report-status\n"
	pktLine := formatPktLine(line)
	flush := []byte("0000")

	data := append(pktLine, flush...)

	refs, err := ParseRefUpdates(data)
	if err != nil {
		t.Fatalf("ParseRefUpdates error: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].Ref != ref {
		t.Fatalf("expected ref %q, got %q", ref, refs[0].Ref)
	}
}

func TestParseRefUpdatesNoData(t *testing.T) {
	refs, err := ParseRefUpdates([]byte{})
	if err != nil {
		t.Fatalf("ParseRefUpdates error: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs for empty input, got %d", len(refs))
	}
}

func TestParseRefUpdatesInvalidSHA(t *testing.T) {
	// Short SHA (not 40 chars)
	line := "abc123 def456 refs/heads/main\n"
	pktLine := formatPktLine(line)
	data := append(pktLine, []byte("0000")...)

	refs, err := ParseRefUpdates(data)
	if err != nil {
		t.Fatalf("ParseRefUpdates error: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs for invalid SHA, got %d", len(refs))
	}
}

func TestInitInstallsPreReceiveHook(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background(), "alice", "hooked"); err != nil {
		t.Fatal(err)
	}

	hookPath := filepath.Join(store.RepoPath("alice", "hooked"), "hooks", "pre-receive")
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("pre-receive hook not installed: %v", err)
	}
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "protected") {
		t.Fatal("hook should mention 'protected'")
	}
}
