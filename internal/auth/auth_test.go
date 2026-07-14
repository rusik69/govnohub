package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"
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

	id, name, err := svc.ValidateToken(ctx, token)
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

func TestBootstrapAdminAndRoles(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool, "test-secret")

	if err := svc.BootstrapAdmin(ctx, "admin", "admin@test.local", "adminpass"); err != nil {
		t.Fatal(err)
	}
	if err := svc.BootstrapAdmin(ctx, "other", "other@test.local", "pass"); err != nil {
		t.Fatal("bootstrap should be idempotent")
	}

	ok, err := svc.IsAdmin(ctx, mustLoginUserID(t, svc, "admin"))
	if err != nil || !ok {
		t.Fatalf("admin role: ok=%v err=%v", ok, err)
	}

	_, err = svc.CreateUser(ctx, "bob", "bob@test.local", "pass", RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	users, err := svc.ListUsers(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("users: %v len=%d", err, len(users))
	}
}

func TestDeleteUser(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool, "test-secret")

	// Bootstrap admin and create a regular user
	if err := svc.BootstrapAdmin(ctx, "admin", "admin@test.local", "adminpass"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Register(ctx, "reguser", "reg@test.local", "pass")
	if err != nil {
		t.Fatal(err)
	}
	adminID := mustLoginUserID(t, svc, "admin")

	t.Run("delete regular user", func(t *testing.T) {
		userToDelete, err := svc.Register(ctx, "todelete", "delete@test.local", "pass")
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.DeleteUser(ctx, adminID, userToDelete.ID); err != nil {
			t.Fatalf("DeleteUser: %v", err)
		}
		// Verify user is gone via GetUser
		_, err = svc.GetUser(ctx, userToDelete.ID)
		if err == nil {
			t.Fatal("expected error after deletion")
		}
	})

	t.Run("cannot delete self", func(t *testing.T) {
		err := svc.DeleteUser(ctx, adminID, adminID)
		if err != ErrForbidden {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("cannot delete last admin", func(t *testing.T) {
		err := svc.DeleteUser(ctx, adminID, adminID)
		if err != ErrForbidden {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("cannot delete non-existent user", func(t *testing.T) {
		err := svc.DeleteUser(ctx, adminID, uuid.New())
		if err != ErrUnauthorized {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})
}

func TestRegisterDuplicate(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool, "test-secret")

	_, err := svc.Register(ctx, "dupuser", "dup@test.local", "pass")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Register(ctx, "dupuser", "other@test.local", "pass2")
	if err != ErrUserExists {
		t.Fatalf("expected ErrUserExists, got %v", err)
	}
	// Same email should also be rejected
	_, err = svc.Register(ctx, "other", "dup@test.local", "pass3")
	if err != ErrUserExists {
		t.Fatalf("expected ErrUserExists for duplicate email, got %v", err)
	}
}

func TestValidateTokenInvalid(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	svc := NewService(pg.Pool, "test-secret")
	ctx := context.Background()

	t.Run("empty token", func(t *testing.T) {
		_, _, err := svc.ValidateToken(ctx, "")
		if err != ErrUnauthorized {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("garbage token", func(t *testing.T) {
		_, _, err := svc.ValidateToken(ctx, "not-a-valid-jwt-token")
		if err != ErrUnauthorized {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("expired-like malformed token", func(t *testing.T) {
		_, _, err := svc.ValidateToken(ctx, "eyJhbG...NiJ9.not.valid")
		if err != ErrUnauthorized {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("token signed with different key", func(t *testing.T) {
		// This is a JWT with alg=none, which should be rejected
		_, _, err := svc.ValidateToken(ctx, "eyJhbG...xIn0.")
		if err != ErrUnauthorized {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})
}

func TestRevokePAT(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool, "test-secret")

	u, err := svc.Register(ctx, "patowner", "pat@test.local", "pass")
	if err != nil {
		t.Fatal(err)
	}

	pat, err := svc.CreatePAT(ctx, u.ID, "short-lived", []string{"repo"})
	if err != nil {
		t.Fatal(err)
	}

	// Validate it works before revoke
	_, err = svc.ValidatePAT(ctx, pat)
	if err != nil {
		t.Fatalf("PAT should be valid before revoke: %v", err)
	}

	tokens, err := svc.ListPATs(ctx, u.ID)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("expected 1 PAT, got %d", len(tokens))
	}

	// Revoke
	if err := svc.RevokePAT(ctx, u.ID, tokens[0].ID); err != nil {
		t.Fatalf("RevokePAT: %v", err)
	}

	// Should no longer validate
	_, err = svc.ValidatePAT(ctx, pat)
	if err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized after revoke, got %v", err)
	}

	// List should be empty
	tokens, err = svc.ListPATs(ctx, u.ID)
	if err != nil || len(tokens) != 0 {
		t.Fatalf("expected 0 PATs after revoke, got %d", len(tokens))
	}
}

func TestHasScopeEdgeCases(t *testing.T) {
	t.Run("nil scopes grants everything", func(t *testing.T) {
		if !HasScope(nil, ScopeRepo) {
			t.Fatal("nil scopes should grant access")
		}
		if !HasScope(nil, ScopeRepoWrite) {
			t.Fatal("nil scopes should grant access")
		}
	})

	t.Run("empty scopes grants everything", func(t *testing.T) {
		if !HasScope([]string{}, ScopeRepo) {
			t.Fatal("empty scopes should grant access")
		}
	})

	t.Run("repo scope does not grant repo:write", func(t *testing.T) {
		if HasScope([]string{ScopeRepo}, ScopeRepoWrite) {
			t.Fatal("repo scope should not grant repo:write")
		}
	})

	t.Run("repo:write grants repo", func(t *testing.T) {
		if !HasScope([]string{ScopeRepoWrite}, ScopeRepo) {
			t.Fatal("repo:write should imply repo")
		}
	})

	t.Run("workflow grants repo access", func(t *testing.T) {
		if !HasScope([]string{ScopeWorkflow}, ScopeRepo) {
			t.Fatal("workflow scope should imply repo")
		}
	})

	t.Run("unrelated scope does not match", func(t *testing.T) {
		if HasScope([]string{ScopeReadUser}, ScopeRepo) {
			t.Fatal("read:user should not grant repo")
		}
	})

	t.Run("multiple scopes", func(t *testing.T) {
		if !HasScope([]string{ScopeReadUser, ScopeRepo}, ScopeRepo) {
			t.Fatal("should have repo scope")
		}
		if !HasScope([]string{ScopeReadUser, ScopeWorkflow}, ScopeRepo) {
			t.Fatal("workflow should imply repo")
		}
		if HasScope([]string{ScopeReadUser}, ScopeWorkflow) {
			t.Fatal("read:user should not grant workflow")
		}
	})
}

func TestGetUser(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool, "test-secret")

	u, err := svc.Register(ctx, "getuser", "get@test.local", "pass")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("by id", func(t *testing.T) {
		got, err := svc.GetUser(ctx, u.ID)
		if err != nil {
			t.Fatalf("GetUser: %v", err)
		}
		if got.Username != "getuser" {
			t.Errorf("username = %q, want %q", got.Username, "getuser")
		}
		if got.Email != "get@test.local" {
			t.Errorf("email = %q, want %q", got.Email, "get@test.local")
		}
	})

	t.Run("by username", func(t *testing.T) {
		got, err := svc.GetUserByUsername(ctx, "getuser")
		if err != nil {
			t.Fatalf("GetUserByUsername: %v", err)
		}
		if got.ID != u.ID {
			t.Errorf("id = %v, want %v", got.ID, u.ID)
		}
	})

	t.Run("non-existent id", func(t *testing.T) {
		_, err := svc.GetUser(ctx, uuid.New())
		if err == nil {
			t.Fatal("expected error for non-existent user")
		}
	})

	t.Run("non-existent username", func(t *testing.T) {
		_, err := svc.GetUserByUsername(ctx, "nobody")
		if err != ErrUnauthorized {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})
}

func TestValidatePATWithExpiredToken(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool, "test-secret")

	u, err := svc.Register(ctx, "patuser", "pat@test.local", "pass")
	if err != nil {
		t.Fatal(err)
	}

	// Create a PAT
	pat, err := svc.CreatePAT(ctx, u.ID, "test", []string{"repo"})
	if err != nil {
		t.Fatal(err)
	}

	// Without ghp_ prefix should still work (code strips it internally)
	t.Run("without prefix still works", func(t *testing.T) {
		withoutPrefix := pat[4:] // strip "ghp_"
		_, scopes, err := svc.ValidatePATWithScopes(ctx, withoutPrefix)
		if err != nil {
			t.Fatalf("expected valid without prefix, got: %v", err)
		}
		if !HasScope(scopes, ScopeRepo) {
			t.Fatal("expected repo scope")
		}
	})

	// Validate with correct prefix
	t.Run("with prefix", func(t *testing.T) {
		_, scopes, err := svc.ValidatePATWithScopes(ctx, pat)
		if err != nil {
			t.Fatalf("ValidatePATWithScopes: %v", err)
		}
		if !HasScope(scopes, ScopeRepo) {
			t.Fatal("expected repo scope")
		}
	})

	// Revoke and validate fails
	t.Run("after revoke", func(t *testing.T) {
		pats, _ := svc.ListPATs(ctx, u.ID)
		svc.RevokePAT(ctx, u.ID, pats[0].ID)
		_, _, err := svc.ValidatePATWithScopes(ctx, pat)
		if err != ErrUnauthorized {
			t.Fatalf("expected ErrUnauthorized after revoke, got %v", err)
		}
	})
}

func TestIntrospectPAT(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool, "test-secret")

	u, err := svc.Register(ctx, "introspectuser", "introspect@test.local", "pass")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("valid token", func(t *testing.T) {
		pat, err := svc.CreatePAT(ctx, u.ID, "my-ci-token", []string{"repo", "workflow"})
		if err != nil {
			t.Fatal(err)
		}

		info, err := svc.IntrospectPAT(ctx, pat)
		if err != nil {
			t.Fatalf("IntrospectPAT: %v", err)
		}
		if !info.Active {
			t.Fatal("token should be active")
		}
		if info.UserID != u.ID {
			t.Fatalf("user_id = %v, want %v", info.UserID, u.ID)
		}
		if info.Username != "introspectuser" {
			t.Fatalf("username = %q, want %q", info.Username, "introspectuser")
		}
		if info.Name != "my-ci-token" {
			t.Fatalf("name = %q, want %q", info.Name, "my-ci-token")
		}
		if len(info.Scopes) != 2 || info.Scopes[0] != "repo" || info.Scopes[1] != "workflow" {
			t.Fatalf("scopes = %v, want [repo workflow]", info.Scopes)
		}
		if info.TokenID == uuid.Nil {
			t.Fatal("token_id should not be nil")
		}
		if info.CreatedAt.IsZero() {
			t.Fatal("created_at should not be zero")
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		info, err := svc.IntrospectPAT(ctx, "ghp_invalidtoken1234567890abcdef")
		if err != nil {
			t.Fatalf("IntrospectPAT should not error on invalid token: %v", err)
		}
		if info.Active {
			t.Fatal("invalid token should not be active")
		}
	})

	t.Run("revoked token", func(t *testing.T) {
		pat, err := svc.CreatePAT(ctx, u.ID, "revoke-me", []string{"repo"})
		if err != nil {
			t.Fatal(err)
		}

		pats, err := svc.ListPATs(ctx, u.ID)
		if err != nil || len(pats) == 0 {
			t.Fatal("no PATs found")
		}
		svc.RevokePAT(ctx, u.ID, pats[0].ID)

		info, err := svc.IntrospectPAT(ctx, pat)
		if err != nil {
			t.Fatalf("IntrospectPAT should not error on revoked token: %v", err)
		}
		if info.Active {
			t.Fatal("revoked token should not be active")
		}
	})

	t.Run("no prefix still works", func(t *testing.T) {
		pat, err := svc.CreatePAT(ctx, u.ID, "noprefix", []string{"repo"})
		if err != nil {
			t.Fatal(err)
		}
		withoutPrefix := pat[4:] // strip "ghp_"

		info, err := svc.IntrospectPAT(ctx, withoutPrefix)
		if err != nil {
			t.Fatalf("IntrospectPAT without prefix: %v", err)
		}
		if !info.Active {
			t.Fatal("token without prefix should be active")
		}
	})
}

func mustLoginUserID(t *testing.T, svc *Service, username string) uuid.UUID {
	t.Helper()
	_, u, err := svc.Login(context.Background(), username, "adminpass")
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}
