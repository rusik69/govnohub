package testenv

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// LocalDeploy starts api-server and git-server as real processes backed by
// testcontainers PostgreSQL — closer to a production deployment than httptest.
type LocalDeploy struct {
	*Env
	apiCmd    *exec.Cmd
	gitCmd    *exec.Cmd
	GitURL    string
	GitSSHURL string
}

func NewLocalDeploy(t *testing.T) *LocalDeploy {
	t.Helper()
	root := moduleRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	apiBin := filepath.Join(binDir, "api-server")
	gitBin := filepath.Join(binDir, "git-server")
	buildBin(t, root, "./cmd/api-server", apiBin)
	buildBin(t, root, "./cmd/git-server", gitBin)

	env := New(t)
	apiPort := freePort(t)
	gitPort := freePort(t)
	sshPort := freePort(t)
	sshHostKey := filepath.Join(binDir, "ssh_host_key")

	baseEnv := []string{
		"DATABASE_URL=" + env.DatabaseURL,
		"JWT_SECRET=test-secret",
		"GIT_ROOT=" + env.GitRoot,
		"ARTIFACT_ROOT=" + env.ArtifactRoot,
		"OPENSEARCH_URL=http://127.0.0.1:1",
		"AI_REVIEW_ENABLED=false",
		"ALLOW_PUBLIC_REGISTRATION=true",
	}

	apiCmd := exec.Command(apiBin)
	apiCmd.Env = append(os.Environ(), append(baseEnv, "HTTP_ADDR=:"+itoa(apiPort))...)

	gitCmd := exec.Command(gitBin)
	gitCmd.Stderr = os.Stderr
	gitCmd.Env = append(os.Environ(), append(baseEnv,
		"GIT_HTTP_ADDR=:"+itoa(gitPort),
		"GIT_SSH_ADDR=:"+itoa(sshPort),
		"GIT_SSH_HOST_KEY="+sshHostKey,
	)...)

	if err := apiCmd.Start(); err != nil {
		t.Fatalf("start api-server: %v", err)
	}
	if err := gitCmd.Start(); err != nil {
		apiCmd.Process.Kill()
		t.Fatalf("start git-server: %v", err)
	}

	apiURL := fmt.Sprintf("http://127.0.0.1:%d", apiPort)
	gitURL := fmt.Sprintf("http://127.0.0.1:%d", gitPort)
	gitSSHURL := fmt.Sprintf("ssh://git@127.0.0.1:%d", sshPort)
	waitHTTP(t, apiURL+"/healthz", 30*time.Second)
	waitHTTP(t, gitURL+"/healthz", 30*time.Second)
	waitTCP(t, fmt.Sprintf("127.0.0.1:%d", sshPort), 30*time.Second)

	env.Server.Close()
	env.URL = apiURL

	ld := &LocalDeploy{Env: env, apiCmd: apiCmd, gitCmd: gitCmd, GitURL: gitURL, GitSSHURL: gitSSHURL}
	origCleanup := env.Cleanup
	env.Cleanup = func() {
		if ld.apiCmd.Process != nil {
			ld.apiCmd.Process.Kill()
		}
		if ld.gitCmd.Process != nil {
			ld.gitCmd.Process.Kill()
		}
		origCleanup()
	}
	return ld
}

func buildBin(t *testing.T, root, pkg, out string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out2, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, out2)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func waitHTTP(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", url)
}

func waitTCP(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for tcp %s", addr)
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
