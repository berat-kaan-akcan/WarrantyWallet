-- 0008_families: family accounts (one family per user) and single-use invite codes
CREATE TABLE families (
    id         uuid PRIMARY KEY,
    name       text NOT NULL,
    owner_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE family_members (
    user_id   uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    family_id uuid NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    role      text NOT NULL CHECK (role IN ('owner', 'member')),
    joined_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX family_members_family_idx ON family_members (family_id);
ALTER TABLE auth_tokens DROP CONSTRAINT IF EXISTS auth_tokens_kind_check;
ALTER TABLE auth_tokens ADD CONSTRAINT auth_tokens_kind_check
    CHECK (kind IN ('refresh', 'password_reset', 'email_verify', 'family_invite'));
