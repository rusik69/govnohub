package org

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Org struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type Team struct {
	ID          uuid.UUID `json:"id"`
	OrgID       uuid.UUID `json:"org_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Create(ctx context.Context, name, displayName, description string) (*Org, error) {
	var o Org
	err := s.pool.QueryRow(ctx, `
		INSERT INTO orgs (name, display_name, description)
		VALUES ($1,$2,$3)
		RETURNING id, name, COALESCE(display_name,''), COALESCE(description,''), created_at`,
		name, displayName, description,
	).Scan(&o.ID, &o.Name, &o.DisplayName, &o.Description, &o.CreatedAt)
	return &o, err
}

func (s *Service) AddMember(ctx context.Context, orgID, userID uuid.UUID, role string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO org_members (org_id, user_id, role) VALUES ($1,$2,$3)
		ON CONFLICT DO NOTHING`, orgID, userID, role)
	return err
}

func (s *Service) CreateTeam(ctx context.Context, orgID uuid.UUID, name, description string) (*Team, error) {
	var t Team
	err := s.pool.QueryRow(ctx, `
		INSERT INTO teams (org_id, name, description) VALUES ($1,$2,$3)
		RETURNING id, org_id, name, COALESCE(description,'')`, orgID, name, description,
	).Scan(&t.ID, &t.OrgID, &t.Name, &t.Description)
	return &t, err
}

func (s *Service) AddTeamMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO team_members (team_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, teamID, userID)
	return err
}

func (s *Service) IsMember(ctx context.Context, orgID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT true FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, userID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return ok, err
}

func (s *Service) List(ctx context.Context) ([]Org, error) {
	return s.ListPaginated(ctx, 0, 0)
}

func (s *Service) ListPaginated(ctx context.Context, limit, offset int) ([]Org, error) {
	query := `SELECT id, name, COALESCE(display_name,''), COALESCE(description,''), created_at FROM orgs`
	var args []any
	if limit > 0 {
		query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, len(args)+1)
		args = append(args, limit)
	}
	if offset > 0 {
		query += fmt.Sprintf(` OFFSET $%d`, len(args)+1)
		args = append(args, offset)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var orgs []Org
	for rows.Next() {
		var o Org
		rows.Scan(&o.ID, &o.Name, &o.DisplayName, &o.Description, &o.CreatedAt)
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}

func (s *Service) GetByName(ctx context.Context, name string) (*Org, error) {
	var o Org
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, COALESCE(display_name,''), COALESCE(description,''), created_at
		FROM orgs WHERE name=$1`, name,
	).Scan(&o.ID, &o.Name, &o.DisplayName, &o.Description, &o.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("org not found")
		}
		return nil, err
	}
	return &o, nil
}

type Member struct {
	UserID   uuid.UUID `json:"user_id"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
}

func (s *Service) ListMembers(ctx context.Context, orgID uuid.UUID) ([]Member, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.user_id, u.username, m.role
		FROM org_members m JOIN users u ON m.user_id = u.id
		WHERE m.org_id=$1 ORDER BY u.username`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Username, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) ListTeams(ctx context.Context, orgID uuid.UUID) ([]Team, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, name, COALESCE(description,'') FROM teams WHERE org_id=$1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Name, &t.Description); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) GetTeam(ctx context.Context, orgID uuid.UUID, teamName string) (*Team, error) {
	var t Team
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, name, COALESCE(description,'')
		FROM teams WHERE org_id=$1 AND name=$2`, orgID, teamName,
	).Scan(&t.ID, &t.OrgID, &t.Name, &t.Description)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("team not found")
		}
		return nil, err
	}
	return &t, nil
}

func (s *Service) ListTeamMembers(ctx context.Context, teamID uuid.UUID) ([]Member, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT tm.user_id, u.username, 'member'
		FROM team_members tm JOIN users u ON tm.user_id = u.id
		WHERE tm.team_id=$1 ORDER BY u.username`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Username, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]Org, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, o.name, COALESCE(o.display_name,''), COALESCE(o.description,''), o.created_at
		FROM orgs o JOIN org_members m ON m.org_id = o.id
		WHERE m.user_id=$1 ORDER BY o.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Org
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.DisplayName, &o.Description, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
