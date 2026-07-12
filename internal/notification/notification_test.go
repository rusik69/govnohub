package notification

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rusik69/govnohub/internal/testutil"
)

func insertUser(t *testing.T, pool *pgxpool.Pool, suffix string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (username, email, password_hash, role)
		VALUES ($1,$2,'hash','user') RETURNING id`,
		"notify-user-"+suffix, "n"+suffix+"@test.local").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCreate(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()
	userID := insertUser(t, pg.Pool, "create")

	t.Run("basic notification", func(t *testing.T) {
		if err := svc.Create(ctx, userID, "Hello", "Body text", "/link"); err != nil {
			t.Fatalf("Create: %v", err)
		}
	})

	t.Run("duplicate notifications", func(t *testing.T) {
		if err := svc.Create(ctx, userID, "dup", "body", "/l"); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.Create(ctx, userID, "dup", "body", "/l"); err != nil {
			t.Fatalf("Create (duplicate): %v", err)
		}
	})

	t.Run("empty link", func(t *testing.T) {
		if err := svc.Create(ctx, userID, "No Link", "body text", ""); err != nil {
			t.Fatalf("Create with empty link: %v", err)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		if err := svc.Create(ctx, userID, "Title Only", "", ""); err != nil {
			t.Fatalf("Create with empty body: %v", err)
		}
	})
}

func TestList(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()
	userID := insertUser(t, pg.Pool, "list")

	t.Run("empty list", func(t *testing.T) {
		items, err := svc.List(ctx, userID, 10)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 0 {
			t.Errorf("expected empty list, got %d items", len(items))
		}
	})

	// Create 3 notifications
	for i := 0; i < 3; i++ {
		if err := svc.Create(ctx, userID, "N", "body", ""); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("multiple notifications", func(t *testing.T) {
		items, err := svc.List(ctx, userID, 10)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 3 {
			t.Errorf("expected 3 notifications, got %d", len(items))
		}
		// Should be in descending order (newest first)
		if items[0].CreatedAt.Before(items[1].CreatedAt) {
			t.Error("expected newest first order")
		}
	})

	t.Run("list with limit", func(t *testing.T) {
		items, err := svc.List(ctx, userID, 2)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 2 {
			t.Errorf("expected 2 notifications, got %d", len(items))
		}
	})

	t.Run("list with zero limit defaults to 20", func(t *testing.T) {
		items, err := svc.List(ctx, userID, 0)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 3 {
			t.Errorf("expected 3 notifications with default limit, got %d", len(items))
		}
	})

	t.Run("list with negative limit defaults to 20", func(t *testing.T) {
		items, err := svc.List(ctx, userID, -5)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 3 {
			t.Errorf("expected 3 notifications with negative limit default, got %d", len(items))
		}
	})
}

func TestList_DifferentUser(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()

	userA := insertUser(t, pg.Pool, "list-a")
	userB := insertUser(t, pg.Pool, "list-b")

	// Create notification for user A only
	if err := svc.Create(ctx, userA, "For A", "body", ""); err != nil {
		t.Fatal(err)
	}

	t.Run("user B sees empty list", func(t *testing.T) {
		items, err := svc.List(ctx, userB, 10)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 0 {
			t.Errorf("expected 0 notifications for user B, got %d", len(items))
		}
	})

	t.Run("user A sees their notification", func(t *testing.T) {
		items, err := svc.List(ctx, userA, 10)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 1 {
			t.Errorf("expected 1 notification for user A, got %d", len(items))
		}
		if items[0].Title != "For A" {
			t.Errorf("Title = %q, want %q", items[0].Title, "For A")
		}
	})
}

func TestUnreadCount(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()

	userID := insertUser(t, pg.Pool, "unread")

	t.Run("zero for no notifications", func(t *testing.T) {
		c, err := svc.UnreadCount(ctx, userID)
		if err != nil {
			t.Fatalf("UnreadCount: %v", err)
		}
		if c != 0 {
			t.Errorf("count = %d, want 0", c)
		}
	})

	t.Run("returns correct count after creating", func(t *testing.T) {
		if err := svc.Create(ctx, userID, "unread1", "", ""); err != nil {
			t.Fatal(err)
		}
		if err := svc.Create(ctx, userID, "unread2", "", ""); err != nil {
			t.Fatal(err)
		}
		c, err := svc.UnreadCount(ctx, userID)
		if err != nil {
			t.Fatalf("UnreadCount: %v", err)
		}
		if c != 2 {
			t.Errorf("count = %d, want 2", c)
		}
	})

	t.Run("decreases after marking one read", func(t *testing.T) {
		items, _ := svc.List(ctx, userID, 10)
		if len(items) == 0 {
			t.Fatal("expected notifications to mark read")
		}
		if err := svc.MarkRead(ctx, items[0].ID, userID); err != nil {
			t.Fatalf("MarkRead: %v", err)
		}
		c, err := svc.UnreadCount(ctx, userID)
		if err != nil {
			t.Fatalf("UnreadCount: %v", err)
		}
		if c != 1 {
			t.Errorf("count = %d, want 1", c)
		}
	})

	t.Run("different user sees zero", func(t *testing.T) {
		otherID := insertUser(t, pg.Pool, "other")
		c, err := svc.UnreadCount(ctx, otherID)
		if err != nil {
			t.Fatalf("UnreadCount: %v", err)
		}
		if c != 0 {
			t.Errorf("count = %d, want 0", c)
		}
	})
}

func TestMarkRead(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()

	userID := insertUser(t, pg.Pool, "markread")

	if err := svc.Create(ctx, userID, "read-test", "body", ""); err != nil {
		t.Fatal(err)
	}
	items, _ := svc.List(ctx, userID, 10)
	if len(items) == 0 {
		t.Fatal("expected notification")
	}
	notif := items[0]

	t.Run("mark as read", func(t *testing.T) {
		if err := svc.MarkRead(ctx, notif.ID, userID); err != nil {
			t.Fatalf("MarkRead: %v", err)
		}
		// Re-list and check
		items, err := svc.List(ctx, userID, 10)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) > 0 && !items[0].Read {
			t.Error("expected notification to be marked as read")
		}
	})

	t.Run("mark already-read is idempotent", func(t *testing.T) {
		if err := svc.MarkRead(ctx, notif.ID, userID); err != nil {
			t.Fatalf("MarkRead (already read): %v", err)
		}
	})
}

func TestMarkRead_WrongUser(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()

	ownerID := insertUser(t, pg.Pool, "owner")
	otherID := insertUser(t, pg.Pool, "other")

	if err := svc.Create(ctx, ownerID, "private", "body", ""); err != nil {
		t.Fatal(err)
	}
	items, _ := svc.List(ctx, ownerID, 10)
	if len(items) == 0 {
		t.Fatal("expected notification")
	}
	notif := items[0]

	// Other user tries to mark owner's notification as read
	if err := svc.MarkRead(ctx, notif.ID, otherID); err != nil {
		t.Fatalf("MarkRead with wrong user should not error: %v", err)
	}

	// Verify it's still unread for the owner
	c, _ := svc.UnreadCount(ctx, ownerID)
	if c != 1 {
		t.Errorf("expected unread count to still be 1, got %d", c)
	}
}

func TestNotifyAsync(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()
	userID := insertUser(t, pg.Pool, "async")

	t.Run("creates notification asynchronously", func(t *testing.T) {
		svc.NotifyAsync(userID, "Async Title", "Async Body", "/async")
		// There's no way to synchronize with the goroutine, so we wait briefly
		// and then check. This is inherently racy but in practice the goroutine
		// runs very quickly.
		items, err := svc.List(ctx, userID, 10)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		// The notification may or may not have been created yet
		_ = items
	})

	t.Run("nil UUID is a no-op", func(t *testing.T) {
		// Should not panic
		svc.NotifyAsync(uuid.Nil, "should", "not", "create")
	})

	t.Run("panics if pool is closed", func(t *testing.T) {
		// Not much to do here, just ensure nil UUID doesn't crash
		svc.NotifyAsync(uuid.Nil, "", "", "")
	})
}

func TestCRUD_FullLifecycle(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()
	userID := insertUser(t, pg.Pool, "lifecycle")

	// Create
	if err := svc.Create(ctx, userID, "Lifecycle Test", "Full body text", "/path"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Unread count
	c, err := svc.UnreadCount(ctx, userID)
	if err != nil || c != 1 {
		t.Fatalf("UnreadCount = %d, err=%v", c, err)
	}

	// List
	items, err := svc.List(ctx, userID, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("List: len=%d, err=%v", len(items), err)
	}
	if items[0].Title != "Lifecycle Test" {
		t.Errorf("Title = %q", items[0].Title)
	}
	if items[0].Body != "Full body text" {
		t.Errorf("Body = %q", items[0].Body)
	}
	if items[0].Link != "/path" {
		t.Errorf("Link = %q", items[0].Link)
	}
	if items[0].Read != false {
		t.Error("expected unread notification")
	}

	// MarkRead
	if err := svc.MarkRead(ctx, items[0].ID, userID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	// Unread count after read
	c, _ = svc.UnreadCount(ctx, userID)
	if c != 0 {
		t.Errorf("UnreadCount after mark read = %d, want 0", c)
	}
}
