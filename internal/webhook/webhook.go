package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rusik69/govnohub/internal/httputil"
)

type Hook struct {
	ID     uuid.UUID `json:"id"`
	RepoID uuid.UUID `json:"repo_id"`
	URL    string    `json:"url"`
	Events []string  `json:"events"`
	Active bool      `json:"active"`
}

type Service struct {
	pool       *pgxpool.Pool
	httpClient *http.Client
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:       pool,
		httpClient: httputil.NewClient(),
	}
}

func (s *Service) Create(ctx context.Context, repoID uuid.UUID, url, secret string, events []string) (*Hook, error) {
	var h Hook
	err := s.pool.QueryRow(ctx, `
		INSERT INTO webhooks (repo_id, url, secret, events) VALUES ($1,$2,$3,$4)
		RETURNING id, repo_id, url, events, active`, repoID, url, secret, events,
	).Scan(&h.ID, &h.RepoID, &h.URL, &h.Events, &h.Active)
	return &h, err
}

func (s *Service) Dispatch(ctx context.Context, repoID uuid.UUID, event string, payload any) error {
	rows, err := s.pool.Query(ctx, `
		SELECT id, url, secret FROM webhooks
		WHERE repo_id=$1 AND active=true AND $2 = ANY(events)`, repoID, event)
	if err != nil {
		return err
	}
	defer rows.Close()
	body, _ := json.Marshal(payload)
	for rows.Next() {
		var id uuid.UUID
		var url, secret string
		if err := rows.Scan(&id, &url, &secret); err != nil {
			return err
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Govnohub-Event", event)
		sig := sign(secret, body)
		req.Header.Set("X-Govnohub-Signature-256", "sha256="+sig)
		resp, err := s.httpClient.Do(req)
		status := 0
		if resp != nil {
			status = resp.StatusCode
			resp.Body.Close()
		}
		_, _ = s.pool.Exec(ctx, `
			INSERT INTO webhook_deliveries (webhook_id, event, payload, status_code)
			VALUES ($1,$2,$3,$4)`, id, event, body, status)
		if err != nil {
			log.Printf("webhook delivery to %s: %v", url, err)
			continue
		}
	}
	return nil
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

type PushEvent struct {
	Ref        string    `json:"ref"`
	Repository string    `json:"repository"`
	Pusher     string    `json:"pusher"`
	After      string    `json:"after"`
	Timestamp  time.Time `json:"timestamp"`
}

func NewPushEvent(owner, name, branch, sha, pusher string) PushEvent {
	return PushEvent{
		Ref:        "refs/heads/" + branch,
		Repository: owner + "/" + name,
		Pusher:     pusher,
		After:      sha,
		Timestamp:  time.Now().UTC(),
	}
}

type Delivery struct {
	ID         uuid.UUID `json:"id"`
	WebhookID  uuid.UUID `json:"webhook_id"`
	Event      string    `json:"event"`
	StatusCode int       `json:"status_code"`
	DeliveredAt time.Time `json:"delivered_at"`
}

func (s *Service) ListDeliveries(ctx context.Context, webhookID uuid.UUID, limit int) ([]Delivery, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, webhook_id, event, status_code, delivered_at
		FROM webhook_deliveries WHERE webhook_id=$1
		ORDER BY delivered_at DESC LIMIT $2`, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Delivery
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &d.StatusCode, &d.DeliveredAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
