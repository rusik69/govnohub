//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestListReleases(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "rel" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")
	repo := owner + "/app"

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/releases", token, map[string]any{
		"tag_name": "v1.0.0", "name": "v1", "body": "first release",
	})
	requireStatus(t, resp, http.StatusOK, "create release")

	resp, releases := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/releases", token, nil)
	requireStatus(t, resp, http.StatusOK, "list releases")
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
	if tag, _ := releases[0]["tag_name"].(string); tag != "v1.0.0" {
		t.Fatalf("unexpected tag: %v", tag)
	}
}
