//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestAdminUserManagement(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")
	userToken := testutil.RegisterAndLogin(t, env.URL, "regular")

	// Non-admin cannot list users
	resp, _ := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/admin/users", userToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin list status=%d", resp.StatusCode)
	}

	// Admin can create a user
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/admin/users", adminToken, map[string]string{
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
	if _, ok := out["id"]; !ok {
		t.Fatal("missing id in create user response")
	}

	// Admin cannot create duplicate user
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/admin/users", adminToken, map[string]string{
		"username": "newbie",
		"email":    "newbie2@test.local",
		"password": "secret123",
		"role":     "user",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate user, got %d", resp.StatusCode)
	}

	// Create user with missing fields fails
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/admin/users", adminToken, map[string]string{
		"username": "",
		"email":    "",
		"password": "",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty fields, got %d", resp.StatusCode)
	}

	// Public self-registration is forbidden
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users", "", map[string]string{
		"username": "blocked",
		"email":    "blocked@test.local",
		"password": "secret123",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("public register status=%d", resp.StatusCode)
	}
}

func TestAdminListUsers(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")

	// Admin creates a couple of users
	for _, name := range []string{"alice", "bob"} {
		resp, _ := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/admin/users", adminToken, map[string]string{
			"username": name,
			"email":    name + "@test.local",
			"password": "password123",
			"role":     "user",
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("create user %s status=%d", name, resp.StatusCode)
		}
	}

	// List all users — response is a JSON array
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users status=%d", resp.StatusCode)
	}
	var users []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		t.Fatal(err)
	}
	if len(users) < 3 {
		t.Fatalf("expected at least 3 users (admin + alice + bob), got %d", len(users))
	}
	usernames := make(map[string]bool)
	for _, u := range users {
		n, _ := u["username"].(string)
		usernames[n] = true
	}
	if !usernames["admin"] {
		t.Error("missing admin in user list")
	}
	if !usernames["alice"] {
		t.Error("missing alice in user list")
	}
	if !usernames["bob"] {
		t.Error("missing bob in user list")
	}
}

func TestAdminDeleteUser(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")

	// Create a user to delete
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/admin/users", adminToken, map[string]string{
		"username": "deleteme",
		"email":    "deleteme@test.local",
		"password": "password123",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create user status=%d", resp.StatusCode)
	}
	targetID, _ := out["id"].(string)
	if targetID == "" {
		t.Fatal("missing user id")
	}

	// Delete the user
	resp, _ = testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/admin/users/"+targetID, adminToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete user status=%d", resp.StatusCode)
	}

	// Verify user is gone by checking the list doesn't contain them
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	listResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	var users []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&users); err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if n, _ := u["username"].(string); n == "deleteme" {
			t.Fatal("deleted user still appears in list")
		}
	}
}

func TestAdminCannotDeleteSelf(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")

	// Get admin's own user ID
	resp, out := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user", adminToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get admin info status=%d", resp.StatusCode)
	}
	adminID, _ := out["id"].(string)
	if adminID == "" {
		t.Fatal("missing admin user id")
	}

	// Admin cannot delete themselves
	resp, _ = testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/admin/users/"+adminID, adminToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for self-deletion, got %d", resp.StatusCode)
	}
}

func TestAdminCannotDeleteLastAdmin(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")

	// Get admin's own user ID
	resp, out := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/user", adminToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get admin info status=%d", resp.StatusCode)
	}
	adminID, _ := out["id"].(string)

	// Try to delete the admin (the only admin) — should fail
	resp, _ = testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/admin/users/"+adminID, adminToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for deleting last admin, got %d", resp.StatusCode)
	}
}

func TestAdminDeleteNonExistentUser(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")

	// Delete a non-existent user
	nonexistentID := uuid.New().String()
	resp, _ := testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/admin/users/"+nonexistentID, adminToken, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent user, got %d", resp.StatusCode)
	}
}

func TestAdminDeleteUserInvalidID(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")

	// Delete with invalid UUID
	resp, _ := testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/admin/users/not-a-uuid", adminToken, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid UUID, got %d", resp.StatusCode)
	}
}

