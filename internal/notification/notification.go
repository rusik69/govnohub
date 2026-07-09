package notification

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Notification struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Link      string    `json:"link,omitempty"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, title, body, link string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notifications (user_id, title, body, link) VALUES ($1,$2,$3,$4)`,
		userID, title, body, link)
	return err
}

func (s *Service) NotifyAsync(userID uuid.UUID, title, body, link string) {
	if userID == uuid.Nil {
		return
	}
	go func() {
		ctx := context.Background()
		_ = s.Create(ctx, userID, title, body, link)
	}()
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, limit int) ([]Notification, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, title, COALESCE(body,''), COALESCE(link,''), read, created_at
		FROM notifications WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.Link, &n.Read, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Service) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND read=false`, userID).Scan(&n)
	return n, err
}

func (s *Service) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE notifications SET read=true WHERE id=$1 AND user_id=$2`, id, userID)
	return err
}
