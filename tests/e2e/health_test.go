//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestHealthz(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	resp, err := http.Get(env.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	requireStatus(t, resp, http.StatusOK, "healthz")
}

func TestCurrentUser(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "me" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)

	resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/user", token, nil)
	requireStatus(t, resp, http.StatusOK, "current user")
	if out["username"] != owner {
		t.Fatalf("unexpected username: %v", out["username"])
	}
}
