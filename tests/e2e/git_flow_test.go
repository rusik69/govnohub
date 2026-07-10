//go:build deploy

package e2e

import (
	"io"
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestGitPushUpdatesCommits(t *testing.T) {
	env := testenv.NewLocalDeploy(t)
	defer env.Cleanup()
	base := env.URL
	owner := "git" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	testGitPush(t, env.GitURL, owner, "app", owner, "password123")

	resp, commits := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/commits", token, nil)
	requireStatus(t, resp, http.StatusOK, "list commits")
	if len(commits) == 0 {
		t.Fatal("expected commits after push")
	}
	msg, _ := commits[0]["message"].(string)
	if msg != "e2e commit" {
		t.Fatalf("unexpected commit message: %q", msg)
	}
}

func TestGitPushContents(t *testing.T) {
	env := testenv.NewLocalDeploy(t)
	defer env.Cleanup()
	base := env.URL
	owner := "gitc" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	testGitPush(t, env.GitURL, owner, "app", owner, "password123")

	req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/repos/"+repo+"/contents/README.md", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusOK, "get contents")
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("expected content body")
	}
}
