//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestIssueCommentsLabelsMilestones(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "iss" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues", token, map[string]string{
		"title": "bug", "body": "details",
	})
	requireStatus(t, resp, http.StatusOK, "create issue")
	issueNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues/"+itoa(issueNum)+"/comments", token, map[string]string{
		"body": "first comment",
	})
	requireStatus(t, resp, http.StatusOK, "add comment")

	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/labels", token, map[string]string{
		"name": "bug", "color": "ff0000",
	})
	requireStatus(t, resp, http.StatusOK, "create label")
	labelID, _ := out["id"].(string)

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues/"+itoa(issueNum)+"/labels/"+labelID, token, nil)
	requireStatus(t, resp, http.StatusOK, "add label")

	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/milestones", token, map[string]string{
		"title": "v1", "description": "first release",
	})
	requireStatus(t, resp, http.StatusOK, "create milestone")
	milestoneID, _ := out["id"].(string)

	resp, _ = testutil.DoJSON(t, http.MethodPatch, base+"/api/v1/repos/"+repo+"/issues/"+itoa(issueNum), token, map[string]string{
		"milestone_id": milestoneID,
	})
	requireStatus(t, resp, http.StatusOK, "patch issue milestone")

	resp, issues := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/issues", token, nil)
	requireStatus(t, resp, http.StatusOK, "list issues")
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues/"+itoa(issueNum)+"/close", token, nil)
	requireStatus(t, resp, http.StatusOK, "close issue")
}

func TestMilestoneClose(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "ms" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/milestones", token, map[string]string{
		"title": "sprint", "description": "Q1",
	})
	requireStatus(t, resp, http.StatusOK, "create milestone")
	milestoneID, _ := out["id"].(string)

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/milestones/"+milestoneID+"/close", token, nil)
	requireStatus(t, resp, http.StatusOK, "close milestone")

	resp, milestones := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/milestones", token, nil)
	requireStatus(t, resp, http.StatusOK, "list milestones")
	if len(milestones) != 1 {
		t.Fatalf("expected 1 milestone, got %d", len(milestones))
	}
	if milestones[0]["state"] != "closed" {
		t.Fatalf("expected closed milestone, got %v", milestones[0]["state"])
	}
}
