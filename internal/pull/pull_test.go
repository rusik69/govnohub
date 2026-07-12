package pull

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/testutil"
)

func setupPullTest(t *testing.T) (*auth.Service, *repo.Service, *Service, *auth.User, *repo.Repository, context.Context) {
	t.Helper()
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)

	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "secret")
	repoSvc := repo.NewService(pg.Pool)
	pullSvc := NewService(pg.Pool)

	u, err := authSvc.Register(ctx, "pulluser", "pull@test.local", "pass")
	if err != nil {
		t.Fatal(err)
	}

	r, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "pull-test-repo", "", false)
	if err != nil {
		t.Fatal(err)
	}

	return authSvc, repoSvc, pullSvc, u, r, ctx
}

func TestPullCreate(t *testing.T) {
	_, _, pullSvc, u, r, ctx := setupPullTest(t)

	t.Run("create basic pull request", func(t *testing.T) {
		pr, err := pullSvc.Create(ctx, r.ID, u.ID, "feat: add feature", "body of the PR", "feature", "main", "abc123")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if pr.Number != 1 {
			t.Errorf("Number = %d, want 1", pr.Number)
		}
		if pr.Title != "feat: add feature" {
			t.Errorf("Title = %q, want %q", pr.Title, "feat: add feature")
		}
		if pr.Body != "body of the PR" {
			t.Errorf("Body = %q, want %q", pr.Body, "body of the PR")
		}
		if pr.State != "open" {
			t.Errorf("State = %q, want %q", pr.State, "open")
		}
		if pr.AuthorID != u.ID {
			t.Errorf("AuthorID mismatch")
		}
		if pr.HeadBranch != "feature" {
			t.Errorf("HeadBranch = %q, want %q", pr.HeadBranch, "feature")
		}
		if pr.BaseBranch != "main" {
			t.Errorf("BaseBranch = %q, want %q", pr.BaseBranch, "main")
		}
		if pr.HeadSHA != "abc123" {
			t.Errorf("HeadSHA = %q, want %q", pr.HeadSHA, "abc123")
		}
		if pr.MergedAt != nil {
			t.Error("MergedAt should be nil for new PR")
		}
		if pr.MergeSHA != "" {
			t.Errorf("MergeSHA should be empty, got %q", pr.MergeSHA)
		}
		if pr.CreatedAt.IsZero() {
			t.Error("CreatedAt should not be zero")
		}
	})

	t.Run("create with empty body", func(t *testing.T) {
		pr, err := pullSvc.Create(ctx, r.ID, u.ID, "minimal", "", "branch", "main", "def456")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if pr.Body != "" {
			t.Errorf("Body = %q, want empty", pr.Body)
		}
	})

	t.Run("auto-increment number per repo", func(t *testing.T) {
		pr1, err := pullSvc.Create(ctx, r.ID, u.ID, "first", "", "branch1", "main", "sha1")
		if err != nil {
			t.Fatal(err)
		}
		pr2, err := pullSvc.Create(ctx, r.ID, u.ID, "second", "", "branch2", "main", "sha2")
		if err != nil {
			t.Fatal(err)
		}
		if pr2.Number <= pr1.Number {
			t.Errorf("pr2.Number (%d) should be > pr1.Number (%d)", pr2.Number, pr1.Number)
		}
	})
}

func TestPullGet(t *testing.T) {
	_, repoSvc, pullSvc, u, r, ctx := setupPullTest(t)

	created, err := pullSvc.Create(ctx, r.ID, u.ID, "get-test", "body", "branch", "main", "abc")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("get existing PR by number", func(t *testing.T) {
		pr, err := pullSvc.Get(ctx, r.ID, created.Number)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if pr.ID != created.ID {
			t.Errorf("ID mismatch")
		}
		if pr.Number != created.Number {
			t.Errorf("Number = %d, want %d", pr.Number, created.Number)
		}
		if pr.Title != "get-test" {
			t.Errorf("Title = %q, want %q", pr.Title, "get-test")
		}
		if pr.Body != "body" {
			t.Errorf("Body = %q, want %q", pr.Body, "body")
		}
		if pr.AuthorID != u.ID {
			t.Errorf("AuthorID mismatch")
		}
	})

	t.Run("get non-existent PR number", func(t *testing.T) {
		_, err := pullSvc.Get(ctx, r.ID, 99999)
		if err != pgx.ErrNoRows {
			t.Fatalf("expected pgx.ErrNoRows, got %v", err)
		}
	})

	t.Run("get PR in wrong repo", func(t *testing.T) {
		otherR, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "other-repo", "", false)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pullSvc.Get(ctx, otherR.ID, created.Number)
		if err != pgx.ErrNoRows {
			t.Fatalf("expected pgx.ErrNoRows for wrong repo, got %v", err)
		}
	})
}

