package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Collaborator struct {
	UserID     uuid.UUID `json:"user_id"`
	Username   string    `json:"username"`
	Permission string    `json:"permission"`
	AddedAt    time.Time `json:"added_at"`
}

func (s *Service) AddCollaborator(ctx context.Context, repoID, userID uuid.UUID, permission string) error {
	if permission != "read" && permission != "write" {
		permission = "read"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO repo_collaborators (repo_id, user_id, permission)
		VALUES ($1, $2, $3)
		ON CONFLICT (repo_id, user_id) DO UPDATE SET permission=EXCLUDED.permission`,
		repoID, userID, permission)
	return err
}

func (s *Service) RemoveCollaborator(ctx context.Context, repoID, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM repo_collaborators WHERE repo_id=$1 AND user_id=$2`, repoID, userID)
	return err
}

func (s *Service) ListCollaborators(ctx context.Context, repoID uuid.UUID) ([]Collaborator, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT rc.user_id, u.username, rc.permission
		FROM repo_collaborators rc
		JOIN users u ON rc.user_id = u.id
		WHERE rc.repo_id=$1 ORDER BY u.username`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Collaborator
	for rows.Next() {
		var c Collaborator
		if err := rows.Scan(&c.UserID, &c.Username, &c.Permission); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
