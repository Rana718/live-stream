-- identity.sql — users, auth_identities, otp_codes, refresh_tokens.
-- Pre-auth lookups (by email/phone/provider/token-hash) run under
-- database.WithSuperAdmin since the caller isn't authenticated yet.

-- name: GetUserByID :one
SELECT id, email, phone, full_name, avatar_url, password_hash,
       is_platform_super_admin, status, token_version, last_login_at,
       created_at, updated_at
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT id, email, phone, full_name, avatar_url, password_hash,
       is_platform_super_admin, status, token_version, last_login_at,
       created_at, updated_at
FROM users
WHERE lower(email) = lower(sqlc.arg(email)::text) AND deleted_at IS NULL;

-- name: GetUserByPhone :one
SELECT id, email, phone, full_name, avatar_url, password_hash,
       is_platform_super_admin, status, token_version, last_login_at,
       created_at, updated_at
FROM users
WHERE phone = sqlc.arg(phone)::text AND deleted_at IS NULL;

-- name: CreateUser :one
INSERT INTO users (email, phone, full_name, avatar_url, password_hash, status)
VALUES (
    sqlc.narg(email)::citext,
    sqlc.narg(phone)::citext,
    sqlc.narg(full_name)::text,
    sqlc.narg(avatar_url)::text,
    sqlc.narg(password_hash)::text,
    COALESCE(sqlc.narg(status)::text, 'active')
)
RETURNING id, email, phone, full_name, avatar_url, password_hash,
          is_platform_super_admin, status, token_version, last_login_at,
          created_at, updated_at;

-- name: UpdateUserProfileFields :one
UPDATE users
SET full_name  = COALESCE(sqlc.narg(full_name)::text, full_name),
    avatar_url = COALESCE(sqlc.narg(avatar_url)::text, avatar_url),
    email      = COALESCE(sqlc.narg(email)::citext, email),
    phone      = COALESCE(sqlc.narg(phone)::citext, phone)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, email, phone, full_name, avatar_url, status;

-- name: SetUserPassword :exec
UPDATE users SET password_hash = $2, token_version = token_version + 1
WHERE id = $1 AND deleted_at IS NULL;

-- name: TouchUserLastLogin :exec
UPDATE users SET last_login_at = now() WHERE id = $1;

-- name: BumpUserTokenVersion :one
UPDATE users SET token_version = token_version + 1
WHERE id = $1 AND deleted_at IS NULL
RETURNING token_version;

-- name: SetUserStatus :exec
UPDATE users SET status = $2, token_version = token_version + 1
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteUser :exec
UPDATE users
SET deleted_at = now(),
    status = 'disabled',
    email = NULL,
    phone = NULL,
    password_hash = NULL,
    full_name = 'Deleted user',
    token_version = token_version + 1
WHERE id = $1;

-- ─────────────────────────────────────────────────────── auth_identities

-- name: GetAuthIdentity :one
SELECT id, user_id, provider, provider_uid, verified_at, created_at
FROM auth_identities
WHERE provider = $1 AND provider_uid = $2;

-- name: ListAuthIdentitiesForUser :many
SELECT id, user_id, provider, provider_uid, verified_at, created_at
FROM auth_identities
WHERE user_id = $1
ORDER BY created_at;

-- name: CreateAuthIdentity :one
INSERT INTO auth_identities (user_id, provider, provider_uid, verified_at)
VALUES ($1, $2, $3, sqlc.narg(verified_at)::timestamptz)
RETURNING id, user_id, provider, provider_uid, verified_at, created_at;

-- name: DeleteAuthIdentity :exec
DELETE FROM auth_identities WHERE user_id = $1 AND provider = $2;

-- ─────────────────────────────────────────────────────────── otp_codes

-- name: CreateOtpCode :one
INSERT INTO otp_codes (channel, purpose, destination, code_hash, expires_at, ip)
VALUES ($1, $2, $3, $4, $5, sqlc.narg(ip)::inet)
RETURNING id, channel, purpose, destination, code_hash, attempts,
          max_attempts, consumed_at, expires_at, created_at;

-- name: GetLatestOtpCode :one
SELECT id, channel, purpose, destination, code_hash, attempts,
       max_attempts, consumed_at, expires_at, created_at
FROM otp_codes
WHERE destination = $1 AND purpose = $2 AND consumed_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: IncrementOtpAttempts :exec
UPDATE otp_codes SET attempts = attempts + 1 WHERE id = $1;

-- name: ConsumeOtpCode :exec
UPDATE otp_codes SET consumed_at = now() WHERE id = $1;

-- name: InvalidatePendingOtpCodes :exec
UPDATE otp_codes SET consumed_at = now()
WHERE destination = $1 AND purpose = $2 AND consumed_at IS NULL;

-- name: CountRecentOtpSends :one
SELECT count(*)
FROM otp_codes
WHERE destination = $1 AND created_at > now() - (sqlc.arg(window_seconds)::int || ' seconds')::interval;

-- ────────────────────────────────────────────────────────── refresh_tokens

-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (
    user_id, tenant_id, family_id, parent_id, token_hash, user_agent, ip, expires_at
)
VALUES (
    $1, $2, $3,
    sqlc.narg(parent_id)::uuid,
    $4,
    sqlc.narg(user_agent)::text,
    sqlc.narg(ip)::inet,
    $5
)
RETURNING id, user_id, tenant_id, family_id, parent_id, replaced_by,
          token_hash, issued_at, expires_at, used_at, revoked_at;

-- name: GetRefreshTokenByHash :one
SELECT id, user_id, tenant_id, family_id, parent_id, replaced_by,
       token_hash, issued_at, expires_at, used_at, revoked_at
FROM refresh_tokens
WHERE token_hash = $1;

-- name: MarkRefreshTokenUsed :exec
UPDATE refresh_tokens SET used_at = now(), replaced_by = sqlc.narg(replaced_by)::uuid
WHERE id = $1;

-- name: RevokeRefreshTokenFamily :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE family_id = $1 AND revoked_at IS NULL;

-- name: RevokeUserRefreshTokens :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: ListActiveSessionsForUser :many
SELECT id, tenant_id, family_id, user_agent, ip, issued_at, expires_at
FROM refresh_tokens
WHERE user_id = $1 AND revoked_at IS NULL AND used_at IS NULL AND expires_at > now()
ORDER BY issued_at DESC;
