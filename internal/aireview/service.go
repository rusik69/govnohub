package aireview

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AIReview struct {
	ID        uuid.UUID `json:"id"`
	PRID      uuid.UUID `json:"pr_id"`
	Model     string    `json:"model"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	pool   *pgxpool.Pool
	client *Client
}

func NewService(pool *pgxpool.Pool, client *Client) *Service {
	return &Service{pool: pool, client: client}
}

func (s *Service) Client() *Client {
	return s.client
}

func (s *Service) Save(ctx context.Context, prID uuid.UUID, model, body string) (*AIReview, error) {
	var r AIReview
	err := s.pool.QueryRow(ctx, `
		INSERT INTO pr_ai_reviews (pr_id, model, body) VALUES ($1,$2,$3)
		RETURNING id, pr_id, model, body, created_at`,
		prID, model, body,
	).Scan(&r.ID, &r.PRID, &r.Model, &r.Body, &r.CreatedAt)
	return &r, err
}

func (s *Service) List(ctx context.Context, prID uuid.UUID) ([]AIReview, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, pr_id, model, body, created_at
		FROM pr_ai_reviews WHERE pr_id=$1 ORDER BY created_at DESC`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reviews []AIReview
	for rows.Next() {
		var r AIReview
		if err := rows.Scan(&r.ID, &r.PRID, &r.Model, &r.Body, &r.CreatedAt); err != nil {
			return nil, err
		}
		reviews = append(reviews, r)
	}
	return reviews, rows.Err()
}
