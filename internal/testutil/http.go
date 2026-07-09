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
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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
	_, _ = DoJSON(t, http.MethodPost, baseURL+"/api/v1/users", "", map[string]string{
		"username": username,
		"email":    username + "@test.local",
		"password": "password123",
	})
	resp, out := DoJSON(t, http.MethodPost, baseURL+"/api/v1/auth/login", "", map[string]string{
		"username": username,
		"password": "password123",
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
