//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestAdminUserManagement(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminSvc := auth.NewService(env.Pool, "test-secret")
	if err := adminSvc.BootstrapAdmin(t.Context(), "admin", "admin@test.local", "adminpass"); err != nil {
		t.Fatal(err)
	}
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/auth/login", "", map[string]string{
		"username": "admin",
		"password": "adminpass",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin login status=%d", resp.StatusCode)
	}
	adminToken, _ := out["token"].(string)

	userToken := testutil.RegisterAndLogin(t, env.URL, "regular")

	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/admin/users", userToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin list status=%d", resp.StatusCode)
	}

	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/admin/users", adminToken, map[string]string{
		"username": "newbie",
		"email":    "newbie@test.local",
		"password": "secret123",
		"role":     "user",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create user status=%d", resp.StatusCode)
	}
	if out["username"] != "newbie" {
		t.Fatalf("unexpected user: %v", out)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users", "", map[string]string{
		"username": "blocked",
		"email":    "blocked@test.local",
		"password": "secret123",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("public register status=%d", resp.StatusCode)
	}
}
