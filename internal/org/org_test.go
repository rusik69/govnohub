package org

import (
	"context"
	"testing"

	"github.com/rusik69/govnohub/internal/auth"
	"github.com/rusik69/govnohub/internal/testutil"
)

func TestOrgAndTeam(t *testing.T) {
	pg := testutil.NewPostgres(t)
	defer pg.Cleanup()
	ctx := context.Background()
	authSvc := auth.NewService(pg.Pool, "s")
	orgSvc := NewService(pg.Pool)

	u, _ := authSvc.Register(ctx, "admin", "admin@test.local", "pass")
	o, err := orgSvc.Create(ctx, "acme", "ACME", "org")
	if err != nil {
		t.Fatal(err)
	}
	if err := orgSvc.AddMember(ctx, o.ID, u.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	team, err := orgSvc.CreateTeam(ctx, o.ID, "core", "core team")
	if err != nil {
		t.Fatal(err)
	}
	if err := orgSvc.AddTeamMember(ctx, team.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	orgs, _ := orgSvc.List(ctx)
	if len(orgs) != 1 {
		t.Fatalf("orgs=%d", len(orgs))
	}
}
