package main

import "testing"

func TestParseGitSSHCommand(t *testing.T) {
	tests := []struct {
		cmd         string
		wantService string
		wantRepo    string
		wantOK      bool
	}{
		{"git-upload-pack 'alice/app.git'", "upload-pack", "alice/app", true},
		{"git-upload-pack '/alice/app.git'", "upload-pack", "alice/app", true},
		{"git-upload-pack -p --stateless-rpc 'alice/app'", "upload-pack", "alice/app", true},
		{"git-receive-pack 'bob/r.git'", "receive-pack", "bob/r", true},
		{"git-shell -c upload-pack", "", "", false},
	}
	for _, tc := range tests {
		service, repo, ok := parseGitSSHCommand(tc.cmd)
		if ok != tc.wantOK {
			t.Fatalf("%q: ok=%v want %v", tc.cmd, ok, tc.wantOK)
		}
		if !tc.wantOK {
			continue
		}
		if service != tc.wantService || repo != tc.wantRepo {
			t.Fatalf("%q: got %s %s want %s %s", tc.cmd, service, repo, tc.wantService, tc.wantRepo)
		}
	}
}
