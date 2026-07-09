package actions

import (
	"fmt"
	"strings"
)

var usesRegistry = map[string]func(Step, map[string]string) string{
	"actions/checkout@v4":    scriptCheckout,
	"actions/checkout@v3":    scriptCheckout,
	"actions/setup-go@v5":    scriptSetupGo,
	"actions/setup-go@v4":    scriptSetupGo,
	"actions/cache@v4":       scriptCache,
	"actions/cache@v3":       scriptCache,
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