func TestPullList(t *testing.T) {
	_, repoSvc, pullSvc, u, r, ctx := setupPullTest(t)

	t.Run("list returns empty for new repo", func(t *testing.T) {
		prs, err := pullSvc.List(ctx, r.ID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(prs) != 0 {
			t.Errorf("expected 0 PRs, got %d", len(prs))
		}
	})

	// Create a few PRs
	for i := 0; i < 3; i++ {
		title := "pr-" + string(rune('a'+i))
		if _, err := pullSvc.Create(ctx, r.ID, u.ID, title, "", "branch", "main", "sha"); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("list returns all PRs in desc order", func(t *testing.T) {
		prs, err := pullSvc.List(ctx, r.ID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(prs) != 3 {
			t.Fatalf("expected 3 PRs, got %d", len(prs))
		}
		// Should be in descending number order
		for i := 1; i < len(prs); i++ {
			if prs[i].Number >= prs[i-1].Number {
				t.Errorf("PRs not in descending order: %d >= %d", prs[i].Number, prs[i-1].Number)
			}
		}
	})

	t.Run("list for different repos are isolated", func(t *testing.T) {
		otherR, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "empty-repo", "", false)
		if err != nil {
			t.Fatal(err)
		}
		prs, err := pullSvc.List(ctx, otherR.ID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(prs) != 0 {
			t.Errorf("expected 0 PRs in other repo, got %d", len(prs))
		}
	})
}

func TestPullMerge(t *testing.T) {
	_, _, pullSvc, u, r, ctx := setupPullTest(t)

	pr, err := pullSvc.Create(ctx, r.ID, u.ID, "merge-pr", "body", "branch", "main", "headsha")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("merge sets state to closed and records merge SHA", func(t *testing.T) {
		if err := pullSvc.Merge(ctx, pr.ID, "mergedsha123"); err != nil {
			t.Fatalf("Merge: %v", err)
		}

		updated, err := pullSvc.Get(ctx, r.ID, pr.Number)
		if err != nil {
			t.Fatal(err)
		}
		if updated.State != "closed" {
			t.Errorf("State = %q, want %q", updated.State, "closed")
		}
		if updated.MergeSHA != "mergedsha123" {
			t.Errorf("MergeSHA = %q, want %q", updated.MergeSHA, "mergedsha123")
		}
		if updated.MergedAt == nil || updated.MergedAt.IsZero() {
			t.Error("MergedAt should be set after merge")
		}
	})

	t.Run("merge is idempotent", func(t *testing.T) {
		secondMergeSHA := "secondsha"
		if err := pullSvc.Merge(ctx, pr.ID, secondMergeSHA); err != nil {
			t.Fatalf("Merge (idempotent): %v", err)
		}
		updated, err := pullSvc.Get(ctx, r.ID, pr.Number)
		if err != nil {
			t.Fatal(err)
		}
		if updated.MergeSHA != secondMergeSHA {
			t.Errorf("MergeSHA after idempotent merge = %q, want %q", updated.MergeSHA, secondMergeSHA)
		}
	})
}

func TestPullReview(t *testing.T) {
	_, _, pullSvc, u, r, ctx := setupPullTest(t)

	pr, err := pullSvc.Create(ctx, r.ID, u.ID, "review-pr", "", "branch", "main", "abc")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("add review and verify", func(t *testing.T) {
		rev, err := pullSvc.AddReview(ctx, pr.ID, u.ID, "approved", "Looks good to me")
		if err != nil {
			t.Fatalf("AddReview: %v", err)
		}
		if rev.PRID != pr.ID {
			t.Errorf("PRID mismatch")
		}
		if rev.ReviewerID != u.ID {
			t.Errorf("ReviewerID mismatch")
		}
		if rev.State != "approved" {
			t.Errorf("State = %q, want %q", rev.State, "approved")
		}
		if rev.Body != "Looks good to me" {
			t.Errorf("Body = %q, want %q", rev.Body, "Looks good to me")
		}
		if rev.CreatedAt.IsZero() {
			t.Error("CreatedAt should not be zero")
		}
	})

	t.Run("add review with empty body", func(t *testing.T) {
		rev, err := pullSvc.AddReview(ctx, pr.ID, u.ID, "changes_requested", "")
		if err != nil {
			t.Fatalf("AddReview: %v", err)
		}
		if rev.Body != "" {
			t.Errorf("Body = %q, want empty", rev.Body)
		}
	})

	t.Run("list reviews returns in order", func(t *testing.T) {
		reviews, err := pullSvc.ListReviews(ctx, pr.ID)
		if err != nil {
			t.Fatalf("ListReviews: %v", err)
		}
		if len(reviews) != 2 {
			t.Fatalf("expected 2 reviews, got %d", len(reviews))
		}
		if reviews[0].State != "approved" {
			t.Errorf("first review state = %q, want %q", reviews[0].State, "approved")
		}
		if reviews[1].State != "changes_requested" {
			t.Errorf("second review state = %q, want %q", reviews[1].State, "changes_requested")
		}
	})

	t.Run("review count by state", func(t *testing.T) {
		count, err := pullSvc.ReviewCount(ctx, pr.ID, "approved")
		if err != nil {
			t.Fatalf("ReviewCount: %v", err)
		}
		if count != 1 {
			t.Errorf("approved count = %d, want 1", count)
		}

		count, err = pullSvc.ReviewCount(ctx, pr.ID, "changes_requested")
		if err != nil {
			t.Fatalf("ReviewCount: %v", err)
		}
		if count != 1 {
			t.Errorf("changes_requested count = %d, want 1", count)
		}
	})
}

func TestPullComment(t *testing.T) {
	_, _, pullSvc, u, r, ctx := setupPullTest(t)

	pr, err := pullSvc.Create(ctx, r.ID, u.ID, "comment-pr", "", "branch", "main", "sha")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("add comment with path and line", func(t *testing.T) {
		c, err := pullSvc.AddComment(ctx, pr.ID, u.ID, "Great code!", "file.go", 42)
		if err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		if c.PRID != pr.ID {
			t.Errorf("PRID mismatch")
		}
		if c.AuthorID != u.ID {
			t.Errorf("AuthorID mismatch")
		}
		if c.Body != "Great code!" {
			t.Errorf("Body = %q, want %q", c.Body, "Great code!")
		}
		if c.Path != "file.go" {
			t.Errorf("Path = %q, want %q", c.Path, "file.go")
		}
		if c.Line != 42 {
			t.Errorf("Line = %d, want 42", c.Line)
		}
		if c.Author == "" {
			t.Error("Author should be set (joined from users table)")
		}
	})

	t.Run("add comment without path and line", func(t *testing.T) {
		c, err := pullSvc.AddComment(ctx, pr.ID, u.ID, "General comment", "", 0)
		if err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		if c.Path != "" {
			t.Errorf("Path = %q, want empty", c.Path)
		}
		if c.Line != 0 {
			t.Errorf("Line = %d, want 0", c.Line)
		}
	})

	t.Run("list comments returns all in order", func(t *testing.T) {
		comments, err := pullSvc.ListComments(ctx, pr.ID)
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if len(comments) != 2 {
			t.Fatalf("expected 2 comments, got %d", len(comments))
		}
		if comments[0].Body != "Great code!" {
			t.Errorf("first comment body = %q, want %q", comments[0].Body, "Great code!")
		}
		if comments[1].Body != "General comment" {
			t.Errorf("second comment body = %q, want %q", comments[1].Body, "General comment")
		}
	})

	t.Run("list comments for non-existent PR returns empty", func(t *testing.T) {
		comments, err := pullSvc.ListComments(ctx, uuid.New())
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if len(comments) != 0 {
			t.Errorf("expected 0 comments, got %d", len(comments))
		}
	})
}

func TestPullFullLifecycle(t *testing.T) {
	_, _, pullSvc, u, r, ctx := setupPullTest(t)

	// Create a PR
	pr, err := pullSvc.Create(ctx, r.ID, u.ID, "lifecycle", "test body", "branch", "main", "abchead")
	if err != nil {
		t.Fatal(err)
	}

	// Add a review
	rev, err := pullSvc.AddReview(ctx, pr.ID, u.ID, "approved", "good")
	if err != nil {
		t.Fatal(err)
	}
	if rev.ID == uuid.Nil {
		t.Error("review ID should not be nil")
	}

	// Add a comment
	_, err = pullSvc.AddComment(ctx, pr.ID, u.ID, "comment text", "", 0)
	if err != nil {
		t.Fatal(err)
	}

	// Merge
	if err := pullSvc.Merge(ctx, pr.ID, "mergefinal"); err != nil {
		t.Fatal(err)
	}

	// Verify final state
	updated, err := pullSvc.Get(ctx, r.ID, pr.Number)
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != "closed" {
		t.Errorf("State = %q, want %q", updated.State, "closed")
	}
	if updated.MergeSHA != "mergefinal" {
		t.Errorf("MergeSHA = %q, want %q", updated.MergeSHA, "mergefinal")
	}
	if updated.MergedAt == nil {
		t.Error("MergedAt should be set")
	}

	// Verify reviews exist
	reviews, err := pullSvc.ListReviews(ctx, pr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 {
		t.Errorf("expected 1 review, got %d", len(reviews))
	}

	// Verify comments exist
	comments, err := pullSvc.ListComments(ctx, pr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(comments))
	}
}

func TestPullListReviewsEmpty(t *testing.T) {
	_, _, pullSvc, u, r, ctx := setupPullTest(t)

	pr, err := pullSvc.Create(ctx, r.ID, u.ID, "empty-reviews", "", "branch", "main", "sha")
	if err != nil {
		t.Fatal(err)
	}

	reviews, err := pullSvc.ListReviews(ctx, pr.ID)
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(reviews))
	}
}

func TestPullReviewCountZero(t *testing.T) {
	_, _, pullSvc, u, r, ctx := setupPullTest(t)

	pr, err := pullSvc.Create(ctx, r.ID, u.ID, "review-count-zero", "", "branch", "main", "sha")
	if err != nil {
		t.Fatal(err)
	}

	count, err := pullSvc.ReviewCount(ctx, pr.ID, "approved")
	if err != nil {
		t.Fatalf("ReviewCount: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 approved reviews, got %d", count)
	}
}

func TestPullMergeTimeIsRecent(t *testing.T) {
	_, _, pullSvc, u, r, ctx := setupPullTest(t)

	pr, err := pullSvc.Create(ctx, r.ID, u.ID, "merge-time", "", "branch", "main", "sha")
	if err != nil {
		t.Fatal(err)
	}

	beforeMerge := time.Now()
	if err := pullSvc.Merge(ctx, pr.ID, "mergesha"); err != nil {
		t.Fatal(err)
	}

	updated, err := pullSvc.Get(ctx, r.ID, pr.Number)
	if err != nil {
		t.Fatal(err)
	}
	if updated.MergedAt == nil {
		t.Fatal("MergedAt is nil")
	}
	if updated.MergedAt.Before(beforeMerge.Add(-time.Second)) {
		t.Errorf("MergedAt (%v) is before merge call (%v)", updated.MergedAt, beforeMerge)
	}
	if updated.MergedAt.After(time.Now().Add(time.Second)) {
		t.Errorf("MergedAt (%v) is in the future", updated.MergedAt)
	}
}
