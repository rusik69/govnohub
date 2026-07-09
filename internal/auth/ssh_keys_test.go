package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"golang.org/x/crypto/ssh"
)

func testSSHPublicKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return string(ssh.MarshalAuthorizedKey(pub))
}

func TestSSHKeys(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := t.Context()
	svc := NewService(pg.Pool, "test-secret")

	u, err := svc.Register(ctx, "sshuser", "ssh@test.local", "password123")
	if err != nil {
		t.Fatal(err)
	}

	pub := testSSHPublicKey(t)
	key, err := svc.AddSSHKey(ctx, u.ID, "laptop", pub)
	if err != nil {
		t.Fatal(err)
	}
	if key.Fingerprint == "" {
		t.Fatal("expected fingerprint")
	}

	keys, err := svc.ListSSHKeys(ctx, u.ID)
	if err != nil || len(keys) != 1 {
		t.Fatalf("list keys: %v len=%d", err, len(keys))
	}

	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pub))
	if err != nil {
		t.Fatal(err)
	}
	id, name, err := svc.LookupUserBySSHPublicKey(ctx, parsed)
	if err != nil || id != u.ID || name != "sshuser" {
		t.Fatalf("lookup: id=%v name=%s err=%v", id, name, err)
	}

	if err := svc.DeleteSSHKey(ctx, u.ID, key.ID); err != nil {
		t.Fatal(err)
	}
	keys, err = svc.ListSSHKeys(ctx, u.ID)
	if err != nil || len(keys) != 0 {
		t.Fatalf("after delete: %v len=%d", err, len(keys))
	}
}

func TestSSHKeyDuplicate(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := t.Context()
	svc := NewService(pg.Pool, "test-secret")

	u1, _ := svc.Register(ctx, "user1", "u1@test.local", "pass")
	u2, _ := svc.Register(ctx, "user2", "u2@test.local", "pass")
	pub := testSSHPublicKey(t)

	if _, err := svc.AddSSHKey(ctx, u1.ID, "key1", pub); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddSSHKey(ctx, u2.ID, "key2", pub); err != ErrDuplicateSSHKey {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}
