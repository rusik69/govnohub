package org

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/testutil"
)

func TestOrgCreate(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool)

	t.Run("create org", func(t *testing.T) {
		o, err := svc.Create(ctx, "testorg", "Test Org", "A test org")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if o.ID == uuid.Nil {
			t.Fatal("expected non-nil ID")
		}
		if o.Name != "testorg" {
			t.Errorf("name = %q, want %q", o.Name, "testorg")
		}
		if o.DisplayName != "Test Org" {
			t.Errorf("display_name = %q, want %q", o.DisplayName, "Test Org")
		}
		if o.Description != "A test org" {
			t.Errorf("description = %q, want %q", o.Description, "A test org")
		}
		if o.CreatedAt.IsZero() {
			t.Fatal("expected non-zero created_at")
		}
	})

	t.Run("duplicate name", func(t *testing.T) {
		_, err := svc.Create(ctx, "testorg", "dup", "dup desc")
		if err == nil {
			t.Fatal("expected error for duplicate org name")
		}
		if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "unique") {
			t.Logf("got error (non-critical message check): %v", err)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		o, err := svc.Create(ctx, "", "No Name", "")
		if err != nil {
			t.Fatalf("Create with empty name: %v", err)
		}
		if o.Name != "" {
			t.Errorf("name = %q, want empty", o.Name)
		}
	})
}

func TestOrgList(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool)

	t.Run("list empty", func(t *testing.T) {
		orgs, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(orgs) != 0 {
			t.Fatalf("expected 0 orgs, got %d", len(orgs))
		}
	})

	t.Run("list with orgs", func(t *testing.T) {
		svc.Create(ctx, "org1", "Org One", "")
		svc.Create(ctx, "org2", "Org Two", "")
		orgs, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(orgs) != 2 {
			t.Fatalf("expected 2 orgs, got %d", len(orgs))
		}
		names := make(map[string]bool)
		for _, o := range orgs {
			names[o.Name] = true
		}
		if !names["org1"] || !names["org2"] {
			t.Errorf("missing expected orgs, got %v", names)
		}
	})
}

func TestGetByName(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool)

	t.Run("existing org", func(t *testing.T) {
		created, err := svc.Create(ctx, "getbyname", "Get By Name", "")
		if err != nil {
			t.Fatal(err)
		}
		got, err := svc.GetByName(ctx, "getbyname")
		if err != nil {
			t.Fatalf("GetByName: %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("id = %v, want %v", got.ID, created.ID)
		}
	})

	t.Run("non-existent org", func(t *testing.T) {
		_, err := svc.GetByName(ctx, "nonexistent")
		if err == nil {
			t.Fatal("expected error for non-existent org")
		}
		if err.Error() != "org not found" {
			t.Errorf("error = %q, want %q", err.Error(), "org not found")
		}
	})
}

func TestAddMemberAndIsMember(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	svc := NewService(pg.Pool)

	u, err := authSvc.Register(ctx, "memberuser", "member@test.local", "pass")
	if err != nil {
		t.Fatal(err)
	}
	o, err := svc.Create(ctx, "memberorg", "Member Org", "")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("add member", func(t *testing.T) {
		if err := svc.AddMember(ctx, o.ID, u.ID, "admin"); err != nil {
			t.Fatalf("AddMember: %v", err)
		}
	})

	t.Run("is member true", func(t *testing.T) {
		ok, err := svc.IsMember(ctx, o.ID, u.ID)
		if err != nil {
			t.Fatalf("IsMember: %v", err)
		}
		if !ok {
			t.Fatal("expected IsMember=true")
		}
	})

	t.Run("is member false", func(t *testing.T) {
		ok, err := svc.IsMember(ctx, o.ID, uuid.New())
		if err != nil {
			t.Fatalf("IsMember: %v", err)
		}
		if ok {
			t.Fatal("expected IsMember=false")
		}
	})

	t.Run("add duplicate member is idempotent", func(t *testing.T) {
		if err := svc.AddMember(ctx, o.ID, u.ID, "member"); err != nil {
			t.Fatalf("AddMember duplicate: %v", err)
		}
	})

	t.Run("is member for non-existent org", func(t *testing.T) {
		ok, err := svc.IsMember(ctx, uuid.New(), u.ID)
		if err != nil {
			t.Fatalf("IsMember: %v", err)
		}
		if ok {
			t.Fatal("expected IsMember=false for non-existent org")
		}
	})
}

