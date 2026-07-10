package main

import "testing"

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

func TestAcceptSSHEnv(t *testing.T) {
	if kv, ok := acceptSSHEnv("GIT_PROTOCOL", "version=2"); !ok || kv != "GIT_PROTOCOL=version=2" {
		t.Fatalf("GIT_PROTOCOL: kv=%q ok=%v", kv, ok)
	}
	if _, ok := acceptSSHEnv("PATH", "/usr/bin"); ok {
		t.Fatal("expected PATH to be rejected")
	}
}
