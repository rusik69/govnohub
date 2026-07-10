package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	ID           uuid.UUID       `json:"id"`
	ActorID      *uuid.UUID      `json:"actor_id,omitempty"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Record(ctx context.Context, actorID uuid.UUID, action, resourceType, resourceID string, metadata any) error {
	var meta []byte
	if metadata != nil {
		var err error
		meta, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, resource_type, resource_id, metadata)
		VALUES ($1,$2,$3,$4,$5)`, actorID, action, resourceType, resourceID, meta)
	return err
}

func (s *Service) List(ctx context.Context, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, actor_id, action, resource_type, resource_id, metadata, created_at
		FROM audit_log ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.ActorID, &e.Action, &e.ResourceType, &e.ResourceID, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
