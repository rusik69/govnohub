package wiki

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/testutil"
)

func setupWikiTest(t *testing.T) (context.Context, *Service, uuid.UUID, uuid.UUID) {
	t.Helper()
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	wikiSvc := NewService(pg.Pool)

	u, err := authSvc.Register(ctx, "wikiuser", "wiki@test.local", "pass")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	r, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "wiki-repo", "", false)
	if err != nil {
		t.Fatalf("Create repo: %v", err)
	}
	return ctx, wikiSvc, r.ID, u.ID
}

// ---- Slugify tests ----

func TestSlugify(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"  Spaces   Everywhere  ", "spaces---everywhere"},
		{"UPPERCASE", "uppercase"},
		{"Special!@#$%^&*()Chars", "specialchars"},
		{"already-slugified", "already-slugified"},
		{"Café & Crème", "caf--crme"},
		{"", ""},
		{"---dashes---", "---dashes---"},
		{"Hello.World.Test", "helloworldtest"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := Slugify(tt.title)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

// ---- Upsert tests ----

func TestUpsert(t *testing.T) {
	ctx, svc, repoID, authorID := setupWikiTest(t)

	t.Run("create basic page", func(t *testing.T) {
		p, err := svc.Upsert(ctx, repoID, authorID, "", "My Page", "Content here")
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}
		if p.Title != "My Page" {
			t.Errorf("Title = %q, want %q", p.Title, "My Page")
		}
		if p.Content != "Content here" {
			t.Errorf("Content = %q, want %q", p.Content, "Content here")
		}
		if p.Slug != "my-page" {
			t.Errorf("Slug = %q, want %q", p.Slug, "my-page")
		}
		if p.RepoID != repoID {
			t.Error("RepoID mismatch")
		}
		if p.AuthorID != authorID {
			t.Error("AuthorID mismatch")
		}
		if p.ID == uuid.Nil {
			t.Error("expected non-nil ID")
		}
		if p.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt")
		}
		if p.UpdatedAt.IsZero() {
			t.Error("expected non-zero UpdatedAt")
		}
	})

	t.Run("create with explicit slug", func(t *testing.T) {
		p, err := svc.Upsert(ctx, repoID, authorID, "explicit-slug", "Explicit", "Body")
		if err != nil {
			t.Fatalf("Upsert with slug: %v", err)
		}
		if p.Slug != "explicit-slug" {
			t.Errorf("Slug = %q, want %q", p.Slug, "explicit-slug")
		}
	})

	t.Run("upsert updates existing page", func(t *testing.T) {
		// Create
		p1, err := svc.Upsert(ctx, repoID, authorID, "", "Update Test", "Original")
		if err != nil {
			t.Fatalf("Upsert create: %v", err)
		}

		// Update
		p2, err := svc.Upsert(ctx, repoID, authorID, p1.Slug, "Update Test Updated", "Updated content")
		if err != nil {
			t.Fatalf("Upsert update: %v", err)
		}
		if p2.Title != "Update Test Updated" {
			t.Errorf("Title after update = %q, want %q", p2.Title, "Update Test Updated")
		}
		if p2.Content != "Updated content" {
			t.Errorf("Content after update = %q, want %q", p2.Content, "Updated content")
		}
		if p2.ID != p1.ID {
			t.Error("expected same ID after upsert")
		}
	})

	t.Run("upsert empty content", func(t *testing.T) {
		p, err := svc.Upsert(ctx, repoID, authorID, "", "Empty Content", "")
		if err != nil {
			t.Fatalf("Upsert empty content: %v", err)
		}
		if p.Content != "" {
			t.Errorf("Content = %q, want empty", p.Content)
		}
	})
}

// ---- Get tests ----

