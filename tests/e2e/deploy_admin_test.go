//go:build deploy

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestAdminAuditLog(t *testing.T) {
	env := testenv.NewLocalDeploy(t)
	defer env.Cleanup()
	base := env.URL

	adminToken := testutil.Login(t, base, "admin", "admin")
	resp, _ := doJSONArray(t, http.MethodGet, base+"/api/v1/admin/audit", adminToken, nil)
	requireStatus(t, resp, http.StatusOK, "admin audit log")
}

func TestDeployAdminUserList(t *testing.T) {
	env := testenv.NewLocalDeploy(t)
	defer env.Cleanup()
	base := env.URL

	adminToken := testutil.Login(t, base, "admin", "admin")
	resp, users := doJSONArray(t, http.MethodGet, base+"/api/v1/admin/users", adminToken, nil)
	requireStatus(t, resp, http.StatusOK, "admin list users")
	if len(users) == 0 {
		t.Fatal("expected users")
	}
}
