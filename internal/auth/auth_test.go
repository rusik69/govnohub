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

func TestLoginInvalid(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	svc := NewService(pg.Pool, "test-secret")
	_, _, err := svc.Login(context.Background(), "nobody", "wrong")
	if err != ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
}
