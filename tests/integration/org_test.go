//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestCreateOrg(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "orgcreator")

	// Create an org
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs", token, map[string]string{
		"name":         "myorg",
		"display_name": "My Org",
		"description":  "A test organization",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create org status=%d out=%v", resp.StatusCode, out)
	}
	if out["name"] != "myorg" {
		t.Errorf("name=%q want myorg", out["name"])
	}
	if out["display_name"] != "My Org" {
		t.Errorf("display_name=%q want My Org", out["display_name"])
	}
	if out["description"] != "A test organization" {
		t.Errorf("description=%q want A test organization", out["description"])
	}
	if _, ok := out["id"]; !ok {
		t.Fatal("missing id in org response")
	}
}

func TestListOrgs(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "orglistuser")

	// Create two orgs
	for _, name := range []string{"org-alpha", "org-beta"} {
		resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs", token, map[string]string{
			"name": name, "display_name": name, "description": "",
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("create org %s: %d", name, resp.StatusCode)
		}
	}

	// List orgs — response is a JSON array, use raw request
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list orgs status=%d", resp.StatusCode)
	}
	var orgs []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil {
		t.Fatal(err)
	}
	if len(orgs) < 2 {
		t.Fatalf("expected at least 2 orgs, got %d", len(orgs))
	}
	names := make(map[string]bool)
	for _, o := range orgs {
		n, _ := o["name"].(string)
		names[n] = true
	}
	if !names["org-alpha"] {
		t.Error("missing org-alpha in list")
	}
	if !names["org-beta"] {
		t.Error("missing org-beta in list")
	}
}

func TestAddOrgMember(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")
	orgOwner := "orgowner"
	orgMember := "orgmember"

	ownerToken := testutil.RegisterAndLogin(t, env.URL, orgOwner)
	memberToken := testutil.RegisterAndLogin(t, env.URL, orgMember)

	// Create an org
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs", ownerToken, map[string]string{
		"name": "teamorg", "display_name": "Team Org", "description": "",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create org status=%d out=%v", resp.StatusCode, out)
	}
	orgID, _ := out["id"].(string)
	if orgID == "" {
		t.Fatal("missing org id")
	}

	// Add a member to the org — need the member's user ID
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user", memberToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get member user info: %d", resp.StatusCode)
	}
	memberUserID, _ := out["id"].(string)
	if memberUserID == "" {
		t.Fatal("missing member user id")
	}

	// Add member via API
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs/teamorg/members", ownerToken, map[string]any{
		"user_id": memberUserID,
		"role":    "member",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add member status=%d out=%v", resp.StatusCode, out)
	}
	if out["status"] != "added" {
		t.Errorf("status=%q want added", out["status"])
	}

	// Add duplicate member should be idempotent
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs/teamorg/members", ownerToken, map[string]any{
		"user_id": memberUserID,
		"role":    "member",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add duplicate member status=%d out=%v", resp.StatusCode, out)
	}
	if out["status"] != "added" {
		t.Errorf("status=%q want added", out["status"])
	}

	// Non-admin adding to org should fail — need admin for that
	_ = adminToken // unused in this test, but kept for reference
}

func TestCreateTeam(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "teamcreator")

	// Create an org first
	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs", token, map[string]string{
		"name": "teamorg", "display_name": "Team Org", "description": "",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create org: %d", resp.StatusCode)
	}

	// Create a team in the org
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs/teamorg/teams", token, map[string]string{
		"name": "engineers", "description": "Engineering team",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create team status=%d out=%v", resp.StatusCode, out)
	}
	if out["name"] != "engineers" {
		t.Errorf("team name=%q want engineers", out["name"])
	}
	if out["description"] != "Engineering team" {
		t.Errorf("description=%q want Engineering team", out["description"])
	}
	if _, ok := out["id"]; !ok {
		t.Fatal("missing id in team response")
	}
}

