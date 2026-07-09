package testutil

import (
	"context"
	"testing"
)

func TestMigrate(t *testing.T) {
	pg := NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	var n int
	if err := pg.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 10 {
		t.Fatalf("expected tables, got %d", n)
	}
}
