ALTER TABLE notifications ADD COLUMN IF NOT EXISTS link TEXT;

CREATE INDEX IF NOT EXISTS idx_notifications_user_unread ON notifications (user_id, read) WHERE read = false;
