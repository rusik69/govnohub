package audit

import (
	"context"
	"encoding/json"
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
		"audit-user-"+suffix, "audit"+suffix+"@test.local").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRecord(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()
	userID := insertUser(t, pg.Pool, "record")

	t.Run("basic audit entry", func(t *testing.T) {
		err := svc.Record(ctx, userID, "test.action", "test", "123", nil)
		if err != nil {
			t.Fatalf("Record: %v", err)
		}
	})

	t.Run("audit entry with metadata", func(t *testing.T) {
		meta := map[string]string{"key": "value", "extra": "info"}
		err := svc.Record(ctx, userID, "meta.action", "widget", "456", meta)
		if err != nil {
			t.Fatalf("Record with metadata: %v", err)
		}
	})

	t.Run("nil metadata produces null in DB", func(t *testing.T) {
		err := svc.Record(ctx, userID, "nil.meta", "resource", "789", nil)
		if err != nil {
			t.Fatalf("Record with nil metadata: %v", err)
		}
		// Verify by listing
		entries, err := svc.ListFiltered(ctx, ListFilter{Action: "nil.meta"}, 10)
		if err != nil {
			t.Fatalf("ListFiltered: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].Metadata != nil {
			t.Errorf("expected nil metadata, got %s", string(entries[0].Metadata))
		}
	})

	t.Run("empty metadata map", func(t *testing.T) {
		err := svc.Record(ctx, userID, "empty.meta", "res", "101", map[string]string{})
		if err != nil {
			t.Fatalf("Record with empty map: %v", err)
		}
	})

	t.Run("numerical metadata", func(t *testing.T) {
		meta := map[string]int{"count": 42, "score": 100}
		err := svc.Record(ctx, userID, "num.meta", "thing", "202", meta)
		if err != nil {
			t.Fatalf("Record with numerical metadata: %v", err)
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
		entries, err := svc.List(ctx, 10)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected empty list, got %d entries", len(entries))
		}
	})

	// Create 3 audit entries
	for i := 0; i < 3; i++ {
		if err := svc.Record(ctx, userID, "list.action", "test", "id", nil); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("multiple entries", func(t *testing.T) {
		entries, err := svc.List(ctx, 10)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(entries) != 3 {
			t.Errorf("expected 3 entries, got %d", len(entries))
		}
		// Should be newest first
		if len(entries) >= 2 && entries[0].CreatedAt.Before(entries[1].CreatedAt) {
			t.Error("expected newest first order")
		}
	})

	t.Run("limit caps results", func(t *testing.T) {
		entries, err := svc.List(ctx, 2)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
	})

	t.Run("zero limit defaults to 50", func(t *testing.T) {
		entries, err := svc.List(ctx, 0)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		// We only have 3 total entries, so all should be returned
		if len(entries) != 3 {
			t.Errorf("expected 3 entries with default limit, got %d", len(entries))
		}
	})

	t.Run("negative limit defaults to 50", func(t *testing.T) {
		entries, err := svc.List(ctx, -5)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(entries) != 3 {
			t.Errorf("expected 3 entries with negative limit default, got %d", len(entries))
		}
	})
}

func TestListFiltered(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()
	userID := insertUser(t, pg.Pool, "filter")

	// Create entries with different actions and resource types
	if err := svc.Record(ctx, userID, "user.create", "user", "u1", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.Record(ctx, userID, "user.delete", "user", "u2", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.Record(ctx, userID, "repo.create", "repo", "r1", map[string]string{"name": "test-repo"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Record(ctx, userID, "repo.delete", "repo", "r2", nil); err != nil {
		t.Fatal(err)
	}
	// Create entry by another user
	otherID := insertUser(t, pg.Pool, "other")
	if err := svc.Record(ctx, otherID, "user.create", "user", "u3", nil); err != nil {
		t.Fatal(err)
	}

	t.Run("filter by action", func(t *testing.T) {
		entries, err := svc.ListFiltered(ctx, ListFilter{Action: "user.create"}, 10)
		if err != nil {
			t.Fatalf("ListFiltered by action: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries with action=user.create, got %d", len(entries))
		}
		for _, e := range entries {
			if e.Action != "user.create" {
				t.Errorf("unexpected action %q", e.Action)
			}
		}
	})

	t.Run("filter by resource_type", func(t *testing.T) {
		entries, err := svc.ListFiltered(ctx, ListFilter{ResourceType: "repo"}, 10)
		if err != nil {
			t.Fatalf("ListFiltered by resource_type: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries with resource_type=repo, got %d", len(entries))
		}
		for _, e := range entries {
			if e.ResourceType != "repo" {
				t.Errorf("unexpected resource_type %q", e.ResourceType)
			}
		}
	})

	t.Run("filter by actor_id", func(t *testing.T) {
		entries, err := svc.ListFiltered(ctx, ListFilter{ActorID: &otherID}, 10)
		if err != nil {
			t.Fatalf("ListFiltered by actor_id: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry for other user, got %d", len(entries))
		}
		if entries[0].ActorID == nil || *entries[0].ActorID != otherID {
			t.Errorf("unexpected actor_id")
		}
	})

	t.Run("filter by action and resource_type", func(t *testing.T) {
		filter := ListFilter{Action: "user.create", ResourceType: "user"}
		entries, err := svc.ListFiltered(ctx, filter, 10)
		if err != nil {
			t.Fatalf("ListFiltered by action+type: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries with action=user.create+type=user, got %d", len(entries))
		}
	})

	t.Run("filter with no matches", func(t *testing.T) {
		entries, err := svc.ListFiltered(ctx, ListFilter{Action: "nonexistent"}, 10)
		if err != nil {
			t.Fatalf("ListFiltered no match: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("filter with limit", func(t *testing.T) {
		entries, err := svc.ListFiltered(ctx, ListFilter{Action: "user.create"}, 1)
		if err != nil {
			t.Fatalf("ListFiltered with limit: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(entries))
		}
	})

	t.Run("empty filter returns all", func(t *testing.T) {
		entries, err := svc.ListFiltered(ctx, ListFilter{}, 10)
		if err != nil {
			t.Fatalf("ListFiltered empty filter: %v", err)
		}
		if len(entries) != 5 {
			t.Errorf("expected 5 entries, got %d", len(entries))
		}
	})
}

func TestRecord_InvalidMetadata(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()
	userID := insertUser(t, pg.Pool, "badmeta")

	t.Run("chan metadata returns error", func(t *testing.T) {
		// Channels cannot be marshaled to JSON
		err := svc.Record(ctx, userID, "bad.meta", "test", "1", make(chan int))
		if err == nil {
			t.Error("expected error for non-serializable metadata")
		}
	})
}

func TestListFiltered_DefaultLimit(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()
	userID := insertUser(t, pg.Pool, "deflimit")

	// Create one entry
	if err := svc.Record(ctx, userID, "limit.test", "test", "1", nil); err != nil {
		t.Fatal(err)
	}

	t.Run("zero limit defaults to 50", func(t *testing.T) {
		entries, err := svc.ListFiltered(ctx, ListFilter{}, 0)
		if err != nil {
			t.Fatalf("ListFiltered with zero limit: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(entries))
		}
	})

	t.Run("negative limit defaults to 50", func(t *testing.T) {
		entries, err := svc.ListFiltered(ctx, ListFilter{}, -10)
		if err != nil {
			t.Fatalf("ListFiltered with negative limit: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(entries))
		}
	})
}

func TestCRUD_FullLifecycle(t *testing.T) {
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	svc := NewService(pg.Pool)
	ctx := context.Background()
	userID := insertUser(t, pg.Pool, "lifecycle")

	// Create multiple entries
	actions := []string{"create", "update", "delete", "view"}
	for _, a := range actions {
		if err := svc.Record(ctx, userID, a, "widget", "wid-"+a, map[string]string{"action": a}); err != nil {
			t.Fatal(err)
		}
	}

	// List all
	all, err := svc.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(all))
	}

	// Verify each entry has correct fields
	actionSet := make(map[string]bool)
	for _, e := range all {
		actionSet[e.Action] = true
		if e.ResourceType != "widget" {
			t.Errorf("expected resource_type=widget, got %q", e.ResourceType)
		}
		if e.ResourceID == "" {
			t.Error("expected non-empty resource_id")
		}
		if e.ID == uuid.Nil {
			t.Error("expected non-nil ID")
		}
		if e.ActorID == nil || *e.ActorID != userID {
			t.Error("expected actor_id to match")
		}
		// Verify metadata
		var meta map[string]string
		if err := json.Unmarshal(e.Metadata, &meta); err != nil {
			t.Errorf("unmarshal metadata: %v", err)
		} else if meta["action"] != e.Action {
			t.Errorf("metadata.action = %q, want %q", meta["action"], e.Action)
		}
	}
	for _, a := range actions {
		if !actionSet[a] {
			t.Errorf("missing action %q in listing", a)
		}
	}

	// Filter by specific action
	createEntries, err := svc.ListFiltered(ctx, ListFilter{Action: "create"}, 10)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if len(createEntries) != 1 {
		t.Errorf("expected 1 create entry, got %d", len(createEntries))
	}
}
