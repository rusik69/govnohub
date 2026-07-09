//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestFullUserJourney(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	user := "e2euser"
	token := testutil.RegisterAndLogin(t, base, user)

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/users/"+user+"/repos", token, map[string]any{
		"name": "app", "description": "e2e app", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+user+"/app/issues", token, map[string]string{
		"title": "first issue", "body": "details",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("issue: %d", resp.StatusCode)
	}
	issueNum, _ := out["number"].(float64)

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+user+"/app/branches", token, map[string]string{
		"name": "feature", "base": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create branch: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+user+"/app/pulls", token, map[string]string{
		"title": "feature", "body": "", "head": "feature", "base": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pr: %d", resp.StatusCode)
	}

	wf := `name: CI
on: push
jobs:
  test:
    runs-on: linux
    steps:
      - run: echo ok`
	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+user+"/app/actions/workflows", token, map[string]string{
		"name": "CI", "path": ".github/workflows/ci.yml", "content": wf,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("workflow: %d %v", resp.StatusCode, out)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+user+"/app/releases", token, map[string]any{
		"tag_name": "v0.1.0", "name": "v0.1", "body": "first",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("release: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+user+"/app/star", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("star: %d", resp.StatusCode)
	}

	other := "e2ecollab"
	testutil.RegisterAndLogin(t, base, other)
	resp, _ = testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+user+"/app/collaborators/"+other, token, map[string]string{
		"permission": "read",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add collaborator: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+user+"/app/protected-branches", token, map[string]any{
		"branch": "main", "required_checks": []string{}, "require_reviews": 0,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("protect branch: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+user+"/app/issues/"+itoa(int(issueNum))+"/close", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("close issue: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/user/repos", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list repos: %d", resp.StatusCode)
	}
}

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
