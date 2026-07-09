//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestBranchProtectionBlocksMerge(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "protuser"
	token := testutil.RegisterAndLogin(t, base, owner)

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/users/"+owner+"/repos", token, map[string]any{
		"name": "app", "private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/protected-branches", token, map[string]any{
		"branch": "main", "required_checks": []string{}, "require_reviews": 1,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("protect: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/branches", token, map[string]string{
		"name": "feature", "base": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create branch: %d", resp.StatusCode)
	}

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/pulls", token, map[string]string{
		"title": "feature", "body": "", "head": "feature", "base": "main",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pr: %d", resp.StatusCode)
	}
	prNum := int(out["number"].(float64))

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/pulls/"+itoa(prNum)+"/merge", token, map[string]any{"squash": false})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("merge without review should be 403, got %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/pulls/"+itoa(prNum)+"/reviews", token, map[string]string{
		"state": "approved", "body": "lgtm",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("review: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+owner+"/app/pulls/"+itoa(prNum)+"/merge", token, map[string]any{"squash": false})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("merge with review should succeed, got %d", resp.StatusCode)
	}
}

func TestCollaboratorsAPI(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "collabowner"
	other := "collabother"
	ownerToken := testutil.RegisterAndLogin(t, base, owner)
	testutil.RegisterAndLogin(t, base, other)

	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/users/"+owner+"/repos", ownerToken, map[string]any{
		"name": "shared", "private": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodPut, base+"/api/v1/repos/"+owner+"/shared/collaborators/"+other, ownerToken, map[string]string{
		"permission": "read",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add collaborator: %d", resp.StatusCode)
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/shared/collaborators", ownerToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list collaborators: %d", resp.StatusCode)
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