func TestAdminAuditLog(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")
	userToken := testutil.RegisterAndLogin(t, env.URL, "audittestuser")

	// Non-admin cannot view audit log
	resp, _ := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/admin/audit", userToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin audit access status=%d", resp.StatusCode)
	}

	// Admin can view audit log
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/admin/audit", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin audit log status=%d", resp.StatusCode)
	}
	var entries []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}

	// There should be audit entries for the user creation (RegisterAndLogin creates a user via admin)
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry")
	}

	// Verify entry structure
	entry := entries[0]
	if _, ok := entry["id"]; !ok {
		t.Error("audit entry missing id")
	}
	if _, ok := entry["action"]; !ok {
		t.Error("audit entry missing action")
	}
	if _, ok := entry["resource_type"]; !ok {
		t.Error("audit entry missing resource_type")
	}
	if _, ok := entry["resource_id"]; !ok {
		t.Error("audit entry missing resource_id")
	}
	if _, ok := entry["created_at"]; !ok {
		t.Error("audit entry missing created_at")
	}
}

func TestAdminAuditRecordsCreateAndDelete(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")

	// Create a user — this should generate an audit entry
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/admin/users", adminToken, map[string]string{
		"username": "audituser",
		"email":    "audituser@test.local",
		"password": "password123",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create user status=%d", resp.StatusCode)
	}
	userID, _ := out["id"].(string)

	// Read audit log and verify user.create entry exists
	req, _ := http.NewRequest(http.MethodGet, env.URL+"/api/v1/admin/audit", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	listResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	var entries []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}

	foundCreate := false
	for _, e := range entries {
		if action, _ := e["action"].(string); action == "user.create" {
			foundCreate = true
			break
		}
	}
	if !foundCreate {
		t.Error("audit log missing user.create entry")
	}

	// Delete the user — should generate a user.delete audit entry
	resp, _ = testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/admin/users/"+userID, adminToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete user status=%d", resp.StatusCode)
	}

	// Read audit log again and check for user.delete
	listResp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp2.Body.Close()
	var entriesAfter []map[string]any
	if err := json.NewDecoder(listResp2.Body).Decode(&entriesAfter); err != nil {
		t.Fatal(err)
	}

	foundDelete := false
	for _, e := range entriesAfter {
		if action, _ := e["action"].(string); action == "user.delete" {
			foundDelete = true
			break
		}
	}
	if !foundDelete {
		t.Error("audit log missing user.delete entry after user deletion")
	}
}

func TestNonAdminCannotAccessAdminEndpoints(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	userToken := testutil.RegisterAndLogin(t, env.URL, "regularuser")

	// List users
	resp, _ := testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/admin/users", userToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin list users: expected 403, got %d", resp.StatusCode)
	}

	// Create user (should fail even trying)
	resp, _ = testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/admin/users", userToken, map[string]string{
		"username": "hacker",
		"email":    "hacker@test.local",
		"password": "hack123",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin create user: expected 403, got %d", resp.StatusCode)
	}

	// Delete user
	resp, _ = testutil.DoJSON(t, http.MethodDelete, env.URL+"/api/v1/admin/users/"+uuid.New().String(), userToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin delete user: expected 403, got %d", resp.StatusCode)
	}

	// Audit log
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/admin/audit", userToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin audit log: expected 403, got %d", resp.StatusCode)
	}
}

func TestAdminCreateUserWithAdminRole(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()

	adminToken := testutil.Login(t, env.URL, "admin", "admin")

	// Create a user with admin role
	resp, out := testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/admin/users", adminToken, map[string]string{
		"username": "secondadmin",
		"email":    "secondadmin@test.local",
		"password": "adminpass",
		"role":     "admin",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create admin user status=%d", resp.StatusCode)
	}
	if role, _ := out["role"].(string); role != "admin" {
		t.Fatalf("role=%q want admin", role)
	}

	// Login as the new admin
	secondAdminToken := testutil.Login(t, env.URL, "secondadmin", "adminpass")

	// Verify the new admin can list users
	resp, _ = testutil.DoJSON(t, http.MethodGet, env.URL+"/api/v1/admin/users", secondAdminToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second admin list users status=%d", resp.StatusCode)
	}
}
