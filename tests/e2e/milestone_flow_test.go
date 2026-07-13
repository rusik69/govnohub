//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/rusik69/govnohub/internal/testutil"
	"github.com/rusik69/govnohub/tests/testenv"
)

func TestMilestoneFlow_CreateListGetClose(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "msf" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "ms-repo")
	repo := owner + "/ms-repo"

	// ---- Create milestone with title and description ----
	t.Run("create milestone", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/milestones", token, map[string]string{
			"title": "v1.0", "description": "First major release",
		})
		requireStatus(t, resp, http.StatusOK, "create milestone")
		if out["title"] != "v1.0" {
			t.Fatalf("expected title v1.0, got %q", out["title"])
		}
		if out["state"] != "open" {
			t.Fatalf("expected state open, got %q", out["state"])
		}
		id, ok := out["id"].(string)
		if !ok || id == "" {
			t.Fatal("missing milestone id")
		}
		if out["description"] != "First major release" {
			t.Fatalf("expected description 'First major release', got %q", out["description"])
		}
	})

	// ---- Create a second milestone ----
	t.Run("create second milestone", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/milestones", token, map[string]string{
			"title": "v2.0",
		})
		requireStatus(t, resp, http.StatusOK, "create milestone 2")
		if out["title"] != "v2.0" {
			t.Fatalf("expected title v2.0, got %q", out["title"])
		}
	})

	// ---- List milestones ----
	t.Run("list milestones", func(t *testing.T) {
		resp, milestones := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/milestones", token, nil)
		requireStatus(t, resp, http.StatusOK, "list milestones")
		if len(milestones) < 2 {
			t.Fatalf("expected at least 2 milestones, got %d", len(milestones))
		}
	})

	// ---- Create milestone, get by ID ----
	t.Run("get milestone by id", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/milestones", token, map[string]string{
			"title": "sprint-1", "description": "Sprint one",
		})
		requireStatus(t, resp, http.StatusOK, "create milestone for get")
		msID, ok := out["id"].(string)
		if !ok || msID == "" {
			t.Fatal("missing milestone id")
		}

		resp, out = testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/milestones/"+msID, token, nil)
		requireStatus(t, resp, http.StatusOK, "get milestone")
		if out["title"] != "sprint-1" {
			t.Fatalf("expected title 'sprint-1', got %q", out["title"])
		}
		if out["description"] != "Sprint one" {
			t.Fatalf("expected description 'Sprint one', got %q", out["description"])
		}
		if out["state"] != "open" {
			t.Fatalf("expected state 'open', got %q", out["state"])
		}
	})

	// ---- Close milestone ----
	t.Run("close milestone", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/milestones", token, map[string]string{
			"title": "to-close",
		})
		requireStatus(t, resp, http.StatusOK, "create milestone to close")
		msID, ok := out["id"].(string)
		if !ok || msID == "" {
			t.Fatal("missing milestone id")
		}

		resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/milestones/"+msID+"/close", token, nil)
		requireStatus(t, resp, http.StatusOK, "close milestone")
		if out["status"] != "closed" {
			t.Fatalf("expected status 'closed', got %q", out["status"])
		}

		// Verify closed when listing
		resp, milestones := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/milestones", token, nil)
		requireStatus(t, resp, http.StatusOK, "list after close")
		found := false
		for _, ms := range milestones {
			if ms["title"] == "to-close" {
				if ms["state"] != "closed" {
					t.Fatalf("expected closed milestone, got state=%v", ms["state"])
				}
				found = true
			}
		}
		if !found {
			t.Fatal("closed milestone not found in list")
		}
	})
}

func TestMilestoneFlow_IssueAssignment(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "msia" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "ia-repo")
	repo := owner + "/ia-repo"

	// Create an issue
	resp, out := testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/issues", token, map[string]string{
		"title": "track in milestone", "body": "needs to be assigned to a milestone",
	})
	requireStatus(t, resp, http.StatusOK, "create issue")
	issueNum := int(out["number"].(float64))

	// Create a milestone
	resp, out = testutil.DoJSON(t, http.MethodPost, base+"/api/v1/repos/"+repo+"/milestones", token, map[string]string{
		"title": "sprint-2", "description": "Second sprint",
	})
	requireStatus(t, resp, http.StatusOK, "create milestone for assignment")
	msID, ok := out["id"].(string)
	if !ok || msID == "" {
		t.Fatal("missing milestone id")
	}

	// Assign milestone to issue via PATCH
	t.Run("assign milestone to issue", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPatch, base+"/api/v1/repos/"+repo+"/issues/"+itoa(issueNum), token, map[string]any{
			"milestone_id": msID,
		})
		requireStatus(t, resp, http.StatusOK, "assign milestone")
		if out["milestone_id"] != msID {
			t.Fatalf("expected milestone_id %q, got %q", msID, out["milestone_id"])
		}
		if out["milestone"] != "sprint-2" {
			t.Fatalf("expected milestone title 'sprint-2', got %q", out["milestone"])
		}
	})

	// Verify milestone on issue GET
	t.Run("verify milestone on issue get", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/issues/"+itoa(issueNum), token, nil)
		requireStatus(t, resp, http.StatusOK, "get issue")
		if out["milestone_id"] != msID {
			t.Fatalf("expected milestone_id %q, got %q", msID, out["milestone_id"])
		}
		if out["milestone"] != "sprint-2" {
			t.Fatalf("expected milestone title 'sprint-2', got %q", out["milestone"])
		}
	})

	// Clear milestone from issue
	t.Run("clear milestone from issue", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodPatch, base+"/api/v1/repos/"+repo+"/issues/"+itoa(issueNum), token, map[string]any{
			"milestone_id": nil,
		})
		requireStatus(t, resp, http.StatusOK, "clear milestone")
		if out["milestone_id"] != nil {
			t.Fatalf("expected nil milestone_id, got %v", out["milestone_id"])
		}
		if out["milestone"] != "" {
			t.Fatalf("expected empty milestone title, got %q", out["milestone"])
		}
	})

	// Verify cleared milestone on issue GET
	t.Run("verify cleared milestone", func(t *testing.T) {
		resp, out := testutil.DoJSON(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/issues/"+itoa(issueNum), token, nil)
		requireStatus(t, resp, http.StatusOK, "get issue after clear")
		if out["milestone_id"] != nil {
			t.Fatalf("expected nil milestone_id after clear, got %v", out["milestone_id"])
		}
	})
}

func TestMilestoneFlow_EmptyList(t *testing.T) {
	env := testenv.New(t)
	defer env.Cleanup()
	base := env.URL
	owner := "msel" + uniqueSuffix()[len(uniqueSuffix())-6:]
	token := testutil.RegisterAndLogin(t, base, owner)
	createRepo(t, base, token, owner, "empty-ms")
	repo := owner + "/empty-ms"

	t.Run("list milestones on repo with none", func(t *testing.T) {
		resp, milestones := doJSONArray(t, http.MethodGet, base+"/api/v1/repos/"+repo+"/milestones", token, nil)
		requireStatus(t, resp, http.StatusOK, "list empty milestones")
		if len(milestones) != 0 {
			t.Fatalf("expected 0 milestones on empty repo, got %d", len(milestones))
		}
	})
}
