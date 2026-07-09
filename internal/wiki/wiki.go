package wiki

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("wiki page not found")

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

type Page struct {
	ID        uuid.UUID `json:"id"`
	RepoID    uuid.UUID `json:"repo_id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	AuthorID  uuid.UUID `json:"author_id"`
	Author    string    `json:"author,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func Slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugRe.ReplaceAllString(s, "")
	return s
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID) ([]Page, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.repo_id, p.slug, p.title, COALESCE(p.content,''), p.author_id,
		       COALESCE(u.username,''), p.created_at, p.updated_at
		FROM wiki_pages p
		LEFT JOIN users u ON p.author_id = u.id
		WHERE p.repo_id=$1 ORDER BY p.title ASC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Page
	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.ID, &p.RepoID, &p.Slug, &p.Title, &p.Content, &p.AuthorID,
			&p.Author, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, repoID uuid.UUID, slug string) (*Page, error) {
	var p Page
	err := s.pool.QueryRow(ctx, `
		SELECT p.id, p.repo_id, p.slug, p.title, COALESCE(p.content,''), p.author_id,
		       COALESCE(u.username,''), p.created_at, p.updated_at
		FROM wiki_pages p
		LEFT JOIN users u ON p.author_id = u.id
		WHERE p.repo_id=$1 AND p.slug=$2`, repoID, slug,
	).Scan(&p.ID, &p.RepoID, &p.Slug, &p.Title, &p.Content, &p.AuthorID,
		&p.Author, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *Service) Upsert(ctx context.Context, repoID, authorID uuid.UUID, slug, title, content string) (*Page, error) {
	if slug == "" {
		slug = Slugify(title)
	}
	var p Page
	err := s.pool.QueryRow(ctx, `
		INSERT INTO wiki_pages (repo_id, slug, title, content, author_id)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (repo_id, slug) DO UPDATE SET
			title=EXCLUDED.title, content=EXCLUDED.content, updated_at=NOW()
		RETURNING id, repo_id, slug, title, COALESCE(content,''), author_id, created_at, updated_at`,
		repoID, slug, title, content, authorID,
	).Scan(&p.ID, &p.RepoID, &p.Slug, &p.Title, &p.Content, &p.AuthorID, &p.CreatedAt, &p.UpdatedAt)
	return &p, err
}

func (s *Service) Delete(ctx context.Context, repoID uuid.UUID, slug string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM wiki_pages WHERE repo_id=$1 AND slug=$2`, repoID, slug)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
