package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

// ListFilter holds optional filter fields for ListFiltered.
// Empty/zero fields are ignored.
type ListFilter struct {
	Action       string     `json:"action,omitempty"`
	ResourceType string     `json:"resource_type,omitempty"`
	ActorID      *uuid.UUID `json:"actor_id,omitempty"`
}

// List returns up to limit audit log entries, newest first.
func (s *Service) List(ctx context.Context, limit int) ([]Entry, error) {
	return s.ListFiltered(ctx, ListFilter{}, limit)
}

// ListFiltered returns audit log entries matching the given filter, newest first.
func (s *Service) ListFiltered(ctx context.Context, filter ListFilter, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	var conditions []string
	var args []any
	argN := 1

	if filter.Action != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", argN))
		args = append(args, filter.Action)
		argN++
	}
	if filter.ResourceType != "" {
		conditions = append(conditions, fmt.Sprintf("resource_type = $%d", argN))
		args = append(args, filter.ResourceType)
		argN++
	}
	if filter.ActorID != nil {
		conditions = append(conditions, fmt.Sprintf("actor_id = $%d", argN))
		args = append(args, *filter.ActorID)
		argN++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	args = append(args, limit)
	query := fmt.Sprintf(`
		SELECT id, actor_id, action, resource_type, resource_id, metadata, created_at
		FROM audit_log %s ORDER BY created_at DESC LIMIT $%d`, where, argN)

	rows, err := s.pool.Query(ctx, query, args...)
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
