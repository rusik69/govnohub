package auth

import (
	"context"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
)

func TestRegisterLoginPAT(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool, "test-secret")

	u, err := svc.Register(ctx, "alice", "alice@test.local", "password123")
	if err != nil {
		t.Fatal(err)
	}

	token, user, err := svc.Login(ctx, "alice", "password123")
	if err != nil || token == "" || user.ID != u.ID {
		t.Fatalf("login failed: %v", err)
	}

	id, name, err := svc.ValidateToken(token)
	if err != nil || id != u.ID || name != "alice" {
		t.Fatalf("validate token: %v", err)
	}

	pat, err := svc.CreatePAT(ctx, u.ID, "ci", []string{"repo"})
	if err != nil || pat == "" {
		t.Fatalf("pat: %v", err)
	}
	patUser, err := svc.ValidatePAT(ctx, pat)
	if err != nil || patUser != u.ID {
		t.Fatalf("validate pat: %v", err)
	}
}

func TestPATScopes(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool, "test-secret")

	u, _ := svc.Register(ctx, "scopeuser", "scope@test.local", "pass")
	readPAT, _ := svc.CreatePAT(ctx, u.ID, "read-only", []string{"repo"})
	writePAT, _ := svc.CreatePAT(ctx, u.ID, "write", []string{"repo:write"})

	_, scopes, err := svc.ValidatePATWithScopes(ctx, readPAT)
	if err != nil || !HasScope(scopes, ScopeRepo) || HasScope(scopes, ScopeRepoWrite) {
		t.Fatalf("read PAT scopes: %v %v", scopes, err)
	}
	_, scopes, err = svc.ValidatePATWithScopes(ctx, writePAT)
	if err != nil || !HasScope(scopes, ScopeRepoWrite) {
		t.Fatalf("write PAT scopes: %v %v", scopes, err)
	}

	emptyPAT, _ := svc.CreatePAT(ctx, u.ID, "legacy", []string{})
	_, scopes, err = svc.ValidatePATWithScopes(ctx, emptyPAT)
	if err != nil || !HasScope(scopes, ScopeRepoWrite) {
		t.Fatal("empty scopes should grant full access for backward compat")
	}

	tokens, err := svc.ListPATs(ctx, u.ID)
	if err != nil || len(tokens) < 3 {
		t.Fatalf("list pats: %v len=%d", err, len(tokens))
	}
}

func TestLoginInvalid(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	svc := NewService(pg.Pool, "test-secret")
	_, _, err := svc.Login(context.Background(), "nobody", "wrong")
	if err != ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
}
