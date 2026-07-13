package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Event represents a public platform event.
type Event struct {
	ID           uuid.UUID        `json:"id"`
	EventType    string           `json:"event_type"`
	ActorID      *uuid.UUID       `json:"actor_id,omitempty"`
	Actor        string           `json:"actor,omitempty"`
	RepoID       *uuid.UUID       `json:"repo_id,omitempty"`
	RepoFullName string           `json:"repo_full_name,omitempty"`
	Payload      json.RawMessage  `json:"payload,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
}

// Service provides event recording and listing.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates a new events service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Record stores a new public event.
func (s *Service) Record(ctx context.Context, eventType string, actorID uuid.UUID, repoID *uuid.UUID, repoFullName string, payload any) error {
	var p json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		p = b
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO events (event_type, actor_id, repo_id, repo_full_name, payload)
		VALUES ($1, $2, $3, $4, $5)`,
		eventType, actorID, repoID, repoFullName, p)
	return err
}

// List returns the most recent public events.
func (s *Service) List(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.event_type, e.actor_id, COALESCE(u.username, ''), e.repo_id, COALESCE(e.repo_full_name, ''), COALESCE(e.payload, 'null'::jsonb), e.created_at
		FROM events e
		LEFT JOIN users u ON e.actor_id = u.id
		ORDER BY e.created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var ev Event
		if err := rows.Scan(&ev.ID, &ev.EventType, &ev.ActorID, &ev.Actor, &ev.RepoID, &ev.RepoFullName, &ev.Payload, &ev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}
