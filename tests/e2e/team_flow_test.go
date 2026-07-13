//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestTeamFlow_OrgRepoPermission(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "tow" + short
	member := "twm" + short
	orgName := "teame" + short

	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	memberToken := testutil.RegisterAndLogin(t, base, member)

	// ---- Create org ----
	t.Run("create org", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs", ownerToken, map[string]string{
			"name": orgName, "display_name": "Team E2E", "description": "test",
		})
		requireStatus(t, resp, http.StatusOK, "create org")
		if out["name"] != orgName {
			t.Fatalf("expected org name %q, got %q", orgName, out["name"])
		}
	})

	// ---- Create public repo in org ----
	t.Run("create public org repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/repos", ownerToken, map[string]any{
			"name": "team-app", "description": "team app", "private": false,
		})
		requireStatus(t, resp, http.StatusOK, "create public org repo")
		if out["full_name"] != orgName+"/team-app" {
			t.Fatalf("expected full_name %q, got %q", orgName+"/team-app", out["full_name"])
		}
	})

	// ---- Create private repo in org ----
	t.Run("create private org repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/repos", ownerToken, map[string]any{
			"name": "private-app", "description": "private team app", "private": true,
		})
		requireStatus(t, resp, http.StatusOK, "create private org repo")
		if out["full_name"] != orgName+"/private-app" {
			t.Fatalf("expected full_name %q, got %q", orgName+"/private-app", out["full_name"])
		}
	})

	// ---- Add member to org ----
	memberID := userID(t, base, memberToken)

	t.Run("add org member", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/members", ownerToken, map[string]any{
			"user_id": memberID, "role": "member",
		})
		requireStatus(t, resp, http.StatusOK, "add org member")
		if out["status"] != "added" {
			t.Fatalf("expected status 'added', got %q", out["status"])
		}
	})

	// ---- Create team ----
	t.Run("create team", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams", ownerToken, map[string]string{
			"name": "devs", "description": "developers",
		})
		requireStatus(t, resp, http.StatusOK, "create team")
		if out["name"] != "devs" {
			t.Fatalf("expected team name 'devs', got %q", out["name"])
		}
	})

	// ---- Add member to team ----
	t.Run("add team member", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams/devs/members", ownerToken, map[string]any{
			"user_id": memberID,
		})
		requireStatus(t, resp, http.StatusOK, "add team member")
		if out["status"] != "added" {
			t.Fatalf("expected status 'added', got %q", out["status"])
		}
	})

	// ---- Team member can access public org repo ----
	t.Run("team member can access public org repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+orgName+"/team-app", memberToken, nil)
		requireStatus(t, resp, http.StatusOK, "team member get public repo")
		if out["name"] != "team-app" {
			t.Fatalf("expected repo name 'team-app', got %q", out["name"])
		}
	})

	// ---- Team member can access private org repo (as org member) ----
	t.Run("team member can access private org repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+orgName+"/private-app", memberToken, nil)
		requireStatus(t, resp, http.StatusOK, "team member get private repo")
		if out["name"] != "private-app" {
			t.Fatalf("expected repo name 'private-app', got %q", out["name"])
		}
	})

	// ---- Non-member cannot access private org repo ----
	t.Run("non-member cannot access private org repo", func(t *testing.T) {
		outsider := "out" + uniqueSuffix()[len(uniqueSuffix())-6:]
		outsiderToken := testutil.RegisterAndLogin(t, base, outsider)
		resp, _ := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+orgName+"/private-app", outsiderToken, nil)
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 404/403 for non-member accessing private org repo, got %d", resp.StatusCode)
		}
	})

	// ---- Non-member CAN access public org repo ----
	t.Run("non-member can access public org repo", func(t *testing.T) {
		viewer := "viw" + uniqueSuffix()[len(uniqueSuffix())-6:]
		viewerToken := testutil.RegisterAndLogin(t, base, viewer)
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+orgName+"/team-app", viewerToken, nil)
		requireStatus(t, resp, http.StatusOK, "non-member get public repo")
		if out["name"] != "team-app" {
			t.Fatalf("expected repo name 'team-app', got %q", out["name"])
		}
	})

	// ---- Owner can list orgs ----
	t.Run("owner can list orgs", func(t *testing.T) {
		resp, orgs := doJSONArray(t, http.MethodGet, base+"/api/v1/orgs", ownerToken, nil)
		requireStatus(t, resp, http.StatusOK, "list orgs")
		found := false
		for _, o := range orgs {
			if n, _ := o["name"].(string); n == orgName {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("org %q not found in list", orgName)
		}
	})
}

func TestTeamFlow_DuplicateTeamName(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "tdn" + short
	orgName := "teamdup" + short

	ownerToken := testutil.RegisterAndLogin(t, base, owner)

	// Create org
	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs", ownerToken, map[string]string{
		"name": orgName, "display_name": "Team Dup", "description": "",
	})
	requireStatus(t, resp, http.StatusOK, "create org")

	// Create first team
	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams", ownerToken, map[string]string{
		"name": "duplicate", "description": "first",
	})
	requireStatus(t, resp, http.StatusOK, "create first team")

	// Create duplicate team name in same org should fail
	t.Run("duplicate team name in same org", func(t *testing.T) {
		resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams", ownerToken, map[string]string{
			"name": "duplicate", "description": "second",
		})
		if resp.StatusCode != http.StatusInternalServerError && resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected error for duplicate team name, got %d", resp.StatusCode)
		}
	})
}

