package issue

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/testutil"
)

func setup(t *testing.T) (context.Context, *Service, *auth.Service, uuid.UUID, uuid.UUID) {
	t.Helper()
	pg := testutil.NewPostgres(t)
	t.Cleanup(pg.Cleanup)
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	issueSvc := NewService(pg.Pool)

	u, err := authSvc.Register(ctx, "dev", "dev@test.local", "pass")
	if err != nil {
		t.Fatal(err)
	}
	r, err := repoSvc.Create(ctx, "user", u.ID, u.Username, "proj", "", false)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, issueSvc, authSvc, r.ID, u.ID
}

func TestCreate(t *testing.T) {
	ctx, svc, _, repoID, authorID := setup(t)

	t.Run("basic issue", func(t *testing.T) {
		i, err := svc.Create(ctx, repoID, authorID, "bug", "details")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if i.Number != 1 {
			t.Errorf("Number = %d, want 1", i.Number)
		}
		if i.Title != "bug" {
			t.Errorf("Title = %q, want %q", i.Title, "bug")
		}
		if i.Body != "details" {
			t.Errorf("Body = %q, want %q", i.Body, "details")
		}
		if i.State != "open" {
			t.Errorf("State = %q, want %q", i.State, "open")
		}
	})

	t.Run("auto-increment number", func(t *testing.T) {
		i1, _ := svc.Create(ctx, repoID, authorID, "first", "")
		i2, _ := svc.Create(ctx, repoID, authorID, "second", "")
		if i2.Number != i1.Number+1 {
			t.Errorf("expected sequential numbers, got %d then %d", i1.Number, i2.Number)
		}
	})
}

func TestGet(t *testing.T) {
	ctx, svc, _, repoID, authorID := setup(t)

	created, err := svc.Create(ctx, repoID, authorID, "bug", "details")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("existing issue by number", func(t *testing.T) {
		got, err := svc.Get(ctx, repoID, created.Number)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("ID mismatch")
		}
		if got.Title != "bug" {
			t.Errorf("Title = %q, want %q", got.Title, "bug")
		}
	})

	t.Run("non-existent number returns error", func(t *testing.T) {
		_, err := svc.Get(ctx, repoID, 9999)
		if err == nil {
			t.Fatal("expected error for non-existent issue")
		}
	})

	t.Run("wrong repo returns error", func(t *testing.T) {
		_, err := svc.Get(ctx, uuid.Nil, created.Number)
		if err == nil {
			t.Fatal("expected error for wrong repo")
		}
	})
}

