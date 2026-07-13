//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestCreateAndGetUserRepo(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "cruser")
	owner := "cruser"

	// Create repo
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "myrepo", "description": "test description", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d out=%v", resp.StatusCode, out)
	}
	name, _ := out["name"].(string)
	if name != "myrepo" {
		t.Fatalf("repo name=%q want myrepo", name)
	}
	desc, _ := out["description"].(string)
	if desc != "test description" {
		t.Fatalf("repo description=%q", desc)
	}
	fullName, _ := out["full_name"].(string)
	if fullName != "cruser/myrepo" {
		t.Fatalf("full_name=%q", fullName)
	}
	isPrivate, _ := out["is_private"].(bool)
	if isPrivate {
		t.Fatal("expected public repo")
	}
	if _, ok := out["id"]; !ok {
		t.Fatal("missing id")
	}

	// Get repo by full name
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/myrepo", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get repo status=%d out=%v", resp.StatusCode, out)
	}
	if out["name"] != "myrepo" {
		t.Fatalf("got name=%v", out["name"])
	}
}

func TestCreatePrivateRepoAccessControl(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	owner := "privowner"
	other := "privother"
	ownerToken := testutil.RegisterAndLogin(t, env.URL, owner)
	otherToken := testutil.RegisterAndLogin(t, env.URL, other)

	// Create private repo
	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", ownerToken, map[string]any{
		"name": "private-repo", "private": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	// Other user should be forbidden from accessing private repo
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/private-repo", otherToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("other user accessing private repo should be 403, got %d", resp.StatusCode)
	}

	// Owner can still access
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/private-repo", ownerToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner accessing private repo should be 200, got %d", resp.StatusCode)
	}
}

func TestPublicRepoAccessibleByAnyone(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	owner := "pubowner"
	other := "pubother"
	ownerToken := testutil.RegisterAndLogin(t, env.URL, owner)
	testutil.RegisterAndLogin(t, env.URL, other)

	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", ownerToken, map[string]any{
		"name": "public-repo", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	// Public repo doesn't require auth (but still need to be logged in for this endpoint)
	resp, out := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/public-repo", ownerToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get public repo status=%d", resp.StatusCode)
	}
	if out["name"] != "public-repo" {
		t.Fatalf("got name=%v", out["name"])
	}
}

func TestListBranchesDefaultMain(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "branchlistuser")
	owner := "branchlistuser"

	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "app", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	// Use raw request to decode array
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/app/branches", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list branches status=%d", resp.StatusCode)
	}
	var branches []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		t.Fatal(err)
	}
	if len(branches) == 0 {
		t.Fatal("expected at least default branch")
	}
	name, _ := branches[0]["name"].(string)
	if name != "main" {
		t.Fatalf("branch=%q want main", name)
	}
}

func TestStarAndUnstarRepo(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "staruser")
	owner := "staruser"

	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "shiny", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/shiny/star", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("star status=%d out=%v", resp.StatusCode, out)
	}
	if out["status"] != "starred" {
		t.Fatalf("got status=%q", out["status"])
	}

	resp, out = testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/repos/"+owner+"/shiny/star", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unstar status=%d", resp.StatusCode)
	}
	if out["status"] != "unstarred" {
		t.Fatalf("got status=%q", out["status"])
	}
}

func TestWatchAndUnwatchRepo(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "watchuser")
	owner := "watchuser"

	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "watched", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/watched/watch", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("watch status=%d", resp.StatusCode)
	}
	if out["status"] != "watching" {
		t.Fatalf("got status=%q", out["status"])
	}

	resp, out = testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/repos/"+owner+"/watched/watch", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unwatch status=%d", resp.StatusCode)
	}
	if out["status"] != "unwatched" {
		t.Fatalf("got status=%q", out["status"])
	}
}

func TestForkRepo(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	owner := "forkowner"
	other := "forkother"
	ownerToken := testutil.RegisterAndLogin(t, env.URL, owner)
	otherToken := testutil.RegisterAndLogin(t, env.URL, other)

	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", ownerToken, map[string]any{
		"name": "original", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/original/fork", otherToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fork status=%d out=%v", resp.StatusCode, out)
	}
	fullName, _ := out["full_name"].(string)
	if fullName != "forkother/original-fork" {
		t.Fatalf("fork full_name=%q", fullName)
	}
	isFork, _ := out["is_fork"].(bool)
	if !isFork {
		t.Fatal("expected fork repo to have is_fork=true")
	}
}

func TestListUserRepos(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "listrepouser")
	owner := "listrepouser"

	// Create multiple repos
	for _, name := range []string{"repo-a", "repo-b", "repo-c"} {
		resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
			"name": name, "private": false,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("create repo %s: %d", name, resp.StatusCode)
		}
	}

	// List user repos
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/user/repos", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list user repos status=%d", resp.StatusCode)
	}
	var repos []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		t.Fatal(err)
	}
	if len(repos) < 3 {
		t.Fatalf("expected at least 3 repos, got %d", len(repos))
	}
	names := map[string]bool{}
	for _, r := range repos {
		n, _ := r["name"].(string)
		names[n] = true
	}
	for _, n := range []string{"repo-a", "repo-b", "repo-c"} {
		if !names[n] {
			t.Fatalf("missing repo %q in user repo list", n)
		}
	}
}

func TestCreateBranchOnRepo(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "branchcreateuser")
	owner := "branchcreateuser"

	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "branchtest", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/repos/"+owner+"/branchtest/branches", token, map[string]string{
		"name": "feature-x", "base": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create branch status=%d out=%v", resp.StatusCode, out)
	}

	branchName, _ := out["branch"].(string)
	if branchName != "feature-x" {
		t.Fatalf("branch name=%q", branchName)
	}
	if sha, ok := out["sha"]; !ok || sha == "" {
		t.Fatal("missing sha in branch response")
	}
}

func TestCollaboratorAccessToPrivateRepo(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	owner := "collabowner"
	collab := "collabmember"
	other := "collabstranger"
	ownerToken := testutil.RegisterAndLogin(t, env.URL, owner)
	collabToken := testutil.RegisterAndLogin(t, env.URL, collab)
	otherToken := testutil.RegisterAndLogin(t, env.URL, other)

	// Create private repo
	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", ownerToken, map[string]any{
		"name": "team-project", "private": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	// Add collaborator with read permission
	resp, _ = testutil.DoJSON(t, http.MethodPut, env.URL+"/api/v1/repos/"+owner+"/team-project/collaborators/"+collab, ownerToken, map[string]string{
		"permission": "write",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add collaborator: %d", resp.StatusCode)
	}

	// Collaborator should have access
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/team-project", collabToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("collaborator access status=%d", resp.StatusCode)
	}

	// Stranger should be forbidden
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/team-project", otherToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("stranger access should be 403, got %d", resp.StatusCode)
	}

	// List collaborators — returns a JSON array directly
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/team-project/collaborators", nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list collaborators: %d", resp.StatusCode)
	}
	var collabList []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&collabList); err != nil {
		t.Fatal(err)
	}
	if len(collabList) < 1 {
		t.Fatal("expected at least 1 collaborator")
	}
	found := false
	for _, c := range collabList {
		if name, _ := c["username"].(string); name == collab {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected collaborator %q in list: %v", collab, collabList)
	}
}
