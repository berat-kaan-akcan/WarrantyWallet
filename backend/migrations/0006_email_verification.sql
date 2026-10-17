-- E-mail verification: verified timestamp + a new single-use token kind.
ALTER TABLE users ADD COLUMN email_verified_at timestamptz;
ALTER TABLE auth_tokens DROP CONSTRAINT IF EXISTS auth_tokens_kind_check;
ALTER TABLE auth_tokens ADD CONSTRAINT auth_tokens_kind_check CHECK (kind IN ('refresh', 'password_reset', 'email_verify'));
