CREATE TABLE IF NOT EXISTS issue_reactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    comment_id UUID REFERENCES issue_comments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One reaction per user per emoji per issue
CREATE UNIQUE INDEX IF NOT EXISTS idx_issue_reactions_issue_unique
    ON issue_reactions(issue_id, user_id, content) WHERE comment_id IS NULL;

-- One reaction per user per emoji per comment
CREATE UNIQUE INDEX IF NOT EXISTS idx_issue_reactions_comment_unique
    ON issue_reactions(comment_id, user_id, content) WHERE comment_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_issue_reactions_issue_ts ON issue_reactions(issue_id, created_at);
CREATE INDEX IF NOT EXISTS idx_issue_reactions_comment_ts ON issue_reactions(comment_id, created_at);
