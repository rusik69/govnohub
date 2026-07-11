//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestAdminUserCRUD(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	admin := adminToken(t, base)

	resp, users := doJSONArray(t, http.MethodGet, base+"/api/v1/admin/users", admin, nil)
	requireStatus(t, resp, http.StatusOK, "list users")
	if len(users) == 0 {
		t.Fatal("expected at least admin user")
	}

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/admin/users", admin, map[string]string{
		"username": "admnew",
		"email":    "admnew@test.local",
		"password": "secret123",
		"role":     "user",
	})
	requireStatus(t, resp, http.StatusOK, "create user")
	if out["username"] != "admnew" {
		t.Fatalf("unexpected user: %v", out)
	}
}

func TestAdminDeleteUser(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	admin := adminToken(t, base)

	testutil.AdminCreateUser(t, base, admin, "todelete")
	resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/user", testutil.Login(t, base, "todelete", "password123"), nil)
	requireStatus(t, resp, http.StatusOK, "login created user")
	targetID, _ := out["id"].(string)
	if targetID == "" {
		t.Fatal("missing user id")
	}

	req, err := http.NewRequest(http.MethodDelete, base+"/api/v1/admin/users/"+targetID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+admin)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete user: got %d", resp.StatusCode)
	}
}

func TestAdminRejectsNonAdmin(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	token := testutil.RegisterAndLogin(t, base, "notadmin")

	resp, _ := doJSONArray(t, http.MethodGet, base+"/api/v1/admin/users", token, nil)
	requireStatus(t, resp, http.StatusForbidden, "non-admin list")
}

func TestAdminRejectsPAT(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	admin := adminToken(t, base)

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/user/tokens", admin, map[string]any{
		"name": "admin-pat", "scopes": []string{"repo", "read:user"},
	})
	requireStatus(t, resp, http.StatusOK, "create PAT")
	pat, _ := out["token"].(string)

	resp, _ = doJSONArray(t, http.MethodGet, base+"/api/v1/admin/users", pat, nil)
	requireStatus(t, resp, http.StatusForbidden, "PAT admin access")
}

func TestAdminAuditLogE2E(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	admin := adminToken(t, base)

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/admin/users", admin, map[string]string{
		"username": "audituser",
		"email":    "audit@test.local",
		"password": "secret123",
	})
	requireStatus(t, resp, http.StatusOK, "create user for audit")

	resp, entries := doJSONArray(t, http.MethodGet, base+"/api/v1/admin/audit", admin, nil)
	requireStatus(t, resp, http.StatusOK, "audit log")
	found := false
	for _, e := range entries {
		if action, _ := e["action"].(string); action == "user.create" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected user.create audit entry")
	}
}
