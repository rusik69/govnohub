//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestLoginSuccess(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	// Login as admin (bootstrapped in testenv)
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/auth/login", "", map[string]string{
		"username": "admin",
		"password": "admin",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d out=%v", resp.StatusCode, out)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatal("missing token")
	}
	u, ok := out["user"].(map[string]any)
	if !ok {
		t.Fatal("missing user object in login response")
	}
	if u["username"] != "admin" {
		t.Fatalf("user.username=%q want admin", u["username"])
	}
	if role, _ := u["role"].(string); role != "admin" {
		t.Fatalf("user.role=%q want admin", role)
	}
}

func TestLoginFailures(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	// Wrong password
	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/auth/login", "", map[string]string{
		"username": "admin",
		"password": "wrongpass",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", resp.StatusCode)
	}

	// Non-existent user
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/auth/login", "", map[string]string{
		"username": "nonexistent",
		"password": "somepass",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-existent user, got %d", resp.StatusCode)
	}
}

func TestPublicRegistrationBlocked(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	// Direct registration should be forbidden (public registration disabled)
	resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users", "", map[string]string{
		"username": "newuser",
		"email":    "newuser@test.local",
		"password": "secret123",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for public registration, got %d", resp.StatusCode)
	}
}

func TestCurrentUserProfile(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	// Login as admin first
	adminToken := testutil.Login(t, env.URL, "admin", "admin")

	// Get current user profile
	resp, out := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user", adminToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get current user status=%d out=%v", resp.StatusCode, out)
	}
	if out["username"] != "admin" {
		t.Fatalf("username=%q want admin", out["username"])
	}
	if _, ok := out["id"]; !ok {
		t.Fatal("missing id in user profile")
	}
	if _, ok := out["email"]; !ok {
		t.Fatal("missing email in user profile")
	}
}

func TestPATLifecycle(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	userToken := testutil.RegisterAndLogin(t, env.URL, "patuser")

	// Create a PAT with repo scope
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/user/tokens", userToken, map[string]any{
		"name":   "my-token",
		"scopes": []string{"repo"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create PAT status=%d out=%v", resp.StatusCode, out)
	}
	pat, _ := out["token"].(string)
	if pat == "" {
		t.Fatal("missing token in create response")
	}
	if len(pat) < 10 || pat[:4] != "ghp_" {
		t.Fatalf("PAT=%q should start with ghp_", pat)
	}

	// List PATs
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user/tokens", userToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list PATs status=%d out=%v", resp.StatusCode, out)
	}
	// The response is a JSON array — decode it manually
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/user/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	listResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list PATs status=%d", listResp.StatusCode)
	}
	var pats []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&pats); err != nil {
		t.Fatal(err)
	}
	if len(pats) != 1 {
		t.Fatalf("expected 1 PAT, got %d", len(pats))
	}
	if pats[0]["name"] != "my-token" {
		t.Fatalf("PAT name=%q want my-token", pats[0]["name"])
	}
	patID, _ := pats[0]["id"].(string)
	if patID == "" {
		t.Fatal("missing PAT id in list response")
	}

	// Revoke the PAT
	resp, out = testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/user/tokens/"+patID, userToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revoke PAT status=%d out=%v", resp.StatusCode, out)
	}
	if out["status"] != "revoked" {
		t.Fatalf("revoke status=%q want revoked", out["status"])
	}

	// List PATs should now be empty
	listResp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp2.Body.Close()
	var patsAfter []map[string]any
	if err := json.NewDecoder(listResp2.Body).Decode(&patsAfter); err != nil {
		t.Fatal(err)
	}
	if len(patsAfter) != 0 {
		t.Fatalf("expected 0 PATs after revoke, got %d", len(patsAfter))
	}
}

func TestPATRevokeNonExistent(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	userToken := testutil.RegisterAndLogin(t, env.URL, "revokenonexist")

	// Revoke a non-existent PAT
	resp, _ := testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/user/tokens/"+uuid.New().String(), userToken, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent PAT, got %d", resp.StatusCode)
	}
}

func TestPATAuthWorks(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	userToken := testutil.RegisterAndLogin(t, env.URL, "patauthuser")
	owner := "patauthuser"

	// Create a PAT with repo scope
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/user/tokens", userToken, map[string]any{
		"name":   "ci-token",
		"scopes": []string{"repo", "repo:write"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create PAT status=%d", resp.StatusCode)
	}
	pat, _ := out["token"].(string)
	if pat == "" {
		t.Fatal("missing PAT")
	}

	// Use PAT to create a repo (write operation)
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", pat, map[string]any{
		"name": "pat-repo", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo with PAT status=%d out=%v", resp.StatusCode, out)
	}
	if out["name"] != "pat-repo" {
		t.Fatalf("repo name=%q", out["name"])
	}

	// Use PAT to read the repo
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/pat-repo", pat, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get repo with PAT status=%d", resp.StatusCode)
	}
}

