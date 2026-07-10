package main

import (
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestParseGitSSHCommand(t *testing.T) {
	tests := []struct {
		cmd            string
		wantService    string
		wantRepo       string
		wantStateless  bool
		wantOK         bool
	}{
		{"git-upload-pack 'alice/app.git'", "upload-pack", "alice/app", false, true},
		{"git-upload-pack '/alice/app.git'", "upload-pack", "alice/app", false, true},
		{"git-upload-pack -p --stateless-rpc 'alice/app'", "upload-pack", "alice/app", true, true},
		{"git-receive-pack 'bob/r.git'", "receive-pack", "bob/r", false, true},
		{"git-shell -c upload-pack", "", "", false, false},
	}
	for _, tc := range tests {
		service, repo, stateless, ok := parseGitSSHCommand(tc.cmd)
		if ok != tc.wantOK {
			t.Fatalf("%q: ok=%v want %v", tc.cmd, ok, tc.wantOK)
		}
		if !tc.wantOK {
			continue
		}
		if service != tc.wantService || repo != tc.wantRepo || stateless != tc.wantStateless {
			t.Fatalf("%q: got %s %s stateless=%v want %s %s stateless=%v",
				tc.cmd, service, repo, stateless, tc.wantService, tc.wantRepo, tc.wantStateless)
		}
	}
}

func TestParseExecCommand(t *testing.T) {
	payload := ssh.Marshal(struct {
		Command string
	}{Command: `git-upload-pack '/alice/app.git'`})
	if got := parseExecCommand(payload); got != `git-upload-pack '/alice/app.git'` {
		t.Fatalf("got %q", got)
	}
}

func TestParseEnvRequest(t *testing.T) {
	payload := ssh.Marshal(struct {
		Name  string
		Value string
	}{Name: "GIT_PROTOCOL", Value: "version=2"})
	var envReq struct {
		Name  string
		Value string
	}
	if err := ssh.Unmarshal(payload, &envReq); err != nil {
		t.Fatal(err)
	}
	if kv, ok := acceptSSHEnv(envReq.Name, envReq.Value); !ok || kv != "GIT_PROTOCOL=version=2" {
		t.Fatalf("kv=%q ok=%v", kv, ok)
	}
}

func TestAcceptSSHEnv(t *testing.T) {
	if kv, ok := acceptSSHEnv("GIT_PROTOCOL", "version=2"); !ok || kv != "GIT_PROTOCOL=version=2" {
		t.Fatalf("GIT_PROTOCOL: kv=%q ok=%v", kv, ok)
	}
	if _, ok := acceptSSHEnv("PATH", "/usr/bin"); ok {
		t.Fatal("expected PATH to be rejected")
	}
}

func TestSSHPackEnv(t *testing.T) {
	got := sshPackEnv(nil)
	if len(got) != 1 || got[0] != "GIT_PROTOCOL=version=2" {
		t.Fatalf("default env=%v", got)
	}
	got = sshPackEnv([]string{"GIT_PROTOCOL=version=1"})
	if len(got) != 1 || got[0] != "GIT_PROTOCOL=version=1" {
		t.Fatalf("preserved env=%v", got)
	}
}
