//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestIssueCommentNotifications(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	owner := "nota" + suffix[len(suffix)-6:]
	commenter := "notb" + suffix[len(suffix)-6:]
	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	commenterToken := testutil.RegisterAndLogin(t, base, commenter)
	createRepo(t, base, ownerToken, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues", ownerToken, map[string]string{
		"title": "needs review", "body": "please comment",
	})
	requireStatus(t, resp, http.StatusOK, "create issue")
	issueNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues/"+itoa(issueNum)+"/comments", commenterToken, map[string]string{
		"body": "on it",
	})
	requireStatus(t, resp, http.StatusOK, "add comment")

	items := waitForNotifications(t, base, ownerToken)
	notifID, _ := items[0]["id"].(string)
	if notifID == "" {
		t.Fatal("missing notification id")
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/notifications/"+notifID+"/read", ownerToken, nil)
	requireStatus(t, resp, http.StatusOK, "mark notification read")
}
