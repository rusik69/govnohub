-- Database index review: add indices for commonly queried patterns
-- Discovered by auditing all SQL query patterns in the codebase.
-- Many tables lacked covering indices for WHERE/ORDER BY clauses.
-- Each index is idempotent (IF NOT EXISTS).

-- issue_comments: queried by issue_id ORDER BY created_at ASC
CREATE INDEX IF NOT EXISTS idx_issue_comments_issue_created
    ON issue_comments(issue_id, created_at);

-- pr_reviews: queried by pr_id ORDER BY created_at ASC, and counted by state
CREATE INDEX IF NOT EXISTS idx_pr_reviews_pr_created
    ON pr_reviews(pr_id, created_at);
CREATE INDEX IF NOT EXISTS idx_pr_reviews_pr_state
    ON pr_reviews(pr_id, state);

-- pr_comments: queried by pr_id ORDER BY created_at ASC
CREATE INDEX IF NOT EXISTS idx_pr_comments_pr_created
    ON pr_comments(pr_id, created_at);

-- personal_access_tokens: queried by user_id ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_pat_user_created
    ON personal_access_tokens(user_id, created_at DESC);

-- webhooks: queried by repo_id (no index existed on this FK column)
CREATE INDEX IF NOT EXISTS idx_webhooks_repo
    ON webhooks(repo_id);

-- webhook_deliveries: queried by webhook_id ORDER BY delivered_at DESC
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook_delivered
    ON webhook_deliveries(webhook_id, delivered_at DESC);

-- releases: queried by repo_id ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_releases_repo_created
    ON releases(repo_id, created_at DESC);

-- release_assets: queried by release_id ORDER BY name
CREATE INDEX IF NOT EXISTS idx_release_assets_release_name
    ON release_assets(release_id, name);

-- notifications: queried by user_id ORDER BY created_at DESC
-- (existing partial index covers only user_id, read WHERE read=false)
CREATE INDEX IF NOT EXISTS idx_notifications_user_created
    ON notifications(user_id, created_at DESC);

-- milestones: queried by repo_id ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_milestones_repo_created
    ON milestones(repo_id, created_at DESC);

-- audit_log: commonly queried ORDER BY created_at DESC (no index on timestamp)
CREATE INDEX IF NOT EXISTS idx_audit_log_created
    ON audit_log(created_at DESC);

-- repos: queried by user/org with ORDER BY updated_at DESC
CREATE INDEX IF NOT EXISTS idx_repos_owner_updated
    ON repos(owner_type, owner_id, updated_at DESC);

-- repo_collaborators: queried by user_id for accessible-repo subqueries
CREATE INDEX IF NOT EXISTS idx_repo_collaborators_user
    ON repo_collaborators(user_id);

-- workflow_runs: queried by head_sha for CI status checks
CREATE INDEX IF NOT EXISTS idx_workflow_runs_head_sha
    ON workflow_runs(head_sha);

-- organizations: queried by user_id to list user's orgs (no index on FK)
CREATE INDEX IF NOT EXISTS idx_org_members_user
    ON org_members(user_id);
