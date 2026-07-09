//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestHealthz(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	resp, err := http.Get(env.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestRegisterLoginMe(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	token := testutil.RegisterAndLogin(t, env.URL, "intuser")
	resp, out := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me status=%d", resp.StatusCode)
	}
	if out["username"] != "intuser" {
		t.Fatalf("user=%v", out)
	}
}

func TestCreateRepoAndIssue(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	token := testutil.RegisterAndLogin(t, env.URL, "repouser")
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/repouser/repos", token, map[string]any{
		"name": "myrepo", "description": "test", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/repouser/myrepo/issues", token, map[string]string{
		"title": "bug", "body": "found",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create issue status=%d", resp.StatusCode)
	}
}
