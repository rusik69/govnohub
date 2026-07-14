CREATE TABLE user_gpg_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    public_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX user_gpg_keys_fingerprint_idx ON user_gpg_keys (fingerprint);
CREATE INDEX user_gpg_keys_user_id_idx ON user_gpg_keys (user_id);
