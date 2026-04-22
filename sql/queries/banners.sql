-- name: CreateBanner :one
INSERT INTO banners (title, subtitle, image_url, background_color,
                     link_type, link_id, link_url, display_order,
                     starts_at, ends_at, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetBannerByID :one
SELECT * FROM banners WHERE id = $1 LIMIT 1;

-- name: ListActiveBanners :many
SELECT * FROM banners
WHERE is_active = TRUE
  AND (starts_at IS NULL OR starts_at <= CURRENT_TIMESTAMP)
  AND (ends_at   IS NULL OR ends_at   >= CURRENT_TIMESTAMP)
ORDER BY display_order ASC, created_at DESC
LIMIT $1;

-- name: ListAllBanners :many
SELECT * FROM banners
ORDER BY display_order ASC, created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateBanner :one
UPDATE banners
SET title = $2,
    subtitle = $3,
    image_url = $4,
    background_color = $5,
    link_type = $6,
    link_id = $7,
    link_url = $8,
    display_order = $9,
    is_active = $10,
    starts_at = $11,
    ends_at = $12,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: SetBannerActive :one
UPDATE banners
SET is_active = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteBanner :exec
DELETE FROM banners WHERE id = $1;
