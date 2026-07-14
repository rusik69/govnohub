package web

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rusik69/govnohub/internal/aireview"
	"github.com/rusik69/govnohub/internal/audit"
	"github.com/rusik69/govnohub/internal/auth"
	gitstore "github.com/rusik69/govnohub/internal/git"
	"github.com/rusik69/govnohub/internal/issue"
	"github.com/rusik69/govnohub/internal/notification"
	"github.com/rusik69/govnohub/internal/org"
	pkg "github.com/rusik69/govnohub/internal/package"
	"github.com/rusik69/govnohub/internal/presence"
	"github.com/rusik69/govnohub/internal/pull"
	"github.com/rusik69/govnohub/internal/release"
	"github.com/rusik69/govnohub/internal/repo"
	"github.com/rusik69/govnohub/internal/search"
	"github.com/rusik69/govnohub/internal/webhook"
	"github.com/rusik69/govnohub/internal/wiki"
)

type Deps struct {
	Auth     *auth.Service
	Repos    *repo.Service
	Git      *gitstore.Store
	Issues   *issue.Service
	Pulls    *pull.Service
	Releases *release.Service
	Packages *pkg.Service
	Webhooks *webhook.Service
	Search   *search.Service
	Pool     *pgxpool.Pool
	Org      *org.Service
	AIReview *aireview.Service
	Notify   *notification.Service
	Wiki     *wiki.Service
	Audit    *audit.Service
	Presence *presence.Tracker
	UploadDir string
}
