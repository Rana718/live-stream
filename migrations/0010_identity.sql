-- 0010_identity.sql
-- Global identity. A `users` row is ONE human across the whole platform;
-- their role and per-tenant profile live in tenant_users / user_profiles
-- (0020). These tables are not tenant-scoped — cross-table "same tenant"
-- visibility for users is added in 0110 once tenant_users exists.

-- ─────────────────────────────────────────────────────────────── users
CREATE TABLE users (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email                  citext,
    phone                  citext,
    full_name              text,
    avatar_url             text,
    password_hash          text,
    is_platform_super_admin boolean NOT NULL DEFAULT false,
    status                 text NOT NULL DEFAULT 'active'
                               CHECK (status IN ('active','disabled')),
    token_version          integer NOT NULL DEFAULT 0,
    last_login_at          timestamptz,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    deleted_at             timestamptz
);

CREATE UNIQUE INDEX uq_users_email ON users (lower(email))
    WHERE email IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX uq_users_phone ON users (phone)
    WHERE phone IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_users_active ON users (id) WHERE deleted_at IS NULL;

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
-- Enhanced in 0110 with "shares my current tenant".
CREATE POLICY users_self ON users
    USING (id = current_app_user() OR is_super_admin())
    WITH CHECK (id = current_app_user() OR is_super_admin());

SELECT apply_updated_at('users');
SELECT apply_app_grants('users');

-- ────────────────────────────────────────────────────── auth_identities
-- One row per external credential. Pre-auth lookup by (provider,
-- provider_uid) is done server-side under WithSuperAdmin.
CREATE TABLE auth_identities (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider     auth_provider NOT NULL,
    provider_uid text NOT NULL,
    verified_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_uid)
);
CREATE INDEX idx_auth_identities_user ON auth_identities (user_id);

ALTER TABLE auth_identities ENABLE ROW LEVEL SECURITY;
ALTER TABLE auth_identities FORCE ROW LEVEL SECURITY;
CREATE POLICY auth_identities_owner ON auth_identities
    USING (user_id = current_app_user() OR is_super_admin())
    WITH CHECK (user_id = current_app_user() OR is_super_admin());
SELECT apply_app_grants('auth_identities');

-- ─────────────────────────────────────────────────────────── otp_codes
CREATE TABLE otp_codes (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    channel      otp_channel NOT NULL,
    purpose      otp_purpose NOT NULL DEFAULT 'login',
    destination  citext NOT NULL,
    code_hash    text NOT NULL,
    attempts     integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 5,
    consumed_at  timestamptz,
    expires_at   timestamptz NOT NULL,
    ip           inet,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_otp_codes_destination ON otp_codes (destination, created_at DESC);
CREATE INDEX idx_otp_codes_expires ON otp_codes (expires_at);

ALTER TABLE otp_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE otp_codes FORCE ROW LEVEL SECURITY;
-- Entirely server-side (auth service uses WithSuperAdmin).
CREATE POLICY otp_codes_super ON otp_codes
    USING (is_super_admin()) WITH CHECK (is_super_admin());
SELECT apply_app_grants('otp_codes');

-- ─────────────────────────────────────────────────────── refresh_tokens
-- DB-backed, family-based rotation. Presenting a token whose used_at is set
-- (reuse) means the family is compromised — the auth service revokes the
-- whole family_id. tenant_id FK is added in 0020 (tenants doesn't exist yet).
CREATE TABLE refresh_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   uuid NOT NULL,
    family_id   uuid NOT NULL,
    parent_id   uuid REFERENCES refresh_tokens(id) ON DELETE SET NULL,
    replaced_by uuid REFERENCES refresh_tokens(id) ON DELETE SET NULL,
    token_hash  text NOT NULL UNIQUE,
    user_agent  text,
    ip          inet,
    issued_at   timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    revoked_at  timestamptz
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens (family_id);

ALTER TABLE refresh_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE refresh_tokens FORCE ROW LEVEL SECURITY;
CREATE POLICY refresh_tokens_owner ON refresh_tokens
    USING (user_id = current_app_user() OR is_super_admin())
    WITH CHECK (user_id = current_app_user() OR is_super_admin());
SELECT apply_app_grants('refresh_tokens');
