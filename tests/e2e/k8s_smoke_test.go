//go:build k8s

package e2e

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rusik69/govnohub/internal/testutil"
)

func repoRoot(t *testing.T) string {
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

func TestK8sClusterReachable(t *testing.T) {
	if os.Getenv("KUBECONFIG") == "" && exec.Command("kubectl", "config", "current-context").Run() != nil {
		t.Skip("kubectl not configured")
	}
	out, err := exec.Command("kubectl", "get", "nodes").CombinedOutput()
	if err != nil {
		t.Skipf("cluster not reachable: %s", out)
	}
}

func TestHelmTemplateRenders(t *testing.T) {
	root := repoRoot(t)
	out, err := exec.Command("helm", "template", "govnohub", filepath.Join(root, "deploy/helm/govnohub"),
		"-f", filepath.Join(root, "deploy/helm/govnohub/values-k3s.yaml")).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %s %v", out, err)
	}
	if len(out) < 100 {
		t.Fatal("empty helm output")
	}
}

func TestGovnohubDeployed(t *testing.T) {
	if os.Getenv("GOVNOHUB_K8S_E2E") == "" {
		t.Skip("set GOVNOHUB_K8S_E2E=1 to run deployed smoke test")
	}
	ns := os.Getenv("NAMESPACE")
	if ns == "" {
		ns = "govnohub"
	}
	deadline := time.Now().Add(2 * time.Minute)
	ready := false
	for time.Now().Before(deadline) {
		out, err := exec.Command("kubectl", "get", "pods", "-n", ns,
			"-l", "app.kubernetes.io/name=govnohub", "--field-selector=status.phase=Running").CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			ready = true
			break
		}
		time.Sleep(5 * time.Second)
	}
	if !ready {
		t.Fatal("govnohub pods not ready")
	}

	base := os.Getenv("GOVNOHUB_BASE_URL")
	if base == "" {
		base = "http://govnohub.local"
	}
	base = strings.TrimRight(base, "/")

	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status=%d", resp.StatusCode)
	}

	adminToken := testutil.Login(t, base, "admin", "admin")
	resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/user", adminToken, nil)
	if resp.StatusCode != http.StatusOK || out["username"] != "admin" {
		t.Fatalf("admin user check: %d %v", resp.StatusCode, out)
	}
}
