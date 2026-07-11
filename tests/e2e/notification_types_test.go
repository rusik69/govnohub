//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestPRReviewNotification(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	owner := "npr" + suffix[len(suffix)-6:]
	reviewer := "nprr" + suffix[len(suffix)-6:]
	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	reviewerToken := testutil.RegisterAndLogin(t, base, reviewer)
	createRepo(t, base, ownerToken, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/branches", ownerToken, map[string]string{
		"name": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create branch")

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls", ownerToken, map[string]string{
		"title": "notify PR", "body": "", "head": "feature", "base": "main",
	})
	requireStatus(t, resp, http.StatusOK, "create PR")
	prNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/pulls/"+itoa(prNum)+"/reviews", reviewerToken, map[string]string{
		"state": "approved", "body": "looks good",
	})
	requireStatus(t, resp, http.StatusOK, "add review")

	items := waitForNotifications(t, base, ownerToken)
	if title, _ := items[0]["title"].(string); title != "PR review" {
		t.Fatalf("unexpected notification title: %v", title)
	}
}

func TestIssueCloseNotification(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	owner := "nic" + suffix[len(suffix)-6:]
	closer := "nicc" + suffix[len(suffix)-6:]
	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	closerToken := testutil.RegisterAndLogin(t, base, closer)
	createRepo(t, base, ownerToken, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues", ownerToken, map[string]string{
		"title": "please close", "body": "",
	})
	requireStatus(t, resp, http.StatusOK, "create issue")
	issueNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues/"+itoa(issueNum)+"/close", closerToken, nil)
	requireStatus(t, resp, http.StatusOK, "close issue")

	items := waitForNotifications(t, base, ownerToken)
	if title, _ := items[0]["title"].(string); title != "Issue closed" {
		t.Fatalf("unexpected notification title: %v", title)
	}
}
