//go:build k8s

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	for time.Now().Before(deadline) {
		out, err := exec.Command("kubectl", "get", "pods", "-n", ns,
			"-l", "app.kubernetes.io/name=govnohub", "--field-selector=status.phase=Running").CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatal("govnohub pods not ready")
}
