//go:build deploy

package e2e

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"golang.org/x/crypto/ssh"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func requireSSH(t *testing.T) {
	t.Helper()
	requireGit(t)
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh not installed")
	}
}

const gitSSHOpts = "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o SendEnv=GIT_PROTOCOL"

func gitRemoteURL(gitBase, owner, repo, user, password string) string {
	remote := fmt.Sprintf("%s/%s/%s.git", strings.TrimRight(gitBase, "/"), owner, repo)
	return strings.Replace(remote, "://", "://"+user+":"+password+"@", 1)
}

func gitSSHRemoteURL(sshBase, owner, repo string) string {
	return fmt.Sprintf("%s/%s/%s.git", strings.TrimRight(sshBase, "/"), owner, repo)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s %v", args, out, err)
	}
}

func runGitSSH(t *testing.T, dir, privKey string, args ...string) {
	t.Helper()
	requireSSH(t)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_SSH_COMMAND=ssh "+gitSSHOpts+" -i "+privKey,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s %v", args, out, err)
	}
}

func runGitAllowFail(t *testing.T, dir string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func runGitSSHAllowFail(t *testing.T, dir, privKey string, args ...string) ([]byte, error) {
	t.Helper()
	requireSSH(t)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_SSH_COMMAND=ssh "+gitSSHOpts+" -i "+privKey,
	)
	return cmd.CombinedOutput()
}

func setupGit(t *testing.T, dir string) {
	t.Helper()
	for _, cfg := range [][]string{
		{"config", "user.email", "e2e@test.local"},
		{"config", "user.name", "e2e"},
	} {
		runGit(t, dir, cfg...)
	}
}

func gitClone(t *testing.T, workDir, cloneURL, dirName string) string {
	t.Helper()
	requireGit(t)
	runGit(t, workDir, "clone", cloneURL, dirName)
	repoDir := filepath.Join(workDir, dirName)
	setupGit(t, repoDir)
	return repoDir
}

func gitCloneSSH(t *testing.T, workDir, sshURL, privKey, dirName string) string {
	t.Helper()
	requireSSH(t)
	runGitSSH(t, workDir, privKey, "clone", sshURL, dirName)
	repoDir := filepath.Join(workDir, dirName)
	setupGit(t, repoDir)
	return repoDir
}

func generateSSHKeyPair(t *testing.T) (pub string, privPath string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pub = string(ssh.MarshalAuthorizedKey(sshPub))
	privPath = filepath.Join(t.TempDir(), "id_rsa")
	pemKey := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	if err := os.WriteFile(privPath, pemKey, 0o600); err != nil {
		t.Fatal(err)
	}
	return pub, privPath
}

func registerSSHKey(t *testing.T, base, token string) (privPath string) {
	t.Helper()
	pub, privPath := generateSSHKeyPair(t)
	resp, _ := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/user/ssh-keys", token, map[string]string{
		"title": "e2e", "key": pub,
	})
	requireStatus(t, resp, http.StatusOK, "register ssh key")
	return privPath
}

func testGitPush(t *testing.T, gitBase, owner, repo, user, password string) {
	t.Helper()
	work := t.TempDir()
	repoDir := gitClone(t, work, gitRemoteURL(gitBase, owner, repo, user, password), "repo")
	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("e2e\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "e2e commit")
	runGit(t, repoDir, "branch", "-M", "main")
	runGit(t, repoDir, "push", "--force", "origin", "main")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
