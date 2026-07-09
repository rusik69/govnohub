package pkg

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Package struct {
	ID          uuid.UUID `json:"id"`
	RepoID      uuid.UUID `json:"repo_id"`
	Name        string    `json:"name"`
	PackageType string    `json:"package_type"`
	Version     string    `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
}

type Service struct {
	pool    *pgxpool.Pool
	rootDir string
}

func NewService(pool *pgxpool.Pool, rootDir string) *Service {
	return &Service{pool: pool, rootDir: rootDir}
}

func (s *Service) Publish(ctx context.Context, repoID uuid.UUID, name, pkgType, version string, r io.Reader) (*Package, error) {
	dir := filepath.Join(s.rootDir, repoID.String(), name, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "artifact")
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()
	var p Package
	err = s.pool.QueryRow(ctx, `
		INSERT INTO packages (repo_id, name, package_type, version, storage_path)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (repo_id, name, version) DO UPDATE SET storage_path=EXCLUDED.storage_path
		RETURNING id, repo_id, name, package_type, version, created_at`,
		repoID, name, pkgType, version, path,
	).Scan(&p.ID, &p.RepoID, &p.Name, &p.PackageType, &p.Version, &p.CreatedAt)
	return &p, err
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID) ([]Package, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, repo_id, name, package_type, version, created_at
		FROM packages WHERE repo_id=$1 ORDER BY created_at DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pkgs []Package
	for rows.Next() {
		var p Package
		if err := rows.Scan(&p.ID, &p.RepoID, &p.Name, &p.PackageType, &p.Version, &p.CreatedAt); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, rows.Err()
}

func (s *Service) Open(ctx context.Context, repoID uuid.UUID, name, version string) (io.ReadCloser, error) {
	var path string
	err := s.pool.QueryRow(ctx, `
		SELECT storage_path FROM packages WHERE repo_id=$1 AND name=$2 AND version=$3`,
		repoID, name, version).Scan(&path)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}