func TestPATScopeReadOnlyEnforcement(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	userToken := testutil.RegisterAndLogin(t, env.URL, "scopereadonly")
	owner := "scopereadonly"

	// Create a PAT with only read scope (repo, not repo:write)
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/user/tokens", userToken, map[string]any{
		"name":   "readonly",
		"scopes": []string{"repo"}, // read-only scope
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create PAT status=%d", resp.StatusCode)
	}
	readOnlyPat, _ := out["token"].(string)
	if readOnlyPat == "" {
		t.Fatal("missing PAT")
	}

	// Use JWT token to create a repo first
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", userToken, map[string]any{
		"name": "scope-test", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	// Try to create another repo with read-only PAT -> should be forbidden (repo:write required)
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", readOnlyPat, map[string]any{
		"name": "should-fail", "private": false,
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for read-only PAT creating repo, got %d", resp.StatusCode)
	}

	// Read the repo with read-only PAT -> should work
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/repos/"+owner+"/scope-test", readOnlyPat, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for read-only PAT reading repo, got %d", resp.StatusCode)
	}
}

func TestPATScopeEnforcement(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	userToken := testutil.RegisterAndLogin(t, env.URL, "scopetest")
	owner := "scopetest"

	// Create a PAT with no scopes (empty scopes list = full access per HasScope logic)
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/user/tokens", userToken, map[string]any{
		"name":   "full-access",
		"scopes": []string{},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create PAT status=%d", resp.StatusCode)
	}
	fullPat, _ := out["token"].(string)

	// Create a PAT with workflow scope only
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/user/tokens", userToken, map[string]any{
		"name":   "workflow-only",
		"scopes": []string{"workflow"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create PAT status=%d", resp.StatusCode)
	}
	workflowPat, _ := out["token"].(string)

	// Create repo with full-access PAT (empty scopes)
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", fullPat, map[string]any{
		"name": "scopes-repo", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo with empty-scope PAT status=%d", resp.StatusCode)
	}

	// Workflow-only PAT should not be able to create repo (no repo:write scope)
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", workflowPat, map[string]any{
		"name": "should-fail", "private": false,
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for workflow-only PAT creating repo, got %d", resp.StatusCode)
	}
}

func TestLoginTwiceReturnsValidTokens(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	token1 := testutil.Login(t, env.URL, "admin", "admin")
	token2 := testutil.Login(t, env.URL, "admin", "admin")

	// Both should be valid JWT tokens
	if token1 == "" || token2 == "" {
		t.Fatal("tokens should not be empty")
	}
	// Use the tokens to verify they work
	resp, _ := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user", token1, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token1 invalid: %d", resp.StatusCode)
	}
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user", token2, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token2 invalid: %d", resp.StatusCode)
	}
}

func TestPATWithoutScopesHasFullAccess(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	userToken := testutil.RegisterAndLogin(t, env.URL, "noscopepat")
	owner := "noscopepat"

	// Create PAT with no scopes (empty scopes = full access in HasScope)
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/user/tokens", userToken, map[string]any{
		"name":   "no-scope-token",
		"scopes": []string{},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create PAT status=%d", resp.StatusCode)
	}
	pat, _ := out["token"].(string)

	// Create repo with this PAT
	resp, out = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/"+owner+"/repos", pat, map[string]any{
		"name": "full-access-repo", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo with no-scope PAT status=%d out=%v", resp.StatusCode, out)
	}
}

func TestAuthRequiredOnProtectedEndpoints(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	// Accessing protected endpoints without auth should return 401
	resp, _ := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", resp.StatusCode)
	}

	// Create repo without auth
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/someone/repos", "", map[string]any{
		"name": "nope",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", resp.StatusCode)
	}
}

func TestPATValidateAfterAdminCreate(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")

	// Admin creates a user
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/admin/users", adminToken, map[string]string{
		"username": "createdbyadmin",
		"email":    "createdbyadmin@test.local",
		"password": "mypassword",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin create user status=%d", resp.StatusCode)
	}
	createdUsername, _ := out["username"].(string)
	if createdUsername != "createdbyadmin" {
		t.Fatalf("username=%q", createdUsername)
	}

	// Now login as the newly created user
	newUserToken := testutil.Login(t, env.URL, "createdbyadmin", "mypassword")

	// Verify GET /api/v1/user returns the correct user
	resp, out = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user", newUserToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get user status=%d", resp.StatusCode)
	}
	if out["username"] != "createdbyadmin" {
		t.Fatalf("username=%q want createdbyadmin", out["username"])
	}
}

func TestCreatePATWithDifferentScopes(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	userToken := testutil.RegisterAndLogin(t, env.URL, "scopesuser")

	// Create PATs with different scopes and verify they all succeed
	scopes := []struct {
		name   string
		scopes []string
	}{
		{"repo-only", []string{"repo"}},
		{"write-only", []string{"repo:write"}},
		{"workflow-only", []string{"workflow"}},
		{"read-user", []string{"read:user"}},
		{"multiple", []string{"repo", "workflow"}},
	}

	for _, s := range scopes {
		resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/user/tokens", userToken, map[string]any{
			"name":   s.name,
			"scopes": s.scopes,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("create PAT %q status=%d", s.name, resp.StatusCode)
		}
		pat, _ := out["token"].(string)
		if !auth.HasScope(s.scopes, "repo") && !auth.HasScope(s.scopes, "workflow") {
			// Just verify token format
			if pat == "" || pat[:4] != "ghp_" {
				t.Fatalf("PAT %q format invalid: %q", s.name, pat)
			}
		}
	}
}
