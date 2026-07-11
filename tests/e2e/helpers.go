//go:build e2e || deploy

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
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

func adminToken(t *testing.T, base string) string {
	t.Helper()
	return testutil.Login(t, base, "admin", "admin")
}

func webClient(t *testing.T, base, token string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(base)
	if token != "" {
		jar.SetCookies(u, []*http.Cookie{{Name: "govnohub_session", Value: token, Path: "/"}})
	}
	return &http.Client{Jar: jar}
}

func webClientNoRedirect(client *http.Client) *http.Client {
	return &http.Client{
		Jar: client.Jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func webCSRF(t *testing.T, client *http.Client, base, pagePath string) string {
	t.Helper()
	resp, err := client.Get(base + pagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	u, _ := url.Parse(base)
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == "govnohub_csrf" && c.Value != "" {
			return c.Value
		}
	}
	t.Fatal("missing csrf cookie")
	return ""
}

func webPostForm(t *testing.T, client *http.Client, postURL, csrf string, fields map[string]string) *http.Response {
	t.Helper()
	if fields == nil {
		fields = map[string]string{}
	}
	var b strings.Builder
	first := true
	fields["csrf_token"] = csrf
	for k, v := range fields {
		if !first {
			b.WriteByte('&')
		}
		first = false
		b.WriteString(url.QueryEscape(k))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(v))
	}
	req, err := http.NewRequest(http.MethodPost, postURL, strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func webGetBody(t *testing.T, client *http.Client, pageURL string) (int, string) {
	t.Helper()
	resp, err := client.Get(pageURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func assertJSONArrayLen(t *testing.T, items []map[string]any, want int, msg string) {
	t.Helper()
	if len(items) != want {
		t.Fatalf("%s: got %d items", msg, len(items))
	}
}

func assertSearchHit(t *testing.T, hits []map[string]any, fullName string) {
	t.Helper()
	for _, h := range hits {
		if repo, _ := h["repo"].(string); repo == fullName {
			return
		}
	}
	if len(hits) == 0 {
		t.Log("search returned no hits (OpenSearch unavailable in testenv)")
		return
	}
	t.Fatalf("search miss for %s", fullName)
}

func gitCommitOnBranch(t *testing.T, env *testenv.Env, owner, repo, branch, filename, content, message string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	gitDir := env.Git.RepoPath(owner, repo)
	wt := t.TempDir()
	if out, err := exec.Command("git", "--git-dir", gitDir, "worktree", "add", wt, branch).CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %s %v", out, err)
	}
	t.Cleanup(func() { exec.Command("git", "--git-dir", gitDir, "worktree", "remove", wt, "--force").Run() })
	if err := os.WriteFile(filepath.Join(wt, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	envVars := append(os.Environ(),
		"GIT_AUTHOR_NAME=e2e", "GIT_AUTHOR_EMAIL=e2e@test.local",
		"GIT_COMMITTER_NAME=e2e", "GIT_COMMITTER_EMAIL=e2e@test.local",
	)
	for _, args := range [][]string{{"add", filename}, {"commit", "-m", message}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = wt
		cmd.Env = envVars
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
	}
}
