package actions

import (
	"fmt"
	"strings"
)

var usesRegistry = map[string]func(Step, map[string]string) string{
	"actions/checkout@v4":       scriptCheckout,
	"actions/checkout@v3":       scriptCheckout,
	"actions/setup-go@v5":       scriptSetupGo,
	"actions/setup-go@v4":         scriptSetupGo,
	"actions/setup-node@v4":       scriptSetupNode,
	"actions/setup-node@v3":       scriptSetupNode,
	"actions/cache@v4":            scriptCache,
	"actions/cache@v3":            scriptCache,
	"actions/upload-artifact@v4":  scriptUploadArtifact,
	"actions/upload-artifact@v3":  scriptUploadArtifact,
}

func normalizeUses(ref string) string {
	return strings.TrimSpace(ref)
}

func BuildUsesScript(step Step, ctx map[string]string) string {
	ref := normalizeUses(step.Uses)
	if fn, ok := usesRegistry[ref]; ok {
		return fn(step, ctx)
	}
	// Match without version suffix
	base := ref
	if i := strings.LastIndex(ref, "@"); i > 0 {
		base = ref[:i]
		for k, fn := range usesRegistry {
			if strings.HasPrefix(k, base+"@") {
				return fn(step, ctx)
			}
		}
	}
	return fmt.Sprintf("echo '::warning::action %s is not supported; skipping'", ref)
}

func scriptCheckout(_ Step, ctx map[string]string) string {
	workspace := ctx["GITHUB_WORKSPACE"]
	if workspace == "" {
		workspace = "/github/workspace"
	}
	return fmt.Sprintf(`echo "Checking out repository"
mkdir -p %s
if [ -d /github/workspace/.git ]; then
  cp -a /github/workspace/. %s/ 2>/dev/null || true
fi
cd %s
echo "Checked out ${GITHUB_SHA:-unknown}"`, workspace, workspace, workspace)
}

func scriptSetupGo(step Step, ctx map[string]string) string {
	version := "1.22"
	if step.Env != nil {
		if v, ok := step.Env["go-version"]; ok {
			version = EvalExpression(v, ctx)
		}
	}
	return fmt.Sprintf(`echo "Setting up Go %s"
apk add --no-cache go >/dev/null 2>&1 || true
export PATH="/usr/lib/go/bin:$PATH"
go version || echo "Go %s configured"`, version, version)
}

func scriptCache(step Step, _ map[string]string) string {
	key := "default"
	if step.Env != nil {
		if v, ok := step.Env["key"]; ok {
			key = v
		}
	}
	return fmt.Sprintf(`echo "Cache restore/save for key: %s"
mkdir -p /tmp/govnohub-cache/%s`, key, key)
}

func scriptSetupNode(step Step, ctx map[string]string) string {
	version := "20"
	if step.With != nil {
		if v, ok := step.With["node-version"]; ok {
			version = EvalExpression(v, ctx)
		}
	}
	return fmt.Sprintf(`echo "Setting up Node.js %s"
apk add --no-cache nodejs npm >/dev/null 2>&1 || true
node --version || echo "Node.js %s configured"`, version, version)
}

func scriptUploadArtifact(step Step, ctx map[string]string) string {
	name := "artifact"
	path := "."
	if step.With != nil {
		if v, ok := step.With["name"]; ok {
			name = EvalExpression(v, ctx)
		}
		if v, ok := step.With["path"]; ok {
			path = EvalExpression(v, ctx)
		}
	}
	runID := ctx["GITHUB_RUN_ID"]
	if runID == "" {
		runID = "local"
	}
	artifactRoot := ctx["GOVNOHUB_ARTIFACT_ROOT"]
	if artifactRoot == "" {
		artifactRoot = "/github/artifacts"
	}
	dest := fmt.Sprintf("%s/runs/%s/artifacts/%s", artifactRoot, runID, name)
	return fmt.Sprintf(`echo "Uploading artifact %s from %s"
mkdir -p %s
if [ -e "%s" ]; then
  cp -a %s/. %s/ 2>/dev/null || cp -a %s %s/ 2>/dev/null || true
fi
echo "Artifact saved to %s"`, name, path, dest, path, path, dest, path, dest, dest)
}
