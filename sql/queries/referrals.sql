-- name: GetOrCreateReferralCode :one
-- Idempotent: every user has exactly one code per tenant. Caller passes
-- a freshly-generated code; on conflict we just return the existing row.
INSERT INTO user_referral_codes (tenant_id, user_id, code)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, user_id) DO UPDATE
    SET created_at = user_referral_codes.created_at
RETURNING *;

-- name: GetReferralCodeByCode :one
SELECT * FROM user_referral_codes WHERE code = $1 LIMIT 1;

-- name: GetReferralCodeForUser :one
SELECT * FROM user_referral_codes
WHERE tenant_id = $1 AND user_id = $2 LIMIT 1;

-- name: IncrementReferralCodeUses :exec
UPDATE user_referral_codes SET uses = uses + 1 WHERE id = $1;

-- name: RecordReferralEvent :one
INSERT INTO referral_events (tenant_id, code, referrer_id, referred_user, status)
VALUES ($1, $2, $3, $4, 'signed_up')
RETURNING *;

-- name: MarkReferralRewarded :one
UPDATE referral_events
SET status = 'rewarded',
    reward_paise = $2,
    rewarded_at = now()
WHERE id = $1
RETURNING *;

-- name: ListReferralEventsForReferrer :many
SELECT * FROM referral_events
WHERE referrer_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ReferralStatsForUser :one
SELECT
    count(*) FILTER (WHERE status = 'signed_up') AS pending,
    count(*) FILTER (WHERE status = 'rewarded')  AS rewarded,
    COALESCE(sum(reward_paise) FILTER (WHERE status = 'rewarded'), 0)::bigint AS total_rewarded_paise
FROM referral_events
WHERE referrer_id = $1;
