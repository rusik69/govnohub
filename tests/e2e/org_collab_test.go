//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestOrgMembersAndTeams(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	owner := "orgown" + suffix[len(suffix)-6:]
	member := "orgmem" + suffix[len(suffix)-6:]
	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	memberToken := testutil.RegisterAndLogin(t, base, member)
	memberID := userID(t, base, memberToken)

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs", ownerToken, map[string]string{
		"name": "org" + suffix[len(suffix)-4:], "display_name": "E2E Org", "description": "test",
	})
	requireStatus(t, resp, http.StatusOK, "create org")
	orgName, _ := out["name"].(string)

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/members", ownerToken, map[string]string{
		"user_id": memberID, "role": "member",
	})
	requireStatus(t, resp, http.StatusOK, "add org member")

	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams", ownerToken, map[string]string{
		"name": "devs", "description": "developers",
	})
	requireStatus(t, resp, http.StatusOK, "create team")
	teamName, _ := out["name"].(string)

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/teams/"+teamName+"/members", ownerToken, map[string]string{
		"user_id": memberID,
	})
	requireStatus(t, resp, http.StatusOK, "add team member")

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/orgs/"+orgName+"/repos", ownerToken, map[string]any{
		"name": "team-app", "private": false,
	})
	requireStatus(t, resp, http.StatusOK, "create org repo")

	resp, orgs := doJSONArray(t, http.MethodGet, base+"/api/v1/orgs", ownerToken, nil)
	requireStatus(t, resp, http.StatusOK, "list orgs")
	if len(orgs) == 0 {
		t.Fatal("expected at least one org")
	}
}

func TestCollaboratorRemove(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "coll" + uniqueSuffix()[len(uniqueSuffix())-6:]
	other := "collb" + uniqueSuffix()[len(uniqueSuffix())-6:]
	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	testutil.RegisterAndLogin(t, base, other)
	createRepo(t, base, ownerToken, owner, "shared")
	repo := owner + "/shared"

	resp, _ := testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+repo+"/collaborators/"+other, ownerToken, map[string]string{
		"permission": "read",
	})
	requireStatus(t, resp, http.StatusOK, "add collaborator")

	resp, collabs := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/collaborators", ownerToken, nil)
	requireStatus(t, resp, http.StatusOK, "list collaborators")
	if len(collabs) != 1 {
		t.Fatalf("expected 1 collaborator, got %d", len(collabs))
	}

	resp, _ = testutil.DoJSON(t, http.MethodDelete, base+"/api/v1/repos/"+repo+"/collaborators/"+other, ownerToken, nil)
	requireStatus(t, resp, http.StatusOK, "remove collaborator")

	resp, collabs = doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/collaborators", ownerToken, nil)
	requireStatus(t, resp, http.StatusOK, "list collaborators after remove")
	if len(collabs) != 0 {
		t.Fatalf("expected 0 collaborators, got %d", len(collabs))
	}
}
