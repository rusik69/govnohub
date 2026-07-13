package issue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Issue struct {
	ID            uuid.UUID  `json:"id"`
	RepoID        uuid.UUID  `json:"repo_id"`
	Number        int        `json:"number"`
	Title         string     `json:"title"`
	Body          string     `json:"body"`
	State         string     `json:"state"`
	AuthorID      uuid.UUID  `json:"author_id"`
	Author        string     `json:"author,omitempty"`
	AssigneeID    *uuid.UUID `json:"assignee_id,omitempty"`
	Assignee      string     `json:"assignee,omitempty"`
	MilestoneID   *uuid.UUID `json:"milestone_id,omitempty"`
	Milestone     string     `json:"milestone,omitempty"`
	Labels        []Label    `json:"labels,omitempty"`
	Assignees     []string   `json:"assignees,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
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

type TimelineEvent struct {
	ID        uuid.UUID       `json:"id"`
	IssueID   uuid.UUID       `json:"issue_id"`
	ActorID   *uuid.UUID      `json:"actor_id,omitempty"`
	Actor     string          `json:"actor,omitempty"`
	EventType string          `json:"event_type"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type Milestone struct {
	ID          uuid.UUID  `json:"id"`
	RepoID      uuid.UUID  `json:"repo_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	State       string     `json:"state"`
	DueOn       *time.Time `json:"due_on,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

const issueSelect = `
	SELECT i.id, i.repo_id, i.number, i.title, COALESCE(i.body,''), i.state, i.author_id,
	       COALESCE(au.username,''), i.assignee_id, COALESCE(asg.username,''),
	       i.milestone_id, COALESCE(m.title,''), i.created_at, i.updated_at`

const issueFrom = `
	FROM issues i
	LEFT JOIN users au ON i.author_id = au.id
	LEFT JOIN users asg ON i.assignee_id = asg.id
	LEFT JOIN milestones m ON i.milestone_id = m.id`

func (s *Service) scanIssue(row interface {
	Scan(dest ...any) error
}) (*Issue, error) {
	var i Issue
	err := row.Scan(&i.ID, &i.RepoID, &i.Number, &i.Title, &i.Body, &i.State, &i.AuthorID,
		&i.Author, &i.AssigneeID, &i.Assignee, &i.MilestoneID, &i.Milestone, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (s *Service) loadLabels(ctx context.Context, issueID uuid.UUID) ([]Label, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT l.id, l.name, l.color FROM labels l
		JOIN issue_labels il ON il.label_id = l.id
		WHERE il.issue_id=$1 ORDER BY l.name`, issueID)
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

func (s *Service) enrich(ctx context.Context, i *Issue) error {
	labels, err := s.loadLabels(ctx, i.ID)
	if err != nil {
		return err
	}
	i.Labels = labels
	assignees, err := s.loadAssignees(ctx, i.ID)
	if err != nil {
		return err
	}
	i.Assignees = assignees
	return nil
}

func (s *Service) loadAssignees(ctx context.Context, issueID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.username FROM issue_assignees ia
		JOIN users u ON ia.user_id = u.id
		WHERE ia.issue_id=$1 ORDER BY u.username`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (s *Service) Create(ctx context.Context, repoID, authorID uuid.UUID, title, body string) (*Issue, error) {
	var number int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(number),0)+1 FROM issues WHERE repo_id=$1`, repoID).Scan(&number)
	if err != nil {
		return nil, err
	}
	var id uuid.UUID
	err = s.pool.QueryRow(ctx, `
		INSERT INTO issues (repo_id, number, title, body, author_id)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`, repoID, number, title, body, authorID).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetByID(ctx, id)
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Issue, error) {
	i, err := s.scanIssue(s.pool.QueryRow(ctx, issueSelect+issueFrom+` WHERE i.id=$1`, id))
	if err != nil {
		return nil, err
	}
	if err := s.enrich(ctx, i); err != nil {
		return nil, err
	}
	return i, nil
}

func (s *Service) List(ctx context.Context, repoID uuid.UUID) ([]Issue, error) {
	rows, err := s.pool.Query(ctx, issueSelect+issueFrom+` WHERE i.repo_id=$1 ORDER BY i.number DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var issues []Issue
	for rows.Next() {
		i, err := s.scanIssue(rows)
		if err != nil {
			return nil, err
		}
		if err := s.enrich(ctx, i); err != nil {
			return nil, err
		}
		issues = append(issues, *i)
	}
	return issues, rows.Err()
}

func (s *Service) Get(ctx context.Context, repoID uuid.UUID, number int) (*Issue, error) {
	i, err := s.scanIssue(s.pool.QueryRow(ctx, issueSelect+issueFrom+` WHERE i.repo_id=$1 AND i.number=$2`, repoID, number))
	if err != nil {
		return nil, err
	}
	if err := s.enrich(ctx, i); err != nil {
		return nil, err
	}
	return i, nil
}

func (s *Service) AddComment(ctx context.Context, issueID, authorID uuid.UUID, body string) (*Comment, error) {
	var c Comment
	err := s.pool.QueryRow(ctx, `
		INSERT INTO issue_comments (issue_id, author_id, body) VALUES ($1,$2,$3)
		RETURNING id, issue_id, author_id, body, created_at`, issueID, authorID, body,
	).Scan(&c.ID, &c.IssueID, &c.AuthorID, &c.Body, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	var author string
	_ = s.pool.QueryRow(ctx, `SELECT username FROM users WHERE id=$1`, authorID).Scan(&author)
	c.Author = author
	return &c, nil
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

func (s *Service) RemoveLabel(ctx context.Context, issueID, labelID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM issue_labels WHERE issue_id=$1 AND label_id=$2`, issueID, labelID)
	return err
}

func (s *Service) SetAssignee(ctx context.Context, issueID uuid.UUID, assigneeID *uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE issues SET assignee_id=$2, updated_at=NOW() WHERE id=$1`, issueID, assigneeID)
	return err
}

// SetAssignees replaces all assignees for an issue with the given user IDs.
func (s *Service) SetAssignees(ctx context.Context, issueID uuid.UUID, userIDs []uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM issue_assignees WHERE issue_id=$1`, issueID); err != nil {
		return err
	}
	for _, uid := range userIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO issue_assignees (issue_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, issueID, uid); err != nil {
			return err
		}
	}
	// Also sync the legacy single assignee_id field to the first user (or nil)
	var first *uuid.UUID
	if len(userIDs) > 0 {
		first = &userIDs[0]
	}
	if _, err := tx.Exec(ctx, `UPDATE issues SET assignee_id=$2, updated_at=NOW() WHERE id=$1`, issueID, first); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) SetMilestone(ctx context.Context, issueID uuid.UUID, milestoneID *uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE issues SET milestone_id=$2, updated_at=NOW() WHERE id=$1`, issueID, milestoneID)
	return err
}

func (s *Service) CreateMilestone(ctx context.Context, repoID uuid.UUID, title, description string, dueOn *time.Time) (*Milestone, error) {
	var m Milestone
	err := s.pool.QueryRow(ctx, `
		INSERT INTO milestones (repo_id, title, description, due_on)
		VALUES ($1,$2,$3,$4)
		RETURNING id, repo_id, title, COALESCE(description,''), state, due_on, created_at`,
		repoID, title, description, dueOn,
	).Scan(&m.ID, &m.RepoID, &m.Title, &m.Description, &m.State, &m.DueOn, &m.CreatedAt)
	return &m, err
}

func (s *Service) ListMilestones(ctx context.Context, repoID uuid.UUID) ([]Milestone, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, repo_id, title, COALESCE(description,''), state, due_on, created_at
		FROM milestones WHERE repo_id=$1 ORDER BY created_at DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Milestone
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.RepoID, &m.Title, &m.Description, &m.State, &m.DueOn, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) GetMilestone(ctx context.Context, repoID, id uuid.UUID) (*Milestone, error) {
	var m Milestone
	err := s.pool.QueryRow(ctx, `
		SELECT id, repo_id, title, COALESCE(description,''), state, due_on, created_at
		FROM milestones WHERE repo_id=$1 AND id=$2`, repoID, id,
	).Scan(&m.ID, &m.RepoID, &m.Title, &m.Description, &m.State, &m.DueOn, &m.CreatedAt)
	return &m, err
}

func (s *Service) CloseMilestone(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE milestones SET state='closed' WHERE id=$1`, id)
	return err
}

func (s *Service) ListRepoUsers(ctx context.Context, repoID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT u.username FROM users u
		JOIN repo_collaborators c ON c.user_id = u.id AND c.repo_id=$1
		UNION
		SELECT u.username FROM users u
		JOIN repos r ON r.id=$1 AND r.owner_type='user' AND r.owner_id=u.id
		ORDER BY 1`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// GetLabelByID returns a label by its ID within a repo.
func (s *Service) GetLabelByID(ctx context.Context, id uuid.UUID) (*Label, error) {
	var l Label
	err := s.pool.QueryRow(ctx, `SELECT id, name, color FROM labels WHERE id=$1`, id).Scan(&l.ID, &l.Name, &l.Color)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// RecordEvent records a timeline event for an issue.
func (s *Service) RecordEvent(ctx context.Context, issueID, actorID uuid.UUID, eventType string, metadata map[string]interface{}) error {
	var meta json.RawMessage
	if len(metadata) > 0 {
		b, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		meta = b
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO issue_events (issue_id, actor_id, event_type, metadata)
		VALUES ($1, $2, $3, $4)`, issueID, actorID, eventType, meta)
	return err
}

// GetTimeline returns timeline events for an issue, ordered chronologically.
func (s *Service) GetTimeline(ctx context.Context, issueID uuid.UUID) ([]TimelineEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.issue_id, e.actor_id, COALESCE(u.username,''), e.event_type, COALESCE(e.metadata,'null'::jsonb), e.created_at
		FROM issue_events e
		LEFT JOIN users u ON e.actor_id = u.id
		WHERE e.issue_id=$1 ORDER BY e.created_at ASC`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TimelineEvent
	for rows.Next() {
		var e TimelineEvent
		if err := rows.Scan(&e.ID, &e.IssueID, &e.ActorID, &e.Actor, &e.EventType, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
