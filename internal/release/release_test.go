package release

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/testutil"
)

func setupReleaseTest(t *testing.T) (context.Context, *Service, *repo.Repository, uuid.UUID) {
	t.Helper()
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	releaseSvc := NewService(pg.Pool, t.TempDir())

	u, err := authSvc.Register(ctx, "reluser", "rel@test.local", "pass")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	r, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "releases", "", false)
	if err != nil {
		t.Fatalf("Create repo: %v", err)
	}
	return ctx, releaseSvc, r, u.ID
}

func TestCreate(t *testing.T) {
	ctx, svc, r, authorID := setupReleaseTest(t)

	t.Run("basic release", func(t *testing.T) {
		rel, err := svc.Create(ctx, r.ID, authorID, "v1.0.0", "First", "Release notes", false, false)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if rel.TagName != "v1.0.0" {
			t.Errorf("TagName = %q, want %q", rel.TagName, "v1.0.0")
		}
		if rel.Name != "First" {
			t.Errorf("Name = %q, want %q", rel.Name, "First")
		}
		if rel.Body != "Release notes" {
			t.Errorf("Body = %q, want %q", rel.Body, "Release notes")
		}
		if rel.Draft != false {
			t.Error("expected non-draft release")
		}
		if rel.Prerelease != false {
			t.Error("expected non-prerelease release")
		}
		if rel.RepoID != r.ID {
			t.Error("RepoID mismatch")
		}
		if rel.AuthorID != authorID {
			t.Error("AuthorID mismatch")
		}
		if rel.ID == uuid.Nil {
			t.Error("expected non-nil ID")
		}
		if rel.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt")
		}
	})

	t.Run("draft release", func(t *testing.T) {
		rel, err := svc.Create(ctx, r.ID, authorID, "v1.1.0-draft", "Draft", "", true, false)
		if err != nil {
			t.Fatalf("Create draft: %v", err)
		}
		if !rel.Draft {
			t.Error("expected draft release")
		}
	})

	t.Run("prerelease", func(t *testing.T) {
		rel, err := svc.Create(ctx, r.ID, authorID, "v2.0.0-rc1", "RC1", "Testing", false, true)
		if err != nil {
			t.Fatalf("Create prerelease: %v", err)
		}
		if !rel.Prerelease {
			t.Error("expected prerelease")
		}
	})

	t.Run("empty name and body", func(t *testing.T) {
		rel, err := svc.Create(ctx, r.ID, authorID, "v3.0.0", "", "", false, false)
		if err != nil {
			t.Fatalf("Create with empty fields: %v", err)
		}
		if rel.Name != "" {
			t.Errorf("Name = %q, want empty", rel.Name)
		}
		if rel.Body != "" {
			t.Errorf("Body = %q, want empty", rel.Body)
		}
	})

	t.Run("duplicate tag name", func(t *testing.T) {
		_, err := svc.Create(ctx, r.ID, authorID, "v1.0.0", "Duplicate", "", false, false)
		if err == nil {
			t.Error("expected error for duplicate tag name")
		}
	})
}

