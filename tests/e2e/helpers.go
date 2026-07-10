//go:build e2e || deploy

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/rusik69/govnohub/internal/testutil"
)

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func uniqueSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func requireStatus(t *testing.T, resp *http.Response, want int, msg string) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("%s: got %d", msg, resp.StatusCode)
	}
}

func createRepo(t *testing.T, base, token, owner, name string) {
	t.Helper()
	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": name, "private": false,
	})
	requireStatus(t, resp, http.StatusOK, "create repo")
}

func doJSONArray(t *testing.T, method, url, token string, body any) (*http.Response, []map[string]any) {
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
	var out []map[string]any
	if resp.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(resp.Body).Decode(&out)
	}
	return resp, out
}

func waitForNotifications(t *testing.T, base, token string) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, items := doJSONArray(t, http.MethodGet, base+"/api/v1/notifications", token, nil)
		if resp.StatusCode == http.StatusOK && len(items) > 0 {
			return items
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("expected notifications")
	return nil
}

func userID(t *testing.T, base, token string) string {
	t.Helper()
	resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/user", token, nil)
	requireStatus(t, resp, http.StatusOK, "get user")
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatal("missing user id")
	}
	return id
}
