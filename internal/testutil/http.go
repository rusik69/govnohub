package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func DoJSON(t *testing.T, method, url, token string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		r = bytes.NewReader(b)
	}
	return doRequest(t, method, url, token, r, "application/json")
}

func DoBody(t *testing.T, method, url, token, contentType string, body io.Reader) (*http.Response, map[string]any) {
	t.Helper()
	return doRequest(t, method, url, token, body, contentType)
}

func doRequest(t *testing.T, method, url, token string, body io.Reader, contentType string) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if body != nil && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	var out map[string]any
	if resp.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(resp.Body).Decode(&out)
	}
	return resp, out
}

func RegisterAndLogin(t *testing.T, baseURL, username string) string {
	t.Helper()
	resp, _ := DoJSON(t, http.MethodPost, baseURL+"/api/v1/users", "", map[string]string{
		"username": username,
		"email":    username + "@test.local",
		"password": "password123",
	})
	if resp.StatusCode == http.StatusForbidden {
		adminToken := Login(t, baseURL, "admin", "admin")
		AdminCreateUser(t, baseURL, adminToken, username)
	}
	return Login(t, baseURL, username, "password123")
}

func Login(t *testing.T, baseURL, username, password string) string {
	t.Helper()
	resp, out := DoJSON(t, http.MethodPost, baseURL+"/api/v1/auth/login", "", map[string]string{
		"username": username,
		"password": password,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d", resp.StatusCode)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatal("missing token")
	}
	return token
}

func AdminCreateUser(t *testing.T, baseURL, adminToken, username string) {
	t.Helper()
	resp, _ := DoJSON(t, http.MethodPost, baseURL+"/api/v1/admin/users", adminToken, map[string]string{
		"username": username,
		"email":    username + "@test.local",
		"password": "password123",
		"role":     "user",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin create user %s: %d", username, resp.StatusCode)
	}
}

// RequireOK fails the test immediately if resp status is not 2xx.
// Useful for assertions like "this call should succeed".
func RequireOK(t *testing.T, resp *http.Response, msg string) {
	t.Helper()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("%s: unexpected status %d (want 2xx)", msg, resp.StatusCode)
	}
}

// RequireStatus fails the test immediately if resp status does not match want.
func RequireStatus(t *testing.T, resp *http.Response, want int, msg string) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("%s: got status %d (want %d)", msg, resp.StatusCode, want)
	}
}