func TestList(t *testing.T) {
	ctx, svc, r, authorID := setupReleaseTest(t)

	t.Run("empty list", func(t *testing.T) {
		releases, err := svc.List(ctx, r.ID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(releases) != 0 {
			t.Errorf("expected empty list, got %d releases", len(releases))
		}
	})

	// Create 3 releases
	for i := 0; i < 3; i++ {
		tag := "v0.1." + string(rune('0'+i))
		if _, err := svc.Create(ctx, r.ID, authorID, tag, "Release "+tag, "", false, false); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("multiple releases", func(t *testing.T) {
		releases, err := svc.List(ctx, r.ID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(releases) != 3 {
			t.Errorf("expected 3 releases, got %d", len(releases))
		}
	})

	t.Run("newest first order", func(t *testing.T) {
		releases, _ := svc.List(ctx, r.ID)
		if len(releases) >= 2 {
			if releases[0].CreatedAt.Before(releases[1].CreatedAt) {
				t.Error("expected newest first order (DESC)")
			}
		}
	})

	t.Run("different repo has empty list", func(t *testing.T) {
		otherRepoID := uuid.New()
		releases, err := svc.List(ctx, otherRepoID)
		if err != nil {
			t.Fatalf("List on different repo: %v", err)
		}
		if len(releases) != 0 {
			t.Errorf("expected 0 releases for different repo, got %d", len(releases))
		}
	})
}

func TestUploadAndListAssets(t *testing.T) {
	ctx, svc, r, authorID := setupReleaseTest(t)

	rel, err := svc.Create(ctx, r.ID, authorID, "v1.0.0", "Release", "", false, false)
	if err != nil {
		t.Fatalf("Create release: %v", err)
	}

	t.Run("upload single asset", func(t *testing.T) {
		asset, err := svc.UploadAsset(ctx, rel.ID, "binary.bin", "application/octet-stream", bytes.NewReader([]byte("binary-data")), 11)
		if err != nil {
			t.Fatalf("UploadAsset: %v", err)
		}
		if asset.Name != "binary.bin" {
			t.Errorf("Name = %q, want %q", asset.Name, "binary.bin")
		}
		if asset.ContentType != "application/octet-stream" {
			t.Errorf("ContentType = %q", asset.ContentType)
		}
		if asset.SizeBytes != 11 {
			t.Errorf("SizeBytes = %d, want 11", asset.SizeBytes)
		}
		if asset.ReleaseID != rel.ID {
			t.Error("ReleaseID mismatch")
		}
		if asset.ID == uuid.Nil {
			t.Error("expected non-nil asset ID")
		}
	})

	t.Run("upload multiple assets", func(t *testing.T) {
		asset2, err := svc.UploadAsset(ctx, rel.ID, "readme.txt", "text/plain", bytes.NewReader([]byte("hello")), 5)
		if err != nil {
			t.Fatalf("UploadAsset 2: %v", err)
		}
		if asset2.Name != "readme.txt" {
			t.Errorf("Name = %q, want %q", asset2.Name, "readme.txt")
		}
	})

	t.Run("upload with empty name", func(t *testing.T) {
		_, err := svc.UploadAsset(ctx, rel.ID, "", "text/plain", bytes.NewReader([]byte("data")), 4)
		if err == nil {
			t.Error("expected error for empty asset name")
		}
	})

	t.Run("upload with nonexistent release ID", func(t *testing.T) {
		_, err := svc.UploadAsset(ctx, uuid.New(), "orphan.bin", "application/octet-stream", bytes.NewReader([]byte("data")), 4)
		if err == nil {
			t.Error("expected error for nonexistent release")
		}
	})
}

func TestListAssets(t *testing.T) {
	ctx, svc, r, authorID := setupReleaseTest(t)

	rel, err := svc.Create(ctx, r.ID, authorID, "v2.0.0", "Asset Test", "", false, false)
	if err != nil {
		t.Fatalf("Create release: %v", err)
	}

	t.Run("empty asset list", func(t *testing.T) {
		assets, err := svc.ListAssets(ctx, rel.ID)
		if err != nil {
			t.Fatalf("ListAssets: %v", err)
		}
		if len(assets) != 0 {
			t.Errorf("expected empty asset list, got %d", len(assets))
		}
	})

	// Upload 2 assets
	svc.UploadAsset(ctx, rel.ID, "file_a.txt", "text/plain", bytes.NewReader([]byte("AAAA")), 4)
	svc.UploadAsset(ctx, rel.ID, "file_b.txt", "text/plain", bytes.NewReader([]byte("BBBB")), 4)

	t.Run("lists all assets", func(t *testing.T) {
		assets, err := svc.ListAssets(ctx, rel.ID)
		if err != nil {
			t.Fatalf("ListAssets: %v", err)
		}
		if len(assets) != 2 {
			t.Errorf("expected 2 assets, got %d", len(assets))
		}
	})

	t.Run("assets ordered by name", func(t *testing.T) {
		assets, _ := svc.ListAssets(ctx, rel.ID)
		if len(assets) >= 2 {
			if assets[0].Name > assets[1].Name {
				t.Error("expected assets ordered by name")
			}
		}
	})

	t.Run("different release returns empty", func(t *testing.T) {
		assets, err := svc.ListAssets(ctx, uuid.New())
		if err != nil {
			t.Fatalf("ListAssets: %v", err)
		}
		if len(assets) != 0 {
			t.Errorf("expected 0 assets, got %d", len(assets))
		}
	})
}

func TestOpenAsset(t *testing.T) {
	ctx, svc, r, authorID := setupReleaseTest(t)

	rel, err := svc.Create(ctx, r.ID, authorID, "v3.0.0", "Download Test", "", false, false)
	if err != nil {
		t.Fatalf("Create release: %v", err)
	}

	content := []byte("downloadable content")
	asset, err := svc.UploadAsset(ctx, rel.ID, "download.bin", "application/octet-stream", bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}

	t.Run("open by ID reads correct content", func(t *testing.T) {
		path, rc, err := svc.OpenAsset(ctx, asset.ID)
		if err != nil {
			t.Fatalf("OpenAsset: %v", err)
		}
		defer rc.Close()
		if path == "" {
			t.Error("expected non-empty path")
		}
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(got) != string(content) {
			t.Errorf("got %q, want %q", string(got), string(content))
		}
	})

	t.Run("open by nonexistent ID returns error", func(t *testing.T) {
		_, _, err := svc.OpenAsset(ctx, uuid.New())
		if err == nil {
			t.Error("expected error for nonexistent asset")
		}
	})
}

func TestOpenAssetByName(t *testing.T) {
	ctx, svc, r, authorID := setupReleaseTest(t)

	rel, err := svc.Create(ctx, r.ID, authorID, "v4.0.0", "ByName Test", "", false, false)
	if err != nil {
		t.Fatalf("Create release: %v", err)
	}

	content := []byte("by-name content")
	_, err = svc.UploadAsset(ctx, rel.ID, "by_name.txt", "text/plain", bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}

	t.Run("open by name returns correct asset", func(t *testing.T) {
		a, rc, err := svc.OpenAssetByName(ctx, rel.ID, "by_name.txt")
		if err != nil {
			t.Fatalf("OpenAssetByName: %v", err)
		}
		defer rc.Close()
		if a.Name != "by_name.txt" {
			t.Errorf("Name = %q", a.Name)
		}
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(got) != string(content) {
			t.Errorf("got %q, want %q", string(got), string(content))
		}
	})

	t.Run("open by nonexistent name returns error", func(t *testing.T) {
		_, _, err := svc.OpenAssetByName(ctx, rel.ID, "nonexistent.txt")
		if err == nil {
			t.Error("expected error for nonexistent asset name")
		}
	})
}

func TestGetByTag(t *testing.T) {
	ctx, svc, r, authorID := setupReleaseTest(t)

	_, err := svc.Create(ctx, r.ID, authorID, "v5.0.0", "Tagged Release", "Body text", false, false)
	if err != nil {
		t.Fatalf("Create release: %v", err)
	}

	t.Run("get by existing tag", func(t *testing.T) {
		rel, err := svc.GetByTag(ctx, r.ID, "v5.0.0")
		if err != nil {
			t.Fatalf("GetByTag: %v", err)
		}
		if rel.TagName != "v5.0.0" {
			t.Errorf("TagName = %q", rel.TagName)
		}
		if rel.Name != "Tagged Release" {
			t.Errorf("Name = %q", rel.Name)
		}
		if rel.Body != "Body text" {
			t.Errorf("Body = %q", rel.Body)
		}
	})

	t.Run("get by nonexistent tag returns error", func(t *testing.T) {
		_, err := svc.GetByTag(ctx, r.ID, "v999.0.0")
		if err == nil {
			t.Error("expected error for nonexistent tag")
		}
	})

	t.Run("get by tag on different repo returns error", func(t *testing.T) {
		_, err := svc.GetByTag(ctx, uuid.New(), "v5.0.0")
		if err == nil {
			t.Error("expected error for tag on different repo")
		}
	})
}

func TestFullLifecycle(t *testing.T) {
	ctx, svc, r, authorID := setupReleaseTest(t)

	// Create a release
	rel, err := svc.Create(ctx, r.ID, authorID, "v6.0.0", "Lifecycle Release", "Full lifecycle test", false, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Upload an asset
	content := []byte("lifecycle data")
	asset, err := svc.UploadAsset(ctx, rel.ID, "lifecycle.bin", "application/octet-stream", bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}

	// List releases
	releases, err := svc.List(ctx, r.ID)
	if err != nil || len(releases) != 1 {
		t.Fatalf("List: len=%d, err=%v", len(releases), err)
	}

	// List assets
	assets, err := svc.ListAssets(ctx, rel.ID)
	if err != nil || len(assets) != 1 {
		t.Fatalf("ListAssets: len=%d, err=%v", len(assets), err)
	}

	// Open asset by ID
	_, rc, err := svc.OpenAsset(ctx, asset.ID)
	if err != nil {
		t.Fatalf("OpenAsset: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != string(content) {
		t.Errorf("OpenAsset content = %q, want %q", string(got), string(content))
	}

	// Open asset by name
	a, rc2, err := svc.OpenAssetByName(ctx, rel.ID, "lifecycle.bin")
	if err != nil {
		t.Fatalf("OpenAssetByName: %v", err)
	}
	defer rc2.Close()
	if a.Name != "lifecycle.bin" {
		t.Errorf("Name = %q", a.Name)
	}

	// Get by tag
	rel2, err := svc.GetByTag(ctx, r.ID, "v6.0.0")
	if err != nil {
		t.Fatalf("GetByTag: %v", err)
	}
	if rel2.ID != rel.ID {
		t.Error("GetByTag returned different release")
	}
}