func TestGet(t *testing.T) {
	ctx, svc, repoID, authorID := setupWikiTest(t)

	// Create a page to get
	_, err := svc.Upsert(ctx, repoID, authorID, "", "Get Test", "Get content")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	t.Run("get existing page", func(t *testing.T) {
		p, err := svc.Get(ctx, repoID, "get-test")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if p.Title != "Get Test" {
			t.Errorf("Title = %q", p.Title)
		}
		if p.Content != "Get content" {
			t.Errorf("Content = %q", p.Content)
		}
		if p.Author == "" {
			t.Error("expected non-empty Author (from join)")
		}
	})

	t.Run("get nonexistent slug", func(t *testing.T) {
		_, err := svc.Get(ctx, repoID, "nonexistent")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("get from different repo", func(t *testing.T) {
		otherRepoID := uuid.New()
		_, err := svc.Get(ctx, otherRepoID, "get-test")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound for different repo, got %v", err)
		}
	})
}

// ---- List tests ----

func TestList(t *testing.T) {
	ctx, svc, repoID, authorID := setupWikiTest(t)

	t.Run("empty list", func(t *testing.T) {
		pages, err := svc.List(ctx, repoID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(pages) != 0 {
			t.Errorf("expected empty list, got %d pages", len(pages))
		}
	})

	// Create 3 pages
	for _, title := range []string{"Alpha", "Beta", "Gamma"} {
		if _, err := svc.Upsert(ctx, repoID, authorID, "", title, "Content for "+title); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("multiple pages", func(t *testing.T) {
		pages, err := svc.List(ctx, repoID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(pages) != 3 {
			t.Errorf("expected 3 pages, got %d", len(pages))
		}
	})

	t.Run("alphabetical order", func(t *testing.T) {
		pages, _ := svc.List(ctx, repoID)
		if len(pages) >= 2 {
			if pages[0].Title > pages[1].Title {
				t.Error("expected alphabetical order by title")
			}
		}
	})

	t.Run("different repo has empty list", func(t *testing.T) {
		pages, err := svc.List(ctx, uuid.New())
		if err != nil {
			t.Fatalf("List on different repo: %v", err)
		}
		if len(pages) != 0 {
			t.Errorf("expected 0 pages for different repo, got %d", len(pages))
		}
	})
}

// ---- Delete tests ----

func TestDelete(t *testing.T) {
	ctx, svc, repoID, authorID := setupWikiTest(t)

	// Create a page to delete
	_, err := svc.Upsert(ctx, repoID, authorID, "", "Delete Test", "Will be deleted")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	t.Run("delete existing page", func(t *testing.T) {
		err := svc.Delete(ctx, repoID, "delete-test")
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}

		// Verify it's gone
		_, err = svc.Get(ctx, repoID, "delete-test")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound after delete, got %v", err)
		}
	})

	t.Run("delete nonexistent slug", func(t *testing.T) {
		err := svc.Delete(ctx, repoID, "nonexistent")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("delete from different repo", func(t *testing.T) {
		err := svc.Delete(ctx, uuid.New(), "delete-test")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound for different repo, got %v", err)
		}
	})
}

// ---- Full lifecycle test ----

func TestFullLifecycle(t *testing.T) {
	ctx, svc, repoID, authorID := setupWikiTest(t)

	// Create a page
	p, err := svc.Upsert(ctx, repoID, authorID, "", "Home", "Welcome to the wiki")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if p.Slug != "home" {
		t.Errorf("Slug = %q", p.Slug)
	}

	// Verify with Get
	got, err := svc.Get(ctx, repoID, "home")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "Welcome to the wiki" {
		t.Errorf("Content = %q", got.Content)
	}

	// Update
	p2, err := svc.Upsert(ctx, repoID, authorID, "home", "Home", "Updated welcome page")
	if err != nil {
		t.Fatalf("Upsert update: %v", err)
	}
	if p2.Content != "Updated welcome page" {
		t.Errorf("Content after update = %q", p2.Content)
	}

	// List should have 1 page
	pages, err := svc.List(ctx, repoID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pages) != 1 {
		t.Errorf("expected 1 page, got %d", len(pages))
	}

	// Delete
	if err := svc.Delete(ctx, repoID, "home"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// List should be empty
	pages, err = svc.List(ctx, repoID)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages after delete, got %d", len(pages))
	}
}
