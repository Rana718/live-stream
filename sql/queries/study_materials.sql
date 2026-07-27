-- name: CreateStudyMaterial :one
-- topic_id/chapter_id/subject_id/uploaded_by are all nullable — no
-- guaranteed parent to derive from, passed explicitly.
INSERT INTO study_materials (topic_id, chapter_id, subject_id, title, description, material_type,
                             file_path, file_size, language, is_free, uploaded_by, tenant_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetStudyMaterialByID :one
SELECT * FROM study_materials WHERE id = $1 LIMIT 1;

-- name: ListMaterialsByChapter :many
SELECT * FROM study_materials WHERE chapter_id = $1 ORDER BY created_at DESC;

-- name: ListMaterialsByTopic :many
SELECT * FROM study_materials WHERE topic_id = $1 ORDER BY created_at DESC;

-- name: ListMaterialsBySubject :many
SELECT * FROM study_materials WHERE subject_id = $1 ORDER BY created_at DESC;

-- name: IncrementMaterialDownload :exec
UPDATE study_materials SET download_count = download_count + 1 WHERE id = $1;

-- name: DeleteStudyMaterial :exec
DELETE FROM study_materials WHERE id = $1;
