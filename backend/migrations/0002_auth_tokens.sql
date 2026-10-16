-- 0002_auth_tokens: refresh tokens and password reset tokens (hashed, single use)
CREATE TABLE auth_tokens (
    id          uuid PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind IN ('refresh', 'password_reset')),
    token_hash  text NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_tokens_user_kind_idx ON auth_tokens (user_id, kind) WHERE used_at IS NULL;