func TestListMembers(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	svc := NewService(pg.Pool)

	alice, _ := authSvc.Register(ctx, "alice", "alice@test.local", "pass")
	bob, _ := authSvc.Register(ctx, "bob", "bob@test.local", "pass")
	o, _ := svc.Create(ctx, "memberlist", "Member List", "")

	t.Run("list members empty", func(t *testing.T) {
		members, err := svc.ListMembers(ctx, o.ID)
		if err != nil {
			t.Fatalf("ListMembers: %v", err)
		}
		if len(members) != 0 {
			t.Fatalf("expected 0 members, got %d", len(members))
		}
	})

	t.Run("list members with members", func(t *testing.T) {
		svc.AddMember(ctx, o.ID, alice.ID, "admin")
		svc.AddMember(ctx, o.ID, bob.ID, "member")

		members, err := svc.ListMembers(ctx, o.ID)
		if err != nil {
			t.Fatalf("ListMembers: %v", err)
		}
		if len(members) != 2 {
			t.Fatalf("expected 2 members, got %d", len(members))
		}
		// Should be sorted by username: alice, bob
		if members[0].Username != "alice" {
			t.Errorf("first member = %q, want %q", members[0].Username, "alice")
		}
		if members[1].Username != "bob" {
			t.Errorf("second member = %q, want %q", members[1].Username, "bob")
		}
		if members[0].Role != "admin" {
			t.Errorf("alice role = %q, want %q", members[0].Role, "admin")
		}
		if members[1].Role != "member" {
			t.Errorf("bob role = %q, want %q", members[1].Role, "member")
		}
	})
}

func TestListForUser(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	svc := NewService(pg.Pool)

	u, _ := authSvc.Register(ctx, "orguser", "orguser@test.local", "pass")

	t.Run("no orgs for user", func(t *testing.T) {
		orgs, err := svc.ListForUser(ctx, u.ID)
		if err != nil {
			t.Fatalf("ListForUser: %v", err)
		}
		if len(orgs) != 0 {
			t.Fatalf("expected 0 orgs, got %d", len(orgs))
		}
	})

	t.Run("list orgs for user", func(t *testing.T) {
		o1, _ := svc.Create(ctx, "userorg1", "User Org 1", "")
		o2, _ := svc.Create(ctx, "userorg2", "User Org 2", "")
		svc.AddMember(ctx, o1.ID, u.ID, "admin")
		svc.AddMember(ctx, o2.ID, u.ID, "member")

		orgs, err := svc.ListForUser(ctx, u.ID)
		if err != nil {
			t.Fatalf("ListForUser: %v", err)
		}
		if len(orgs) != 2 {
			t.Fatalf("expected 2 orgs, got %d", len(orgs))
		}
		// Should be sorted by name
		if orgs[0].Name != "userorg1" {
			t.Errorf("first org = %q, want %q", orgs[0].Name, "userorg1")
		}
		if orgs[1].Name != "userorg2" {
			t.Errorf("second org = %q, want %q", orgs[1].Name, "userorg2")
		}
	})
}

func TestCreateTeam(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool)

	o, _ := svc.Create(ctx, "teamorg", "Team Org", "")

	t.Run("create team", func(t *testing.T) {
		team, err := svc.CreateTeam(ctx, o.ID, "core", "Core team")
		if err != nil {
			t.Fatalf("CreateTeam: %v", err)
		}
		if team.ID == uuid.Nil {
			t.Fatal("expected non-nil team ID")
		}
		if team.OrgID != o.ID {
			t.Errorf("org_id = %v, want %v", team.OrgID, o.ID)
		}
		if team.Name != "core" {
			t.Errorf("name = %q, want %q", team.Name, "core")
		}
		if team.Description != "Core team" {
			t.Errorf("description = %q, want %q", team.Description, "Core team")
		}
	})

	t.Run("duplicate team name in same org", func(t *testing.T) {
		_, err := svc.CreateTeam(ctx, o.ID, "core", "duplicate")
		if err == nil {
			t.Fatal("expected error for duplicate team name")
		}
	})

	t.Run("same team name in different org", func(t *testing.T) {
		o2, _ := svc.Create(ctx, "teamorg2", "Team Org 2", "")
		team, err := svc.CreateTeam(ctx, o2.ID, "core", "Core in other org")
		if err != nil {
			t.Fatalf("CreateTeam in different org: %v", err)
		}
		if team.Name != "core" {
			t.Errorf("name = %q, want %q", team.Name, "core")
		}
	})
}

func TestListTeams(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool)

	o, _ := svc.Create(ctx, "teamlist", "Team List", "")

	t.Run("list teams empty", func(t *testing.T) {
		teams, err := svc.ListTeams(ctx, o.ID)
		if err != nil {
			t.Fatalf("ListTeams: %v", err)
		}
		if len(teams) != 0 {
			t.Fatalf("expected 0 teams, got %d", len(teams))
		}
	})

	t.Run("list teams with teams", func(t *testing.T) {
		svc.CreateTeam(ctx, o.ID, "alpha", "Alpha team")
		svc.CreateTeam(ctx, o.ID, "beta", "Beta team")
		teams, err := svc.ListTeams(ctx, o.ID)
		if err != nil {
			t.Fatalf("ListTeams: %v", err)
		}
		if len(teams) != 2 {
			t.Fatalf("expected 2 teams, got %d", len(teams))
		}
		// Should be sorted by name
		if teams[0].Name != "alpha" {
			t.Errorf("first team = %q, want %q", teams[0].Name, "alpha")
		}
		if teams[1].Name != "beta" {
			t.Errorf("second team = %q, want %q", teams[1].Name, "beta")
		}
	})
}

