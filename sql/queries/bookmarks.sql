-- name: CreateBookmark :one
INSERT INTO bookmarks (user_id, lecture_id, material_id, timestamp_seconds, note)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListMyBookmarks :many
SELECT b.*, l.title AS lecture_title, m.title AS material_title
FROM bookmarks b
LEFT JOIN lectures l ON l.id = b.lecture_id
LEFT JOIN study_materials m ON m.id = b.material_id
WHERE b.user_id = $1
ORDER BY b.created_at DESC
LIMIT $2 OFFSET $3;

-- name: DeleteBookmark :exec
DELETE FROM bookmarks WHERE id = $1 AND user_id = $2;

-- name: ListMyBookmarksForLecture :many
SELECT * FROM bookmarks
WHERE user_id = $1 AND lecture_id = $2
ORDER BY timestamp_seconds ASC;
