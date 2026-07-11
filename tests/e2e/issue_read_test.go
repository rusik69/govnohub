//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestGetIssue(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "iget" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues", token, map[string]string{
		"title": "bug report", "body": "details here",
	})
	requireStatus(t, resp, http.StatusOK, "create issue")
	num := int(out["number"].(float64))

	resp, got := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/issues/"+itoa(num), token, nil)
	requireStatus(t, resp, http.StatusOK, "get issue")
	if got["title"] != "bug report" {
		t.Fatalf("unexpected title: %v", got["title"])
	}
}

func TestListIssueComments(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "icmt" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues", token, map[string]string{
		"title": "discuss", "body": "",
	})
	requireStatus(t, resp, http.StatusOK, "create issue")
	num := int(out["number"].(float64))

	for _, body := range []string{"first", "second"} {
		resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues/"+itoa(num)+"/comments", token, map[string]string{
			"body": body,
		})
		requireStatus(t, resp, http.StatusOK, "add comment")
	}

	resp, comments := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/issues/"+itoa(num)+"/comments", token, nil)
	requireStatus(t, resp, http.StatusOK, "list comments")
	assertJSONArrayLen(t, comments, 2, "issue comments")
}

func TestListLabels(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "ilbl" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/labels", token, map[string]string{
		"name": "enhancement", "color": "00ff00",
	})
	requireStatus(t, resp, http.StatusOK, "create label")

	resp, labels := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/labels", token, nil)
	requireStatus(t, resp, http.StatusOK, "list labels")
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(labels))
	}
}

func TestRemoveIssueLabel(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "irmv" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues", token, map[string]string{
		"title": "labeled", "body": "",
	})
	requireStatus(t, resp, http.StatusOK, "create issue")
	num := int(out["number"].(float64))

	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/labels", token, map[string]string{
		"name": "bug", "color": "ff0000",
	})
	requireStatus(t, resp, http.StatusOK, "create label")
	labelID, _ := out["id"].(string)

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues/"+itoa(num)+"/labels/"+labelID, token, nil)
	requireStatus(t, resp, http.StatusOK, "add label")

	resp, _ = testutil.DoJSON(t, http.MethodDelete, base+"/api/v1/repos/"+repo+"/issues/"+itoa(num)+"/labels/"+labelID, token, nil)
	requireStatus(t, resp, http.StatusOK, "remove label")
}

func TestPatchIssueAssignee(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	owner := "ipat" + suffix[len(suffix)-6:]
	assignee := "ipata" + suffix[len(suffix)-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	testutil.RegisterAndLogin(t, base, assignee)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues", token, map[string]string{
		"title": "assign me", "body": "",
	})
	requireStatus(t, resp, http.StatusOK, "create issue")
	num := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPatch, base+"/api/v1/repos/"+repo+"/issues/"+itoa(num), token, map[string]string{
		"assignee": assignee,
	})
	requireStatus(t, resp, http.StatusOK, "patch issue assignee")

	resp, got := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/issues/"+itoa(num), token, nil)
	requireStatus(t, resp, http.StatusOK, "get issue")
	if got["assignee"] != assignee {
		t.Fatalf("unexpected assignee: %v", got["assignee"])
	}
}