func TestAddTeamMember(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	svc := NewService(pg.Pool)

	u, _ := authSvc.Register(ctx, "teammember", "teammember@test.local", "pass")
	o, _ := svc.Create(ctx, "teamadd", "Team Add", "")
	team, _ := svc.CreateTeam(ctx, o.ID, "dev", "Dev team")

	t.Run("add team member", func(t *testing.T) {
		if err := svc.AddTeamMember(ctx, team.ID, u.ID); err != nil {
			t.Fatalf("AddTeamMember: %v", err)
		}
	})

	t.Run("add duplicate team member is idempotent", func(t *testing.T) {
		if err := svc.AddTeamMember(ctx, team.ID, u.ID); err != nil {
			t.Fatalf("AddTeamMember duplicate: %v", err)
		}
	})

	t.Run("list team members", func(t *testing.T) {
		members, err := svc.ListTeamMembers(ctx, team.ID)
		if err != nil {
			t.Fatalf("ListTeamMembers: %v", err)
		}
		if len(members) != 1 {
			t.Fatalf("expected 1 member, got %d", len(members))
		}
		if members[0].Username != "teammember" {
			t.Errorf("username = %q, want %q", members[0].Username, "teammember")
		}
	})
}

func TestOrgGetTeam(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool)

	o, _ := svc.Create(ctx, "getteamorg", "Get Team", "")

	t.Run("get existing team", func(t *testing.T) {
		created, _ := svc.CreateTeam(ctx, o.ID, "myteam", "My team")
		got, err := svc.GetTeam(ctx, o.ID, "myteam")
		if err != nil {
			t.Fatalf("GetTeam: %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("id = %v, want %v", got.ID, created.ID)
		}
		if got.Name != "myteam" {
			t.Errorf("name = %q, want %q", got.Name, "myteam")
		}
	})

	t.Run("get non-existent team", func(t *testing.T) {
		_, err := svc.GetTeam(ctx, o.ID, "nonexistent")
		if err == nil {
			t.Fatal("expected error for non-existent team")
		}
		if err.Error() != "team not found" {
			t.Errorf("error = %q, want %q", err.Error(), "team not found")
		}
	})
}

func TestOrgFullLifecycle(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	svc := NewService(pg.Pool)

	// Create two users
	alice, _ := authSvc.Register(ctx, "alice", "alice@lifecycle.test", "pass")
	bob, _ := authSvc.Register(ctx, "bob", "bob@lifecycle.test", "pass")

	// Create an org
	org, err := svc.Create(ctx, "lifecycle", "Lifecycle Org", "Test lifecycle")
	if err != nil {
		t.Fatal(err)
	}

	// Add alice as admin
	if err := svc.AddMember(ctx, org.ID, alice.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	// Add bob as member
	if err := svc.AddMember(ctx, org.ID, bob.ID, "member"); err != nil {
		t.Fatal(err)
	}

	// Verify membership
	if ok, _ := svc.IsMember(ctx, org.ID, alice.ID); !ok {
		t.Fatal("alice should be member")
	}
	if ok, _ := svc.IsMember(ctx, org.ID, bob.ID); !ok {
		t.Fatal("bob should be member")
	}
	if ok, _ := svc.IsMember(ctx, org.ID, uuid.New()); ok {
		t.Fatal("random user should not be member")
	}

	// Create teams
	team, err := svc.CreateTeam(ctx, org.ID, "engineers", "Engineering")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTeam(ctx, org.ID, "managers", "Management"); err != nil {
		t.Fatal(err)
	}

	// Add team members
	if err := svc.AddTeamMember(ctx, team.ID, alice.ID); err != nil {
		t.Fatal(err)
	}

	// List orgs for user
	userOrgs, err := svc.ListForUser(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(userOrgs) != 1 || userOrgs[0].Name != "lifecycle" {
		t.Fatalf("ListForUser: expected [lifecycle], got %v", userOrgs)
	}

	// List members
	members, err := svc.ListMembers(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	// List teams
	teams, err := svc.ListTeams(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}

	// Get team by name
	gotTeam, err := svc.GetTeam(ctx, org.ID, "engineers")
	if err != nil {
		t.Fatal(err)
	}
	if gotTeam.ID != team.ID {
		t.Errorf("GetTeam returned wrong team")
	}

	// List team members
	teamMembers, err := svc.ListTeamMembers(ctx, team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teamMembers) != 1 || teamMembers[0].Username != "alice" {
		t.Fatalf("ListTeamMembers: expected [alice], got %v", teamMembers)
	}
}

func TestOrgEmptyDisplayName(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	svc := NewService(pg.Pool)

	// Create with empty display name and description
	o, err := svc.Create(ctx, "emptydisplay", "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if o.DisplayName != "" {
		t.Errorf("expected empty display name, got %q", o.DisplayName)
	}

	// GetByName should return same
	got, err := svc.GetByName(ctx, "emptydisplay")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.DisplayName != "" {
		t.Errorf("expected empty display name from GetByName, got %q", got.DisplayName)
	}
}
