#!/usr/bin/env bash
set -euo pipefail
BASE="${BASE:-http://govnohub.local}"
PASS=0
FAIL=0

check() {
  local name="$1" code="$2" expect="$3"
  if [ "$code" = "$expect" ]; then
    echo "✓ $name ($code)"
    PASS=$((PASS+1))
  else
    echo "✗ $name (got $code, want $expect)"
    FAIL=$((FAIL+1))
  fi
}

login() {
  curl -s -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$1\",\"password\":\"$2\"}" | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])"
}

api() {
  local method="$1" path="$2" token="$3" data="${4:-}"
  if [ -n "$data" ]; then
    curl -s -o /tmp/govnohub_resp.json -w "%{http_code}" -X "$method" "$BASE$path" \
      -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "$data"
  else
    curl -s -o /tmp/govnohub_resp.json -w "%{http_code}" -X "$method" "$BASE$path" \
      -H "Authorization: Bearer $token"
  fi
}

echo "==> Auth"
ADMIN_TOKEN=$(login admin admin)
DEV_TOKEN=$(login devuser dev123)
check "admin login" 200 200
check "devuser login" 200 200

echo "==> User & admin"
check "GET /user" "$(api GET /api/v1/user "$DEV_TOKEN")" 200
check "GET /admin/users (admin)" "$(api GET /api/v1/admin/users "$ADMIN_TOKEN")" 200
check "GET /admin/users (deny)" "$(api GET /api/v1/admin/users "$DEV_TOKEN")" 403

echo "==> Projects"
check "list repos" "$(api GET /api/v1/user/repos "$DEV_TOKEN")" 200
REPO_NAME="curl-test-$(date +%s)"
CODE=$(api POST /api/v1/users/devuser/repos "$DEV_TOKEN" "{\"name\":\"$REPO_NAME\",\"description\":\"api test\",\"private\":false}")
check "create project" "$CODE" 200
REPO=$(python3 -c "import json; print(json.load(open('/tmp/govnohub_resp.json'))['name'])")

echo "==> Repo features ($REPO)"
check "get repo" "$(api GET /api/v1/repos/devuser/$REPO "$DEV_TOKEN")" 200
check "list issues" "$(api GET /api/v1/repos/devuser/$REPO/issues "$DEV_TOKEN")" 200
CODE=$(api POST /api/v1/repos/devuser/$REPO/issues "$DEV_TOKEN" '{"title":"Test issue","body":"body"}')
check "create issue" "$CODE" 200
ISSUE=$(python3 -c "import json; print(json.load(open('/tmp/govnohub_resp.json'))['number'])")
check "get issue" "$(api GET /api/v1/repos/devuser/$REPO/issues/$ISSUE "$DEV_TOKEN")" 200
check "comment issue" "$(api POST /api/v1/repos/devuser/$REPO/issues/$ISSUE/comments "$DEV_TOKEN" '{"body":"hello"}')" 200
check "list comments" "$(api GET /api/v1/repos/devuser/$REPO/issues/$ISSUE/comments "$DEV_TOKEN")" 200
check "create label" "$(api POST /api/v1/repos/devuser/$REPO/labels "$DEV_TOKEN" '{"name":"bug","color":"#ff0000"}')" 200
check "list labels" "$(api GET /api/v1/repos/devuser/$REPO/labels "$DEV_TOKEN")" 200
check "star" "$(api POST /api/v1/repos/devuser/$REPO/star "$DEV_TOKEN")" 200
check "watch" "$(api POST /api/v1/repos/devuser/$REPO/watch "$DEV_TOKEN")" 200
check "create branch" "$(api POST /api/v1/repos/devuser/$REPO/branches "$DEV_TOKEN" '{"name":"feature","base":"main"}')" 200
check "protect branch" "$(api POST /api/v1/repos/devuser/$REPO/protected-branches "$DEV_TOKEN" '{"branch":"main","required_checks":[],"require_reviews":0}')" 200
check "list protected" "$(api GET /api/v1/repos/devuser/$REPO/protected-branches "$DEV_TOKEN")" 200
check "add webhook" "$(api POST /api/v1/repos/devuser/$REPO/webhooks "$DEV_TOKEN" '{"url":"http://example.com/hook","secret":"s","events":["push"]}')" 200
check "list webhooks" "$(api GET /api/v1/repos/devuser/$REPO/webhooks "$DEV_TOKEN")" 200
check "create release" "$(api POST /api/v1/repos/devuser/$REPO/releases "$DEV_TOKEN" '{"tag_name":"v0.1.0","name":"v0.1","body":"notes"}')" 200
check "list releases" "$(api GET /api/v1/repos/devuser/$REPO/releases "$DEV_TOKEN")" 200
check "publish package" "$(curl -s -o /tmp/govnohub_resp.json -w "%{http_code}" -X POST "$BASE/api/v1/repos/devuser/$REPO/packages?name=demo&version=1.0.0&type=generic" -H "Authorization: Bearer $DEV_TOKEN" -H 'Content-Type: text/plain' -d 'pkg-data')" 200
check "list packages" "$(api GET /api/v1/repos/devuser/$REPO/packages "$DEV_TOKEN")" 200
check "upsert workflow" "$(api POST /api/v1/repos/devuser/$REPO/actions/workflows "$DEV_TOKEN" '{"name":"CI","path":".govnohub/workflows/ci.yaml","content":"name: CI\non: [push]\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"}')" 200
check "list workflows" "$(api GET /api/v1/repos/devuser/$REPO/actions/workflows "$DEV_TOKEN")" 200

echo "==> Search & tokens"
check "search" "$(api GET "/api/v1/search?q=Test" "$DEV_TOKEN")" 200
check "create PAT" "$(api POST /api/v1/user/tokens "$DEV_TOKEN" '{"name":"cli","scopes":["repo"]}')" 200
check "list PATs" "$(api GET /api/v1/user/tokens "$DEV_TOKEN")" 200

echo "==> UI"
check "web /" "$(curl -s -o /dev/null -w "%{http_code}" "$BASE/")" 200
check "web /login" "$(curl -s -o /dev/null -w "%{http_code}" "$BASE/login")" 200
check "web /search" "$(curl -s -o /dev/null -w "%{http_code}" -H "Host: govnohub.local" "$BASE/search")" 200

echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