func TestTeamFlow_MultipleTeamsMultipleMembers(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "tmt" + short
	member1 := "tm1" + short
	member2 := "tm2" + short
	orgName := "teammt" + short

	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	member1Token := testutil.RegisterAndLogin(t, base, member1)
	member2Token := testutil.RegisterAndLogin(t, base, member2)
	member1ID := userID(t, base, member1Token)
	member2ID := userID(t, base, member2Token)

	// Create org
	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs", ownerToken, map[string]string{
		"name": orgName, "display_name": "Multi Team", "description": "",
	})
	requireStatus(t, resp, http.StatusOK, "create org")

	// Add both members to org
	for _, mid := range []string{member1ID, member2ID} {
		resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/members", ownerToken, map[string]any{
			"user_id": mid, "role": "member",
		})
		requireStatus(t, resp, http.StatusOK, "add org member")
	}

	// Create two teams
	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams", ownerToken, map[string]string{
		"name": "alpha", "description": "Alpha team",
	})
	requireStatus(t, resp, http.StatusOK, "create alpha team")

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams", ownerToken, map[string]string{
		"name": "beta", "description": "Beta team",
	})
	requireStatus(t, resp, http.StatusOK, "create beta team")

	// Add member1 to alpha, member2 to beta
	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams/alpha/members", ownerToken, map[string]any{
		"user_id": member1ID,
	})
	requireStatus(t, resp, http.StatusOK, "add member1 to alpha")

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams/beta/members", ownerToken, map[string]any{
		"user_id": member2ID,
	})
	requireStatus(t, resp, http.StatusOK, "add member2 to beta")

	// Create repos
	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/repos", ownerToken, map[string]any{
		"name": "alpha-repo", "private": false,
	})
	requireStatus(t, resp, http.StatusOK, "create alpha repo")

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/repos", ownerToken, map[string]any{
		"name": "beta-repo", "private": false,
	})
	requireStatus(t, resp, http.StatusOK, "create beta repo")

	// Both members can access both repos (org members have read access)
	t.Run("member1 can access alpha-repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+orgName+"/alpha-repo", member1Token, nil)
		requireStatus(t, resp, http.StatusOK, "member1 get alpha-repo")
		if out["name"] != "alpha-repo" {
			t.Fatalf("expected alpha-repo, got %q", out["name"])
		}
	})

	t.Run("member2 can access beta-repo", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+orgName+"/beta-repo", member2Token, nil)
		requireStatus(t, resp, http.StatusOK, "member2 get beta-repo")
		if out["name"] != "beta-repo" {
			t.Fatalf("expected beta-repo, got %q", out["name"])
		}
	})

	// List orgs for member1
	t.Run("list orgs for member1", func(t *testing.T) {
		// Use raw request since the response is an array
		resp, orgs := doJSONArray(t, http.MethodGet, base+"/api/v1/orgs", member1Token, nil)
		requireStatus(t, resp, http.StatusOK, "list orgs for member1")
		if len(orgs) == 0 {
			t.Fatal("expected at least one org in list for member1")
		}
	})

	// List orgs for member2
	t.Run("list orgs for member2", func(t *testing.T) {
		resp, orgs := doJSONArray(t, http.MethodGet, base+"/api/v1/orgs", member2Token, nil)
		requireStatus(t, resp, http.StatusOK, "list orgs for member2")
		if len(orgs) == 0 {
			t.Fatal("expected at least one org in list for member2")
		}
	})
}

func TestTeamFlow_AddMemberTwice(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	short := suffix[len(suffix)-6:]
	owner := "ta2" + short
	member := "tam2" + short
	orgName := "teamx2" + short

	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	memberToken := testutil.RegisterAndLogin(t, base, member)
	memberID := userID(t, base, memberToken)

	// Create org and team
	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs", ownerToken, map[string]string{
		"name": orgName, "display_name": "X2", "description": "",
	})
	requireStatus(t, resp, http.StatusOK, "create org")

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams", ownerToken, map[string]string{
		"name": "core", "description": "core",
	})
	requireStatus(t, resp, http.StatusOK, "create team")

	// Add member to team first time
	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams/core/members", ownerToken, map[string]any{
		"user_id": memberID,
	})
	requireStatus(t, resp, http.StatusOK, "first add team member")
	if out["status"] != "added" {
		t.Fatalf("expected 'added', got %q", out["status"])
	}

	// Add same member again — should be idempotent
	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams/core/members", ownerToken, map[string]any{
		"user_id": memberID,
	})
	requireStatus(t, resp, http.StatusOK, "second add team member (idempotent)")
	if out["status"] != "added" {
		t.Fatalf("expected 'added', got %q", out["status"])
	}
}
