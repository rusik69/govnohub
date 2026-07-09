package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/rusik69/govnohub/internal/db"
)

type Postgres struct {
	Pool        *pgxpool.Pool
	DatabaseURL string
	Cleanup     func()
}

func NewPostgres(t *testing.T) *Postgres {
	t.Helper()
	ctx := context.Background()

	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return connectPostgres(t, ctx, url, nil)
	}

	pg, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("govnohub"),
		postgres.WithUsername("govnohub"),
		postgres.WithPassword("govnohub"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Skipf("postgres container unavailable (set TEST_DATABASE_URL to use external DB): %v", err)
	}

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	term := func() {
		if err := pg.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "terminate postgres: %v\n", err)
		}
	}
	return connectPostgres(t, ctx, connStr, term)
}

func connectPostgres(t *testing.T, ctx context.Context, connStr string, afterClose func()) *Postgres {
	t.Helper()
	pool, err := db.Connect(ctx, connStr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cleanup := func() {
		pool.Close()
		if afterClose != nil {
			afterClose()
		}
	}
	return &Postgres{Pool: pool, DatabaseURL: connStr, Cleanup: cleanup}
}