func TestList(t *testing.T) {
	ctx, svc, _, repoID, authorID := setup(t)

	t.Run("empty list", func(t *testing.T) {
		issues, err := svc.List(ctx, repoID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(issues) != 0 {
			t.Errorf("expected empty list, got %d items", len(issues))
		}
	})

	svc.Create(ctx, repoID, authorID, "one", "")
	svc.Create(ctx, repoID, authorID, "two", "")

	t.Run("multiple issues", func(t *testing.T) {
		issues, err := svc.List(ctx, repoID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(issues) != 2 {
			t.Errorf("expected 2 issues, got %d", len(issues))
		}
		if issues[0].Number != 2 { // descending order
			t.Errorf("expected newest first, got number %d", issues[0].Number)
		}
	})
}

func TestClose(t *testing.T) {
	ctx, svc, _, repoID, authorID := setup(t)

	i, err := svc.Create(ctx, repoID, authorID, "to-close", "")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("close open issue", func(t *testing.T) {
		if err := svc.Close(ctx, i.ID); err != nil {
			t.Fatalf("Close: %v", err)
		}
		got, _ := svc.Get(ctx, repoID, i.Number)
		if got.State != "closed" {
			t.Errorf("State = %q, want %q", got.State, "closed")
		}
	})

	t.Run("close already closed issue", func(t *testing.T) {
		if err := svc.Close(ctx, i.ID); err != nil {
			t.Fatalf("Close (already closed): %v", err)
		}
		got, _ := svc.Get(ctx, repoID, i.Number)
		if got.State != "closed" {
			t.Errorf("State = %q, want %q", got.State, "closed")
		}
	})
}

func TestAddComment(t *testing.T) {
	ctx, svc, _, repoID, authorID := setup(t)

	i, err := svc.Create(ctx, repoID, authorID, "comment-test", "")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("add comment to issue", func(t *testing.T) {
		c, err := svc.AddComment(ctx, i.ID, authorID, "a comment")
		if err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		if c.Body != "a comment" {
			t.Errorf("Body = %q, want %q", c.Body, "a comment")
		}
		if c.IssueID != i.ID {
			t.Errorf("IssueID mismatch")
		}
	})

	t.Run("add comment with empty body", func(t *testing.T) {
		_, err := svc.AddComment(ctx, i.ID, authorID, "")
		if err != nil {
			t.Fatalf("AddComment with empty body: %v", err)
		}
	})
}

func TestListComments(t *testing.T) {
	ctx, svc, _, repoID, authorID := setup(t)

	i, err := svc.Create(ctx, repoID, authorID, "comments", "")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("empty comments list", func(t *testing.T) {
		comments, err := svc.ListComments(ctx, i.ID)
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if len(comments) != 0 {
			t.Errorf("expected empty comments, got %d", len(comments))
		}
	})

	svc.AddComment(ctx, i.ID, authorID, "first")
	svc.AddComment(ctx, i.ID, authorID, "second")

	t.Run("multiple comments", func(t *testing.T) {
		comments, err := svc.ListComments(ctx, i.ID)
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if len(comments) != 2 {
			t.Errorf("expected 2 comments, got %d", len(comments))
		}
		if comments[0].Body != "first" {
			t.Errorf("expected first comment first, got %q", comments[0].Body)
		}
	})
}

func TestSetAssignee(t *testing.T) {
	ctx, svc, authSvc, repoID, authorID := setup(t)

	i, err := svc.Create(ctx, repoID, authorID, "assignee-test", "")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("set assignee", func(t *testing.T) {
		assigneeID := authorID // self-assign
		if err := svc.SetAssignee(ctx, i.ID, &assigneeID); err != nil {
			t.Fatalf("SetAssignee: %v", err)
		}
		got, _ := svc.Get(ctx, repoID, i.Number)
		if got.AssigneeID == nil || *got.AssigneeID != assigneeID {
			t.Errorf("expected assignee %v, got %v", assigneeID, got.AssigneeID)
		}
	})

	t.Run("clear assignee", func(t *testing.T) {
		if err := svc.SetAssignee(ctx, i.ID, nil); err != nil {
			t.Fatalf("SetAssignee(nil): %v", err)
		}
		got, _ := svc.Get(ctx, repoID, i.Number)
		if got.AssigneeID != nil {
			t.Errorf("expected nil assignee, got %v", got.AssigneeID)
		}
	})
	_ = authSvc
}

func TestLabelOperations(t *testing.T) {
	ctx, svc, _, repoID, authorID := setup(t)

	i, err := svc.Create(ctx, repoID, authorID, "labels", "")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("create label", func(t *testing.T) {
		l, err := svc.CreateLabel(ctx, repoID, "bug", "ff0000")
		if err != nil {
			t.Fatalf("CreateLabel: %v", err)
		}
		if l.Name != "bug" {
			t.Errorf("Name = %q, want %q", l.Name, "bug")
		}
		if l.Color != "ff0000" {
			t.Errorf("Color = %q, want %q", l.Color, "ff0000")
		}
	})

	l, _ := svc.CreateLabel(ctx, repoID, "enhancement", "00ff00")

	t.Run("list labels", func(t *testing.T) {
		labels, err := svc.ListLabels(ctx, repoID)
		if err != nil {
			t.Fatalf("ListLabels: %v", err)
		}
		if len(labels) < 1 {
			t.Fatal("expected at least 1 label")
		}
		found := false
		for _, lb := range labels {
			if lb.Name == "enhancement" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected 'enhancement' label in list")
		}
	})

	t.Run("add label to issue", func(t *testing.T) {
		if err := svc.AddLabel(ctx, i.ID, l.ID); err != nil {
			t.Fatalf("AddLabel: %v", err)
		}
		got, _ := svc.Get(ctx, repoID, i.Number)
		if len(got.Labels) != 1 {
			t.Errorf("expected 1 label on issue, got %d", len(got.Labels))
		}
		if got.Labels[0].Name != "enhancement" {
			t.Errorf("Label.Name = %q, want %q", got.Labels[0].Name, "enhancement")
		}
	})

	t.Run("add label idempotent (duplicate)", func(t *testing.T) {
		if err := svc.AddLabel(ctx, i.ID, l.ID); err != nil {
			t.Fatalf("AddLabel (duplicate): %v", err)
		}
		got, _ := svc.Get(ctx, repoID, i.Number)
		if len(got.Labels) != 1 {
			t.Errorf("expected still 1 label after duplicate add, got %d", len(got.Labels))
		}
	})

	t.Run("remove label from issue", func(t *testing.T) {
		if err := svc.RemoveLabel(ctx, i.ID, l.ID); err != nil {
			t.Fatalf("RemoveLabel: %v", err)
		}
		got, _ := svc.Get(ctx, repoID, i.Number)
		if len(got.Labels) != 0 {
			t.Errorf("expected 0 labels after remove, got %d", len(got.Labels))
		}
	})
}

func TestSetMilestone(t *testing.T) {
	ctx, svc, _, repoID, authorID := setup(t)

	i, err := svc.Create(ctx, repoID, authorID, "milestone-test", "")
	if err != nil {
		t.Fatal(err)
	}

	m, err := svc.CreateMilestone(ctx, repoID, "v1.0", "first release", nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("set milestone on issue", func(t *testing.T) {
		if err := svc.SetMilestone(ctx, i.ID, &m.ID); err != nil {
			t.Fatalf("SetMilestone: %v", err)
		}
		got, _ := svc.Get(ctx, repoID, i.Number)
		if got.MilestoneID == nil || *got.MilestoneID != m.ID {
			t.Errorf("expected milestone %v, got %v", m.ID, got.MilestoneID)
		}
	})

	t.Run("clear milestone", func(t *testing.T) {
		if err := svc.SetMilestone(ctx, i.ID, nil); err != nil {
			t.Fatalf("SetMilestone(nil): %v", err)
		}
		got, _ := svc.Get(ctx, repoID, i.Number)
		if got.MilestoneID != nil {
			t.Errorf("expected nil milestone, got %v", got.MilestoneID)
		}
	})
}

func TestMilestoneLifecycle(t *testing.T) {
	ctx, svc, _, repoID, _ := setup(t)

	t.Run("create and list milestones", func(t *testing.T) {
		m, err := svc.CreateMilestone(ctx, repoID, "v2.0", "next", nil)
		if err != nil {
			t.Fatalf("CreateMilestone: %v", err)
		}
		if m.Title != "v2.0" {
			t.Errorf("Title = %q, want %q", m.Title, "v2.0")
		}
		if m.State != "open" {
			t.Errorf("State = %q, want %q", m.State, "open")
		}
	})

	t.Run("get milestone", func(t *testing.T) {
		ms, _ := svc.ListMilestones(ctx, repoID)
		if len(ms) == 0 {
			t.Fatal("expected milestones")
		}
		got, err := svc.GetMilestone(ctx, repoID, ms[0].ID)
		if err != nil {
			t.Fatalf("GetMilestone: %v", err)
		}
		if got.ID != ms[0].ID {
			t.Errorf("ID mismatch")
		}
	})

	t.Run("close milestone", func(t *testing.T) {
		ms, _ := svc.ListMilestones(ctx, repoID)
		if len(ms) == 0 {
			t.Fatal("expected milestones")
		}
		if err := svc.CloseMilestone(ctx, ms[0].ID); err != nil {
			t.Fatalf("CloseMilestone: %v", err)
		}
		got, _ := svc.GetMilestone(ctx, repoID, ms[0].ID)
		if got.State != "closed" {
			t.Errorf("State = %q, want %q", got.State, "closed")
		}
	})
}

func TestIssueLifecycle(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	repoSvc := repo.NewService(pg.Pool)
	issueSvc := NewService(pg.Pool)

	u, _ := authSvc.Register(ctx, "dev", "dev@test.local", "pass")
	r, _ := repoSvc.Create(ctx, "user", u.ID, u.Username, "proj", "", false)

	i, err := issueSvc.Create(ctx, r.ID, u.ID, "bug", "details")
	if err != nil || i.Number != 1 {
		t.Fatalf("create: %v %+v", err, i)
	}
	if _, err := issueSvc.AddComment(ctx, i.ID, u.ID, "fix soon"); err != nil {
		t.Fatal(err)
	}
	if err := issueSvc.Close(ctx, i.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := issueSvc.List(ctx, r.ID)
	if len(list) != 1 || list[0].State != "closed" {
		t.Fatalf("list=%+v", list)
	}
}
