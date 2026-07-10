//go:build integration

package integration

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func webClient(t *testing.T, base, token string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(base)
	jar.SetCookies(u, []*http.Cookie{{Name: "govnohub_session", Value: token, Path: "/"}})
	return &http.Client{Jar: jar}
}

func TestWebRepoPageShowsBranches(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	token := testutil.RegisterAndLogin(t, env.URL, "webbranch")
	testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/webbranch/repos", token, map[string]any{
		"name": "app", "private": false,
	})

	client := webClient(t, env.URL, token)
	resp, err := client.Get(env.URL + "/webbranch/app")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `id="branch-select"`) {
		t.Fatal("expected branch selector on repo page")
	}
}

func TestWebIssueCreateFormHasBody(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	token := testutil.RegisterAndLogin(t, env.URL, "webissue")
	testutil.DoJSON(t, http.MethodPost, env.URL+"/api/v1/users/webissue/repos", token, map[string]any{
		"name": "app", "private": false,
	})

	client := webClient(t, env.URL, token)
	resp, err := client.Get(env.URL + "/webissue/app/issues")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `name="body"`) {
		t.Fatal("expected issue body field")
	}
}
