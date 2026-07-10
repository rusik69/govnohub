//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestListBranches(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	token := testutil.RegisterAndLogin(t, env.URL, "branchuser")
	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/branchuser/repos", token, map[string]any{
		"name": "app", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo status=%d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/repos/branchuser/app/branches", nil)
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
		t.Fatal("expected default branch")
	}
	name, _ := branches[0]["name"].(string)
	if name != "main" {
		t.Fatalf("branch=%q want main", name)
	}
}
