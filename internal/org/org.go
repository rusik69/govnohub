package org

import (
	"context"
	"errors"
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
	rows, err := s.pool.Query(ctx, `SELECT id, name, COALESCE(display_name,''), COALESCE(description,''), created_at FROM orgs`)
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
