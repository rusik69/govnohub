//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestWebLoginLogout(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	user := "wlogin" + uniqueSuffix()[len(uniqueSuffix())-6:]
	testutil.RegisterAndLogin(t, base, user)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := webCSRF(t, client, base, "/login")

	resp := webPostForm(t, client, base+"/login", csrf, map[string]string{
		"username": user,
		"password": "password123",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d", resp.StatusCode)
	}

	code, body := webGetBody(t, client, base+"/")
	if code != http.StatusOK {
		t.Fatalf("dashboard status=%d", code)
	}
	if !strings.Contains(body, user) {
		t.Fatal("expected username on dashboard")
	}

	csrf = webCSRF(t, client, base, "/")
	resp = webPostForm(t, webClientNoRedirect(client), base+"/logout", csrf, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("logout status=%d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/login") {
		t.Fatalf("logout redirect=%q", loc)
	}
}

func TestWebDashboardCreateRepo(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	user := "wdash" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, user)
	client := webClient(t, base, token)

	csrf := webCSRF(t, client, base, "/")
	repoName := "webapp" + uniqueSuffix()[len(uniqueSuffix())-5:]
	resp := webPostForm(t, webClientNoRedirect(client), base+"/repos/create", csrf, map[string]string{
		"name":        repoName,
		"description": "from web",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create repo status=%d", resp.StatusCode)
	}

	code, body := webGetBody(t, client, base+"/")
	if code != http.StatusOK {
		t.Fatalf("dashboard status=%d", code)
	}
	if !strings.Contains(body, user+"/"+repoName) {
		t.Fatalf("expected %s/%s on dashboard", user, repoName)
	}
}

func TestWebRepoStarWatch(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	user := "wstar" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, user)
	createRepo(t, base, token, user, "app")
	client := webClient(t, base, token)
	repoPath := "/" + user + "/app"

	csrf := webCSRF(t, client, base, repoPath)
	resp := webPostForm(t, client, base+repoPath+"/star", csrf, nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Starred") {
		t.Fatal("expected Starred after star")
	}

	csrf = webCSRF(t, client, base, repoPath)
	resp = webPostForm(t, client, base+repoPath+"/unstar", csrf, nil)
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), ">Star<") && !strings.Contains(string(body), "Star\n") {
		if strings.Contains(string(body), "Starred") {
			t.Fatal("expected unstarred state")
		}
	}

	csrf = webCSRF(t, client, base, repoPath)
	resp = webPostForm(t, client, base+repoPath+"/watch", csrf, nil)
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Unwatch") {
		t.Fatal("expected Unwatch after watch")
	}

	csrf = webCSRF(t, client, base, repoPath)
	resp = webPostForm(t, client, base+repoPath+"/unwatch", csrf, nil)
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Watch") {
		t.Fatal("expected Watch after unwatch")
	}
}

func TestWebIssueCreate(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	user := "wiss" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, user)
	createRepo(t, base, token, user, "app")
	client := webClient(t, base, token)
	repoPath := "/" + user + "/app"

	csrf := webCSRF(t, client, base, repoPath+"/issues")
	title := "web issue " + uniqueSuffix()[len(uniqueSuffix())-6:]
	resp := webPostForm(t, client, base+repoPath+"/issues", csrf, map[string]string{
		"title": title,
		"body":  "created via web form",
	})
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), title) {
		t.Fatalf("expected issue title %q on page", title)
	}
}

func TestWebNotificationsPage(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	suffix := uniqueSuffix()
	owner := "wnot" + suffix[len(suffix)-6:]
	commenter := "wnotc" + suffix[len(suffix)-6:]
	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	commenterToken := testutil.RegisterAndLogin(t, base, commenter)
	createRepo(t, base, ownerToken, owner, "app")
	repo := owner + "/app"

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues", ownerToken, map[string]string{
		"title": "notify web", "body": "",
	})
	requireStatus(t, resp, http.StatusOK, "create issue")
	issueNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues/"+itoa(issueNum)+"/comments", commenterToken, map[string]string{
		"body": "web notif trigger",
	})
	requireStatus(t, resp, http.StatusOK, "add comment")
	waitForNotifications(t, base, ownerToken)

	client := webClient(t, base, ownerToken)
	code, body := webGetBody(t, client, base+"/notifications")
	if code != http.StatusOK {
		t.Fatalf("notifications status=%d", code)
	}
	if !strings.Contains(body, "Issue comment") {
		t.Fatal("expected Issue comment notification on web page")
	}
}

func TestWebRepoBranches(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	user := "wbr" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, user)
	createRepo(t, base, token, user, "app")
	client := webClient(t, base, token)

	code, body := webGetBody(t, client, base+"/"+user+"/app")
	if code != http.StatusOK {
		t.Fatalf("repo page status=%d", code)
	}
	if !strings.Contains(body, `id="branch-select"`) {
		t.Fatal("expected branch selector on repo page")
	}
	if !strings.Contains(body, "main") {
		t.Fatal("expected main branch in selector")
	}
}

func TestWebIssueCreateFormHasBody(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	user := "wform" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, user)
	createRepo(t, base, token, user, "app")
	client := webClient(t, base, token)

	_, body := webGetBody(t, client, base+"/"+user+"/app/issues")
	if !strings.Contains(body, `name="body"`) {
		t.Fatal("expected issue body field")
	}
}