func TestAddTeamMember(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	orgOwner := "tmowner"
	memberUser := "tmmember"
	ownerToken := testutil.RegisterAndLogin(t, env.URL, orgOwner)
	memberToken := testutil.RegisterAndLogin(t, env.URL, memberUser)

	// Create an org
	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs", ownerToken, map[string]string{
		"name": "teammgr", "display_name": "Team Mgr", "description": "",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create org: %d", resp.StatusCode)
	}

	// Create a team
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs/teammgr/teams", ownerToken, map[string]string{
		"name": "dev", "description": "Devs",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create team: %d", resp.StatusCode)
	}

	// Get member's user ID
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user", memberToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get member info: %d", resp.StatusCode)
	}
	memberID, _ := out["id"].(string)
	if memberID == "" {
		t.Fatal("missing member id")
	}

	// Add member to the team
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs/teammgr/teams/dev/members", ownerToken, map[string]any{
		"user_id": memberID,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add team member status=%d out=%v", resp.StatusCode, out)
	}
	if out["status"] != "added" {
		t.Errorf("status=%q want added", out["status"])
	}

	// Add duplicate team member should be idempotent
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs/teammgr/teams/dev/members", ownerToken, map[string]any{
		"user_id": memberID,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add duplicate team member status=%d out=%v", resp.StatusCode, out)
	}
	if out["status"] != "added" {
		t.Errorf("status=%q want added", out["status"])
	}
}

func TestCreateOrgAndCreateRepo(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token := testutil.RegisterAndLogin(t, env.URL, "orgrepouser")

	// Create an org
	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs", token, map[string]string{
		"name": "myorg", "display_name": "My Org", "description": "",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create org: %d", resp.StatusCode)
	}

	// Create a repo in the org
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs/myorg/repos", token, map[string]any{
		"name": "org-repo", "description": "org repo", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create org repo status=%d out=%v", resp.StatusCode, out)
	}
	fullName, _ := out["full_name"].(string)
	if fullName != "myorg/org-repo" {
		t.Errorf("full_name=%q want myorg/org-repo", fullName)
	}
	if owner, _ := out["owner"].(string); owner != "myorg" {
		t.Errorf("owner=%q want myorg", owner)
	}

	// Get the repo and verify it's accessible
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/myorg/org-repo", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get org repo status=%d out=%v", resp.StatusCode, out)
	}
	if out["name"] != "org-repo" {
		t.Errorf("repo name=%q", out["name"])
	}
}

func TestOrgLifecycle(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")
	aliceToken := testutil.RegisterAndLogin(t, env.URL, "alice")
	bobToken := testutil.RegisterAndLogin(t, env.URL, "bob")

	// Alice creates an org
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs", aliceToken, map[string]string{
		"name": "lifecycle", "display_name": "Lifecycle Org", "description": "Test lifecycle",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create org: %d", resp.StatusCode)
	}

	// Get Bob's user ID
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user", bobToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get bob: %d", resp.StatusCode)
	}
	bobID, _ := out["id"].(string)

	// Alice adds Bob as a member
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs/lifecycle/members", aliceToken, map[string]any{
		"user_id": bobID,
		"role":    "member",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add bob as member: %d out=%v", resp.StatusCode, out)
	}

	// Create a team
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs/lifecycle/teams", aliceToken, map[string]string{
		"name": "engineers", "description": "Engineering",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create team: %d", resp.StatusCode)
	}

	// Add Bob to the team
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs/lifecycle/teams/engineers/members", aliceToken, map[string]any{
		"user_id": bobID,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add bob to team: %d out=%v", resp.StatusCode, out)
	}

	// Verify the org appears in the list
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/orgs", nil)
	req.Header.Set("Authorization", "Bearer "+aliceToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list orgs: %d", resp.StatusCode)
	}
	var orgs []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range orgs {
		if n, _ := o["name"].(string); n == "lifecycle" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("org 'lifecycle' not found in list")
	}

	// Admin can also add org repo
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/orgs/lifecycle/repos", adminToken, map[string]any{
		"name": "main-repo", "description": "Main org repo", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin create org repo status=%d out=%v", resp.StatusCode, out)
	}
	fullName, _ := out["full_name"].(string)
	if fullName != "lifecycle/main-repo" {
		t.Errorf("full_name=%q want lifecycle/main-repo", fullName)
	}
}
