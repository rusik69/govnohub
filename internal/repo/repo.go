package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("repository not found")
var ErrForbidden = errors.New("forbidden")

type Repository struct {
	ID            uuid.UUID `json:"id"`
	OwnerType     string    `json:"owner_type"`
	OwnerID       uuid.UUID `json:"owner_id"`
	OwnerName     string    `json:"owner_name"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Description   string    `json:"description"`
	DefaultBranch string    `json:"default_branch"`
	IsPrivate     bool      `json:"is_private"`
	IsFork        bool      `json:"is_fork"`
	StarCount     int       `json:"star_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Create(ctx context.Context, ownerType string, ownerID uuid.UUID, ownerName, name, description string, isPrivate bool) (*Repository, error) {
	var r Repository
	err := s.pool.QueryRow(ctx, `
		INSERT INTO repos (owner_type, owner_id, name, description, is_private)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, owner_type, owner_id, name, COALESCE(description,''), default_branch,
		          is_private, is_fork, star_count, created_at, updated_at`,
		ownerType, ownerID, name, description, isPrivate,
	).Scan(&r.ID, &r.OwnerType, &r.OwnerID, &r.Name, &r.Description, &r.DefaultBranch,
		&r.IsPrivate, &r.IsFork, &r.StarCount, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.OwnerName = ownerName
	r.FullName = ownerName + "/" + r.Name
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO branches (repo_id, name, head_sha) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, r.ID, r.DefaultBranch, "0000000000000000000000000000000000000000")
	return &r, nil
}

func (s *Service) GetByFullName(ctx context.Context, owner, name string) (*Repository, error) {
	var r Repository
	err := s.pool.QueryRow(ctx, `
		SELECT r.id, r.owner_type, r.owner_id, r.name, COALESCE(r.description,''),
		       r.default_branch, r.is_private, r.is_fork, r.star_count, r.created_at, r.updated_at,
		       COALESCE(u.username, o.name) AS owner_name
		FROM repos r
		LEFT JOIN users u ON r.owner_type='user' AND r.owner_id=u.id
		LEFT JOIN orgs o ON r.owner_type='org' AND r.owner_id=o.id
		WHERE (u.username=$1 OR o.name=$1) AND r.name=$2`, owner, name,
	).Scan(&r.ID, &r.OwnerType, &r.OwnerID, &r.Name, &r.Description, &r.DefaultBranch,
		&r.IsPrivate, &r.IsFork, &r.StarCount, &r.CreatedAt, &r.UpdatedAt, &r.OwnerName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	r.FullName = r.OwnerName + "/" + r.Name
	return &r, nil
}

func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]Repository, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.owner_type, r.owner_id, r.name, COALESCE(r.description,''),
		       r.default_branch, r.is_private, r.is_fork, r.star_count, r.created_at, r.updated_at,
		       COALESCE(u.username, o.name) AS owner_name
		FROM repos r
		LEFT JOIN users u ON r.owner_type='user' AND r.owner_id=u.id
		LEFT JOIN orgs o ON r.owner_type='org' AND r.owner_id=o.id
		WHERE (r.owner_type='user' AND r.owner_id=$1)
		   OR r.id IN (SELECT repo_id FROM repo_collaborators WHERE user_id=$1)
		ORDER BY r.updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRepos(rows)
}

func (s *Service) ListForOrg(ctx context.Context, orgID uuid.UUID) ([]Repository, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.owner_type, r.owner_id, r.name, COALESCE(r.description,''),
		       r.default_branch, r.is_private, r.is_fork, r.star_count, r.created_at, r.updated_at,
		       o.name AS owner_name
		FROM repos r
		JOIN orgs o ON r.owner_type='org' AND r.owner_id=o.id
		WHERE r.owner_id=$1
		ORDER BY r.updated_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRepos(rows)
}

func (s *Service) CanAccess(ctx context.Context, repoID, userID uuid.UUID, minPerm string) (bool, error) {
	var ownerType string
	var ownerID uuid.UUID
	var isPrivate bool
	err := s.pool.QueryRow(ctx, `SELECT owner_type, owner_id, is_private FROM repos WHERE id=$1`, repoID).
		Scan(&ownerType, &ownerID, &isPrivate)
	if err != nil {
		return false, err
	}
	if ownerType == "user" && ownerID == userID {
		return true, nil
	}
	if ownerType == "org" {
		var role string
		err = s.pool.QueryRow(ctx, `SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2`, ownerID, userID).Scan(&role)
		if err == nil {
			orgPerm := "read"
			if role == "admin" {
				orgPerm = "admin"
			}
			if permRank(orgPerm) >= permRank(minPerm) {
				return true, nil
			}
		}
	}
	var perm string
	err = s.pool.QueryRow(ctx, `SELECT permission FROM repo_collaborators WHERE repo_id=$1 AND user_id=$2`, repoID, userID).Scan(&perm)
	if err == nil {
		return permRank(perm) >= permRank(minPerm), nil
	}
	return !isPrivate, nil
}

func (s *Service) UpdateBranchHead(ctx context.Context, repoID uuid.UUID, branch, sha string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO branches (repo_id, name, head_sha) VALUES ($1, $2, $3)
		ON CONFLICT (repo_id, name) DO UPDATE SET head_sha=EXCLUDED.head_sha`, repoID, branch, sha)
	return err
}

func (s *Service) Fork(ctx context.Context, source *Repository, userID uuid.UUID, username string) (*Repository, error) {
	desc := "Forked from " + source.FullName
	return s.Create(ctx, "user", userID, username, source.Name+"-fork", desc, source.IsPrivate)
}

func (s *Service) Star(ctx context.Context, repoID, userID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var inserted uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO repo_stars (repo_id, user_id) VALUES ($1,$2)
		ON CONFLICT DO NOTHING RETURNING user_id`, repoID, userID).Scan(&inserted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE repos SET star_count = star_count + 1 WHERE id=$1`, repoID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) Watch(ctx context.Context, repoID, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO repo_watchers (repo_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, repoID, userID)
	return err
}

func permRank(p string) int {
	switch p {
	case "admin":
		return 3
	case "write":
		return 2
	default:
		return 1
	}
}

func scanRepos(rows pgx.Rows) ([]Repository, error) {
	var repos []Repository
	for rows.Next() {
		var r Repository
		if err := rows.Scan(&r.ID, &r.OwnerType, &r.OwnerID, &r.Name, &r.Description, &r.DefaultBranch,
			&r.IsPrivate, &r.IsFork, &r.StarCount, &r.CreatedAt, &r.UpdatedAt, &r.OwnerName); err != nil {
			return nil, err
		}
		r.FullName = r.OwnerName + "/" + r.Name
		repos = append(repos, r)
	}
	if repos == nil {
		repos = []Repository{}
	}
	return repos, rows.Err()
}

func (s *Service) ResolveOwnerID(ctx context.Context, ownerName string) (string, uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT id FROM users WHERE username=$1`, ownerName).Scan(&id)
	if err == nil {
		return "user", id, nil
	}
	err = s.pool.QueryRow(ctx, `SELECT id FROM orgs WHERE name=$1`, ownerName).Scan(&id)
	if err == nil {
		return "org", id, nil
	}
	return "", uuid.Nil, fmt.Errorf("owner not found: %s", ownerName)
}
