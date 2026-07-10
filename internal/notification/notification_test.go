package notification

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rusik69/govnohub/internal/testutil"
)

func TestNotificationCRUD(t *testing.T) {
	pg := testutil.NewPostgres(t)
	svc := NewService(pg.Pool)
	ctx := context.Background()

	userID := insertUser(t, pg.Pool)

	if err := svc.Create(ctx, userID, "Hello", "Body text", "/link"); err != nil {
		t.Fatal(err)
	}
	count, err := svc.UnreadCount(ctx, userID)
	if err != nil || count != 1 {
		t.Fatalf("unread=%d err=%v", count, err)
	}
	items, err := svc.List(ctx, userID, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("list=%v err=%v", items, err)
	}
	if items[0].Link != "/link" {
		t.Fatalf("link=%q", items[0].Link)
	}
	if err := svc.MarkRead(ctx, items[0].ID, userID); err != nil {
		t.Fatal(err)
	}
	count, _ = svc.UnreadCount(ctx, userID)
	if count != 0 {
		t.Fatalf("unread after read=%d", count)
	}
}

func insertUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (username, email, password_hash, role)
		VALUES ($1,$2,'hash','user') RETURNING id`, "notify-user", "n@test.local").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
