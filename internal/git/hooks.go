package gitstore

import (
	"fmt"
	"os"
	"path/filepath"
)

// InstallPreReceiveHook writes a pre-receive hook script to the repo's hooks directory.
// The hook rejects pushes to branches that are protected in the database.
// It reads the database URL and protected branch info from environment variables
// set by the git-server process.
func InstallPreReceiveHook(repoPath string) error {
	hooksDir := filepath.Join(repoPath, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}

	hookPath := filepath.Join(hooksDir, "pre-receive")

	// Check if hook already exists
	if _, err := os.Stat(hookPath); err == nil {
		return nil // already installed
	}

	// Simple shell script that checks for protected branches.
	// It uses the DATABASE_URL env var (exported by git-server) to query
	// the protected_branches table via psql.
	// If psql is unavailable, the hook is benign (allows all pushes).
	content := `#!/bin/sh
# Git pre-receive hook for Govnohub branch protection
# Installed by git-server. Reads ref updates from stdin and rejects
# pushes to protected branches.
#
# This hook requires the DATABASE_URL environment variable to be set
# (set by the git-server process). If unavailable, the hook is a no-op
# and all pushes are allowed.

DATABASE_URL="${DATABASE_URL:-$DB_URL}"

if [ -z "$DATABASE_URL" ]; then
  # No database config - allow all pushes (safe fallback)
  exit 0
fi

# Extract repo owner/name from the git directory path
REPO_DIR="${GIT_DIR:-$(git rev-parse --git-dir 2>/dev/null)}"
if [ -z "$REPO_DIR" ]; then
  exit 0
fi

# Derive owner/name from the path: e.g., /data/git/alice/demo.git
# The path is expected to be: <git_root>/<owner>/<name>.git
REPO_DIR="$(cd "$REPO_DIR" 2>/dev/null && pwd)"
GIT_ROOT="${GIT_ROOT:-}"
if [ -z "$GIT_ROOT" ]; then
  # Try to guess git root from the path
  exit 0
fi

REL_PATH="${REPO_DIR#$GIT_ROOT/}"
REL_PATH="${REL_PATH%.git}"
OWNER="${REL_PATH%%/*}"
NAME="${REL_PATH#*/}"

if [ -z "$OWNER" ] || [ -z "$NAME" ]; then
  exit 0
fi

# Read ref updates from stdin and check each one
while read OLD_SHA NEW_SHA REF; do
  case "$REF" in
    refs/heads/*)
      BRANCH="${REF#refs/heads/}"
      # Query the database to check if this branch is protected
      # Use psql if available
      if command -v psql >/dev/null 2>&1; then
        PROTECTED=$(echo "SELECT 1 FROM protected_branches WHERE repo_id=(SELECT id FROM repositories WHERE owner_name='$OWNER' AND name='$NAME') AND branch_name='$BRANCH'" | psql "${DATABASE_URL}" -t -A 2>/dev/null)
        if [ "$PROTECTED" = "1" ]; then
          echo "ERROR: Cannot push to protected branch '$BRANCH' in $OWNER/$NAME" >&2
          exit 1
        fi
      else
        # psql not available - allow the push
        :
      fi
      ;;
  esac
done

exit 0
`

	if err := os.WriteFile(hookPath, []byte(content), 0o755); err != nil {
		return fmt.Errorf("write pre-receive hook: %w", err)
	}
	return nil
}

// RemovePreReceiveHook removes the pre-receive hook from the repo.
func RemovePreReceiveHook(repoPath string) error {
	hookPath := filepath.Join(repoPath, "hooks", "pre-receive")
	if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
