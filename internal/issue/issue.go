package issue

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Issue struct {
	ID        uuid.UUID `json:"id"`
	RepoID    uuid.UUID `json:"repo_id"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	AuthorID  uuid.UUID `json:"author_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Comment struct {
	ID        uuid.UUID `json:"id"`
	IssueID   uuid.UUID `json:"issue_id"`
	AuthorID  uuid.UUID `json:"author_id"`
	Author    string    `json:"author,omitempty"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Label struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Create(ctx context.Context, repoID, authorID uuid.UUID, title, body string) (*Issue, error) {
	var number int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(number),0)+1 FROM issues WHERE repo_id=$1`, repoID).Scan(&number)
	if err != nil {
		return nil, err
	}
	var i Issue
	err = s.pool.QueryRow(ctx, `
		INSERT INTO issues (repo_id, number, title, body, author_id)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, repo_id, number, title, COALESCE(body,''), state, author_id, created_at, updated_at`,
		repoID, number, title, body, authorID,
	).Scan(&i.ID, &i.RepoID, &i.Number, &i.Title, &i.Body, &i.State, &i.AuthorID, &i.CreatedAt, &i.UpdatedAt)
	return &i, err
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID) ([]Issue, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, repo_id, number, title, COALESCE(body,''), state, author_id, created_at, updated_at
		FROM issues WHERE repo_id=$1 ORDER BY number DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var issues []Issue
	for rows.Next() {
		var i Issue
		if err := rows.Scan(&i.ID, &i.RepoID, &i.Number, &i.Title, &i.Body, &i.State, &i.AuthorID, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

func (s *Service) Get(ctx context.Context, repoID uuid.UUID, number int) (*Issue, error) {
	var i Issue
	err := s.pool.QueryRow(ctx, `
		SELECT id, repo_id, number, title, COALESCE(body,''), state, author_id, created_at, updated_at
		FROM issues WHERE repo_id=$1 AND number=$2`, repoID, number,
	).Scan(&i.ID, &i.RepoID, &i.Number, &i.Title, &i.Body, &i.State, &i.AuthorID, &i.CreatedAt, &i.UpdatedAt)
	return &i, err
}

func (s *Service) AddComment(ctx context.Context, issueID, authorID uuid.UUID, body string) (*Comment, error) {
	var c Comment
	err := s.pool.QueryRow(ctx, `
		INSERT INTO issue_comments (issue_id, author_id, body) VALUES ($1,$2,$3)
		RETURNING id, issue_id, author_id, body, created_at`, issueID, authorID, body,
	).Scan(&c.ID, &c.IssueID, &c.AuthorID, &c.Body, &c.CreatedAt)
	return &c, err
}

func (s *Service) ListComments(ctx context.Context, issueID uuid.UUID) ([]Comment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.issue_id, c.author_id, COALESCE(u.username,''), c.body, c.created_at
		FROM issue_comments c
		LEFT JOIN users u ON c.author_id = u.id
		WHERE c.issue_id=$1 ORDER BY c.created_at ASC`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.IssueID, &c.AuthorID, &c.Author, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Service) ListLabels(ctx context.Context, repoID uuid.UUID) ([]Label, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, color FROM labels WHERE repo_id=$1 ORDER BY name`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Label
	for rows.Next() {
		var l Label
		if err := rows.Scan(&l.ID, &l.Name, &l.Color); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Service) Close(ctx context.Context, issueID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE issues SET state='closed', closed_at=NOW(), updated_at=NOW() WHERE id=$1`, issueID)
	return err
}

func (s *Service) CreateLabel(ctx context.Context, repoID uuid.UUID, name, color string) (*Label, error) {
	var l Label
	err := s.pool.QueryRow(ctx, `
		INSERT INTO labels (repo_id, name, color) VALUES ($1,$2,$3)
		RETURNING id, name, color`, repoID, name, color,
	).Scan(&l.ID, &l.Name, &l.Color)
	return &l, err
}

func (s *Service) AddLabel(ctx context.Context, issueID, labelID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO issue_labels (issue_id, label_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, issueID, labelID)
	return err
}
