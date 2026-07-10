//go:build e2e

package e2e

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
	"golang.org/x/crypto/ssh"
)

func testSSHPublicKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return string(ssh.MarshalAuthorizedKey(pub))
}

func TestPATLifecycle(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "pat" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "app")

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/user/tokens", token, map[string]any{
		"name": "ci", "scopes": []string{"repo", "repo:write", "workflow"},
	})
	requireStatus(t, resp, http.StatusOK, "create PAT")
	pat, _ := out["token"].(string)
	if pat == "" {
		t.Fatal("missing PAT token")
	}

	resp, pats := doJSONArray(t, http.MethodGet, base+"/api/v1/user/tokens", token, nil)
	requireStatus(t, resp, http.StatusOK, "list PATs")
	if len(pats) != 1 {
		t.Fatalf("expected 1 PAT, got %d", len(pats))
	}
	patID, _ := pats[0]["id"].(string)
	if patID == "" {
		t.Fatal("missing PAT id")
	}

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app", pat, nil)
	requireStatus(t, resp, http.StatusOK, "PAT repo access")

	resp, _ = testutil.DoJSON(t, http.MethodDelete, base+"/api/v1/user/tokens/"+patID, token, nil)
	requireStatus(t, resp, http.StatusOK, "revoke PAT")

	resp, _ = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+owner+"/app", pat, nil)
	requireStatus(t, resp, http.StatusUnauthorized, "revoked PAT rejected")
}

func TestSSHKeyLifecycle(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	token := testutil.RegisterAndLogin(t, base, "ssh"+uniqueSuffix()[len(uniqueSuffix())-6:])
	pub := testSSHPublicKey(t)

	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/user/ssh-keys", token, map[string]string{
		"title": "laptop", "key": pub,
	})
	requireStatus(t, resp, http.StatusOK, "create SSH key")
	keyID, _ := out["id"].(string)
	if keyID == "" {
		t.Fatal("missing key id")
	}

	resp, keys := doJSONArray(t, http.MethodGet, base+"/api/v1/user/ssh-keys", token, nil)
	requireStatus(t, resp, http.StatusOK, "list SSH keys")
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	resp, _ = testutil.DoJSON(t, http.MethodDelete, base+"/api/v1/user/ssh-keys/"+keyID, token, nil)
	requireStatus(t, resp, http.StatusOK, "delete SSH key")

	resp, keys = doJSONArray(t, http.MethodGet, base+"/api/v1/user/ssh-keys", token, nil)
	requireStatus(t, resp, http.StatusOK, "list SSH keys after delete")
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}
}
