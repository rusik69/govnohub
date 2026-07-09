package release

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Release struct {
	ID         uuid.UUID `json:"id"`
	RepoID     uuid.UUID `json:"repo_id"`
	TagName    string    `json:"tag_name"`
	Name       string    `json:"name"`
	Body       string    `json:"body"`
	AuthorID   uuid.UUID `json:"author_id"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	CreatedAt  time.Time `json:"created_at"`
}

type Asset struct {
	ID          uuid.UUID `json:"id"`
	ReleaseID   uuid.UUID `json:"release_id"`
	Name        string    `json:"name"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
}

type Service struct {
	pool    *pgxpool.Pool
	rootDir string
}

func NewService(pool *pgxpool.Pool, rootDir string) *Service {
	return &Service{pool: pool, rootDir: rootDir}
}

func (s *Service) Create(ctx context.Context, repoID, authorID uuid.UUID, tag, name, body string, draft, prerelease bool) (*Release, error) {
	var r Release
	err := s.pool.QueryRow(ctx, `
		INSERT INTO releases (repo_id, tag_name, name, body, author_id, draft, prerelease)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, repo_id, tag_name, COALESCE(name,''), COALESCE(body,''), author_id, draft, prerelease, created_at`,
		repoID, tag, name, body, authorID, draft, prerelease,
	).Scan(&r.ID, &r.RepoID, &r.TagName, &r.Name, &r.Body, &r.AuthorID, &r.Draft, &r.Prerelease, &r.CreatedAt)
	return &r, err
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID) ([]Release, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, repo_id, tag_name, COALESCE(name,''), COALESCE(body,''), author_id, draft, prerelease, created_at
		FROM releases WHERE repo_id=$1 ORDER BY created_at DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var releases []Release
	for rows.Next() {
		var r Release
		if err := rows.Scan(&r.ID, &r.RepoID, &r.TagName, &r.Name, &r.Body, &r.AuthorID, &r.Draft, &r.Prerelease, &r.CreatedAt); err != nil {
			return nil, err
		}
		releases = append(releases, r)
	}
	return releases, rows.Err()
}

func (s *Service) UploadAsset(ctx context.Context, releaseID uuid.UUID, name, contentType string, r io.Reader, size int64) (*Asset, error) {
	dir := filepath.Join(s.rootDir, releaseID.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return nil, err
	}
	var a Asset
	err = s.pool.QueryRow(ctx, `
		INSERT INTO release_assets (release_id, name, content_type, size_bytes, storage_path)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, release_id, name, content_type, size_bytes`,
		releaseID, name, contentType, size, path,
	).Scan(&a.ID, &a.ReleaseID, &a.Name, &a.ContentType, &a.SizeBytes)
	return &a, err
}

func (s *Service) OpenAsset(ctx context.Context, assetID uuid.UUID) (string, io.ReadCloser, error) {
	var path string
	err := s.pool.QueryRow(ctx, `SELECT storage_path FROM release_assets WHERE id=$1`, assetID).Scan(&path)
	if err != nil {
		return "", nil, err
	}
	f, err := os.Open(path)
	return path, f, err
}
