-- name: CreateLecture :one
INSERT INTO lectures (topic_id, chapter_id, subject_id, title, description, lecture_type,
                     instructor_id, stream_id, recording_id, thumbnail_url, scheduled_at,
                     duration_seconds, language, is_free, is_published, display_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: GetLectureByID :one
SELECT * FROM lectures WHERE id = $1 LIMIT 1;

-- name: ListLecturesByTopic :many
SELECT * FROM lectures
WHERE topic_id = $1 AND is_published = TRUE
ORDER BY display_order ASC, scheduled_at ASC NULLS LAST;

-- name: ListLecturesByChapter :many
SELECT * FROM lectures
WHERE chapter_id = $1 AND is_published = TRUE
ORDER BY display_order ASC, scheduled_at ASC NULLS LAST;

-- name: ListLecturesBySubject :many
SELECT * FROM lectures
WHERE subject_id = $1 AND is_published = TRUE
ORDER BY display_order ASC, scheduled_at ASC NULLS LAST;

-- name: ListLiveLectures :many
SELECT * FROM lectures
WHERE lecture_type = 'live' AND scheduled_at >= CURRENT_TIMESTAMP - INTERVAL '1 day'
ORDER BY scheduled_at ASC
LIMIT $1 OFFSET $2;

-- name: ListLecturesByInstructor :many
SELECT * FROM lectures WHERE instructor_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: SearchLectures :many
SELECT * FROM lectures
WHERE is_published = TRUE
  AND (search_vector @@ plainto_tsquery('simple', $1) OR title ILIKE '%' || $1 || '%')
ORDER BY ts_rank(search_vector, plainto_tsquery('simple', $1)) DESC
LIMIT $2 OFFSET $3;

-- name: UpdateLecture :one
UPDATE lectures
SET title = $2, description = $3, lecture_type = $4, thumbnail_url = $5, scheduled_at = $6,
    duration_seconds = $7, language = $8, is_free = $9, is_published = $10, display_order = $11,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: IncrementLectureViewCount :exec
UPDATE lectures SET view_count = view_count + 1 WHERE id = $1;

-- name: DeleteLecture :exec
DELETE FROM lectures WHERE id = $1;

-- name: UpsertLectureView :one
INSERT INTO lecture_views (user_id, lecture_id, watched_seconds, completed, last_watched_at)
VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
ON CONFLICT (user_id, lecture_id) DO UPDATE
    SET watched_seconds = GREATEST(lecture_views.watched_seconds, EXCLUDED.watched_seconds),
        completed = lecture_views.completed OR EXCLUDED.completed,
        last_watched_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetLectureView :one
SELECT * FROM lecture_views WHERE user_id = $1 AND lecture_id = $2 LIMIT 1;

-- name: ListUserLectureHistory :many
SELECT lv.*, l.title, l.thumbnail_url, l.duration_seconds
FROM lecture_views lv
JOIN lectures l ON l.id = lv.lecture_id
WHERE lv.user_id = $1
ORDER BY lv.last_watched_at DESC
LIMIT $2 OFFSET $3;
