-- name: CreateUser :one
-- Phone-primary user creation. Email + password are optional now: a Google
-- signup will pass google_sub instead, an OTP-only signup leaves both null.
INSERT INTO users (tenant_id, phone_number, email, password_hash, full_name, role, auth_method)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 LIMIT 1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE tenant_id = $1 AND lower(email) = lower($2) LIMIT 1;

-- name: GetUserByPhone :one
SELECT * FROM users WHERE tenant_id = $1 AND phone_number = $2 LIMIT 1;

-- name: GetUserByGoogleSub :one
SELECT * FROM users WHERE tenant_id = $1 AND google_sub = $2 LIMIT 1;

-- name: UpdateUser :one
UPDATE users
SET full_name = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: UpdateOnboardingProfile :one
UPDATE users
SET full_name = COALESCE(NULLIF($2::text, ''), full_name),
    class_level = $3,
    board = $4,
    exam_goal = $5,
    onboarding_completed = TRUE,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: ListUsers :many
SELECT * FROM users
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: AdminSetUserRole :one
UPDATE users
SET role = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: AdminSetUserActive :one
UPDATE users
SET is_active = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: AdminUpdateUser :one
UPDATE users
SET full_name = $2,
    email = $3,
    phone_number = $4,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: AdminResetUserPassword :one
UPDATE users
SET password_hash = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: CreateUserWithPhone :one
-- Used by the OTP login path for first-time sign-ins. No username, no email,
-- no password — phone is the only identity. The user can attach an email or
-- a Google account later via the linking endpoints.
INSERT INTO users (tenant_id, phone_number, phone_verified, auth_method, role)
VALUES ($1, $2, TRUE, 'phone', 'student')
RETURNING *;

-- name: CreateUserWithGoogle :one
-- Used by the Google sign-in path for first-time sign-ins.
INSERT INTO users (tenant_id, email, full_name, google_sub, auth_method, role, email_verified)
VALUES ($1, $2, $3, $4, 'google', 'student', TRUE)
RETURNING *;

-- name: LinkPhoneToUser :one
UPDATE users
SET phone_number = $2, phone_verified = TRUE, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: LinkGoogleToUser :one
UPDATE users
SET google_sub = $2, email_verified = TRUE, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: CreateSmsCode :one
INSERT INTO sms_codes (phone_number, code_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetLatestSmsCode :one
-- Only the newest unconsumed, unexpired code is valid. Prior codes become
-- dead letters on send — a fresh send invalidates them implicitly.
SELECT * FROM sms_codes
WHERE phone_number = $1 AND consumed = FALSE AND expires_at > CURRENT_TIMESTAMP
ORDER BY created_at DESC
LIMIT 1;

-- name: IncrementSmsCodeAttempts :exec
UPDATE sms_codes SET attempts = attempts + 1 WHERE id = $1;

-- name: ConsumeSmsCode :exec
UPDATE sms_codes SET consumed = TRUE WHERE id = $1;

-- name: InvalidateOlderSmsCodes :exec
-- Called before issuing a new code so previous pending codes for the same
-- number become invalid — prevents an attacker from racing two codes.
UPDATE sms_codes SET consumed = TRUE WHERE phone_number = $1 AND consumed = FALSE;
