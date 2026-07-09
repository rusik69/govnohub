package pull

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PullRequest struct {
	ID         uuid.UUID  `json:"id"`
	RepoID     uuid.UUID  `json:"repo_id"`
	Number     int        `json:"number"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	State      string     `json:"state"`
	AuthorID   uuid.UUID  `json:"author_id"`
	HeadBranch string     `json:"head_branch"`
	BaseBranch string     `json:"base_branch"`
	HeadSHA    string     `json:"head_sha"`
	MergedAt   *time.Time `json:"merged_at,omitempty"`
	MergeSHA   string     `json:"merge_sha,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Review struct {
	ID         uuid.UUID `json:"id"`
	PRID       uuid.UUID `json:"pr_id"`
	ReviewerID uuid.UUID `json:"reviewer_id"`
	Reviewer   string    `json:"reviewer,omitempty"`
	State      string    `json:"state"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Create(ctx context.Context, repoID, authorID uuid.UUID, title, body, head, base, headSHA string) (*PullRequest, error) {
	var number int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(number),0)+1 FROM pull_requests WHERE repo_id=$1`, repoID).Scan(&number)
	if err != nil {
		return nil, err
	}
	var pr PullRequest
	err = s.pool.QueryRow(ctx, `
		INSERT INTO pull_requests (repo_id, number, title, body, author_id, head_branch, base_branch, head_sha)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, repo_id, number, title, COALESCE(body,''), state, author_id,
		          head_branch, base_branch, head_sha, created_at, updated_at`,
		repoID, number, title, body, authorID, head, base, headSHA,
	).Scan(&pr.ID, &pr.RepoID, &pr.Number, &pr.Title, &pr.Body, &pr.State, &pr.AuthorID,
		&pr.HeadBranch, &pr.BaseBranch, &pr.HeadSHA, &pr.CreatedAt, &pr.UpdatedAt)
	return &pr, err
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID) ([]PullRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, repo_id, number, title, COALESCE(body,''), state, author_id,
		       head_branch, base_branch, head_sha, merged_at, COALESCE(merge_sha,''), created_at, updated_at
		FROM pull_requests WHERE repo_id=$1 ORDER BY number DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var prs []PullRequest
	for rows.Next() {
		var pr PullRequest
		if err := rows.Scan(&pr.ID, &pr.RepoID, &pr.Number, &pr.Title, &pr.Body, &pr.State, &pr.AuthorID,
			&pr.HeadBranch, &pr.BaseBranch, &pr.HeadSHA, &pr.MergedAt, &pr.MergeSHA, &pr.CreatedAt, &pr.UpdatedAt); err != nil {
			return nil, err
		}
		prs = append(prs, pr)
	}
	return prs, rows.Err()
}

func (s *Service) Get(ctx context.Context, repoID uuid.UUID, number int) (*PullRequest, error) {
	var pr PullRequest
	err := s.pool.QueryRow(ctx, `
		SELECT id, repo_id, number, title, COALESCE(body,''), state, author_id,
		       head_branch, base_branch, head_sha, merged_at, COALESCE(merge_sha,''), created_at, updated_at
		FROM pull_requests WHERE repo_id=$1 AND number=$2`, repoID, number,
	).Scan(&pr.ID, &pr.RepoID, &pr.Number, &pr.Title, &pr.Body, &pr.State, &pr.AuthorID,
		&pr.HeadBranch, &pr.BaseBranch, &pr.HeadSHA, &pr.MergedAt, &pr.MergeSHA, &pr.CreatedAt, &pr.UpdatedAt)
	return &pr, err
}

func (s *Service) AddReview(ctx context.Context, prID, reviewerID uuid.UUID, state, body string) (*Review, error) {
	var r Review
	err := s.pool.QueryRow(ctx, `
		INSERT INTO pr_reviews (pr_id, reviewer_id, state, body) VALUES ($1,$2,$3,$4)
		RETURNING id, pr_id, reviewer_id, state, COALESCE(body,''), created_at`,
		prID, reviewerID, state, body,
	).Scan(&r.ID, &r.PRID, &r.ReviewerID, &r.State, &r.Body, &r.CreatedAt)
	return &r, err
}

func (s *Service) ListReviews(ctx context.Context, prID uuid.UUID) ([]Review, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.pr_id, r.reviewer_id, COALESCE(u.username,''), r.state, COALESCE(r.body,''), r.created_at
		FROM pr_reviews r
		LEFT JOIN users u ON r.reviewer_id = u.id
		WHERE r.pr_id=$1 ORDER BY r.created_at ASC`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Review
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ID, &r.PRID, &r.ReviewerID, &r.Reviewer, &r.State, &r.Body, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) Merge(ctx context.Context, prID uuid.UUID, mergeSHA string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE pull_requests SET state='closed', merged_at=NOW(), merge_sha=$2, updated_at=NOW()
		WHERE id=$1`, prID, mergeSHA)
	return err
}

func (s *Service) ReviewCount(ctx context.Context, prID uuid.UUID, state string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM pr_reviews WHERE pr_id=$1 AND state=$2`, prID, state).Scan(&n)
	return n, err
}

type LineComment struct {
	ID        uuid.UUID `json:"id"`
	PRID      uuid.UUID `json:"pr_id"`
	AuthorID  uuid.UUID `json:"author_id"`
	Author    string    `json:"author,omitempty"`
	Body      string    `json:"body"`
	Path      string    `json:"path,omitempty"`
	Line      int       `json:"line,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) AddComment(ctx context.Context, prID, authorID uuid.UUID, body, path string, line int) (*LineComment, error) {
	var c LineComment
	err := s.pool.QueryRow(ctx, `
		INSERT INTO pr_comments (pr_id, author_id, body, path, line) VALUES ($1,$2,$3,$4,$5)
		RETURNING id, pr_id, author_id, body, COALESCE(path,''), COALESCE(line,0), created_at`,
		prID, authorID, body, path, line,
	).Scan(&c.ID, &c.PRID, &c.AuthorID, &c.Body, &c.Path, &c.Line, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	_ = s.pool.QueryRow(ctx, `SELECT username FROM users WHERE id=$1`, authorID).Scan(&c.Author)
	return &c, nil
}

func (s *Service) ListComments(ctx context.Context, prID uuid.UUID) ([]LineComment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.pr_id, c.author_id, COALESCE(u.username,''), c.body,
		       COALESCE(c.path,''), COALESCE(c.line,0), c.created_at
		FROM pr_comments c
		LEFT JOIN users u ON c.author_id = u.id
		WHERE c.pr_id=$1 ORDER BY c.created_at ASC`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LineComment
	for rows.Next() {
		var c LineComment
		if err := rows.Scan(&c.ID, &c.PRID, &c.AuthorID, &c.Author, &c.Body, &c.Path, &c.Line, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
