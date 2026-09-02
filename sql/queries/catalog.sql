-- catalog.sql — courses, instructors, sections, lessons, content bodies,
-- batches, live_sessions, recordings, video assets, class_schedules,
-- content_progress, lesson_bookmarks, certificates.

-- ─────────────────────────────────────────────────────────────── courses

-- name: CreateCourse :one
INSERT INTO courses (
    tenant_id, exam_category_id, title, slug, summary, description_rich,
    thumbnail_url, promo_video_url, language, level, class_level, exam_goal,
    hsn_sac, tax_rate_bps, refund_window_days, starts_on, ends_on, seats, created_by
)
VALUES ($1, sqlc.narg(exam_category_id)::uuid, $2, sqlc.arg(slug)::citext,
        sqlc.narg(summary)::text, COALESCE(sqlc.narg(description_rich)::jsonb, '{}'::jsonb),
        sqlc.narg(thumbnail_url)::text, sqlc.narg(promo_video_url)::text,
        COALESCE(sqlc.narg(language)::text, 'en'), COALESCE(sqlc.narg(level)::text, 'foundation'),
        sqlc.narg(class_level)::text, sqlc.narg(exam_goal)::text,
        sqlc.narg(hsn_sac)::text, COALESCE(sqlc.narg(tax_rate_bps)::int, 0),
        COALESCE(sqlc.narg(refund_window_days)::int, 0),
        sqlc.narg(starts_on)::date, sqlc.narg(ends_on)::date, sqlc.narg(seats)::int,
        sqlc.narg(created_by)::uuid)
RETURNING id, tenant_id, title, slug, status, approval_status, created_at;

-- name: GetCourse :one
SELECT id, tenant_id, exam_category_id, title, slug, summary, description_rich,
       thumbnail_url, promo_video_url, language, level, class_level, exam_goal,
       status, approval_status, hsn_sac, tax_rate_bps, refund_window_days,
       starts_on, ends_on, seats, created_by, created_at, updated_at
FROM courses WHERE id = $1 AND deleted_at IS NULL;

-- name: GetCourseBySlug :one
SELECT id, tenant_id, title, slug, summary, thumbnail_url, status, tax_rate_bps, hsn_sac
FROM courses WHERE tenant_id = $1 AND slug = sqlc.arg(slug)::citext AND deleted_at IS NULL;

-- name: ListPublishedCourses :many
SELECT id, title, slug, summary, thumbnail_url, language, level, class_level, exam_goal
FROM courses
WHERE tenant_id = $1 AND status = 'published' AND deleted_at IS NULL
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: ListCoursesForAdmin :many
SELECT id, title, slug, status, approval_status, created_at
FROM courses WHERE tenant_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: SearchCourses :many
SELECT id, title, slug, summary, thumbnail_url
FROM courses
WHERE tenant_id = $1 AND status = 'published' AND deleted_at IS NULL
  AND search_vector @@ websearch_to_tsquery('english', sqlc.arg(q)::text)
ORDER BY ts_rank(search_vector, websearch_to_tsquery('english', sqlc.arg(q)::text)) DESC
LIMIT $2;

-- name: UpdateCourse :one
UPDATE courses SET
    title = COALESCE(sqlc.narg(title)::text, title),
    summary = COALESCE(sqlc.narg(summary)::text, summary),
    description_rich = COALESCE(sqlc.narg(description_rich)::jsonb, description_rich),
    thumbnail_url = COALESCE(sqlc.narg(thumbnail_url)::text, thumbnail_url),
    promo_video_url = COALESCE(sqlc.narg(promo_video_url)::text, promo_video_url),
    language = COALESCE(sqlc.narg(language)::text, language),
    level = COALESCE(sqlc.narg(level)::text, level),
    class_level = COALESCE(sqlc.narg(class_level)::text, class_level),
    exam_goal = COALESCE(sqlc.narg(exam_goal)::text, exam_goal),
    hsn_sac = COALESCE(sqlc.narg(hsn_sac)::text, hsn_sac),
    tax_rate_bps = COALESCE(sqlc.narg(tax_rate_bps)::int, tax_rate_bps),
    refund_window_days = COALESCE(sqlc.narg(refund_window_days)::int, refund_window_days)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, title, slug, status, tax_rate_bps;

-- name: SetCourseStatus :exec
UPDATE courses SET status = $2 WHERE id = $1 AND deleted_at IS NULL;

-- name: ApproveCourse :one
UPDATE courses SET approval_status = 'approved', approved_by = $2, approved_at = now(), status = 'published'
WHERE id = $1
RETURNING id, approval_status, status, approved_at;

-- name: RejectCourse :one
UPDATE courses SET approval_status = 'rejected', rejection_reason = $2
WHERE id = $1
RETURNING id, approval_status, rejection_reason;

-- name: ListPendingCourses :many
SELECT id, title, slug, created_by, created_at
FROM courses WHERE tenant_id = $1 AND approval_status = 'pending' AND deleted_at IS NULL
ORDER BY created_at LIMIT $2 OFFSET $3;

-- name: DeleteCourse :exec
UPDATE courses SET deleted_at = now() WHERE id = $1;

-- ─────────────────────────────────────────────────────── course_instructors

-- name: AddCourseInstructor :one
INSERT INTO course_instructors (tenant_id, course_id, user_id, role, revenue_share_bps)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(role)::text, 'instructor'),
        COALESCE(sqlc.narg(revenue_share_bps)::int, 0))
ON CONFLICT (course_id, user_id) DO UPDATE SET
    role = EXCLUDED.role, revenue_share_bps = EXCLUDED.revenue_share_bps
RETURNING id, course_id, user_id, role, revenue_share_bps;

-- name: ListCourseInstructors :many
SELECT ci.user_id, ci.role, ci.revenue_share_bps, u.full_name, u.avatar_url
FROM course_instructors ci JOIN users u ON u.id = ci.user_id
WHERE ci.course_id = $1 ORDER BY ci.role;

-- name: RemoveCourseInstructor :exec
DELETE FROM course_instructors WHERE course_id = $1 AND user_id = $2;

-- ────────────────────────────────────────────────────────────── batches

-- name: CreateBatch :one
INSERT INTO batches (tenant_id, course_id, name, description, instructor_id, starts_on, ends_on, max_students)
VALUES ($1, $2, $3, sqlc.narg(description)::text, sqlc.narg(instructor_id)::uuid,
        sqlc.narg(starts_on)::date, sqlc.narg(ends_on)::date, sqlc.narg(max_students)::int)
RETURNING id, tenant_id, course_id, name, instructor_id, starts_on, ends_on, max_students, is_active;

-- name: GetBatch :one
SELECT id, tenant_id, course_id, name, description, instructor_id, starts_on, ends_on, max_students, is_active
FROM batches WHERE id = $1 AND deleted_at IS NULL;

-- name: ListBatchesByCourse :many
SELECT id, name, description, instructor_id, starts_on, ends_on, max_students, is_active
FROM batches WHERE tenant_id = $1 AND course_id = $2 AND deleted_at IS NULL
ORDER BY starts_on DESC NULLS LAST;

-- name: UpdateBatch :one
UPDATE batches SET
    name = COALESCE(sqlc.narg(name)::text, name),
    description = COALESCE(sqlc.narg(description)::text, description),
    instructor_id = COALESCE(sqlc.narg(instructor_id)::uuid, instructor_id),
    starts_on = COALESCE(sqlc.narg(starts_on)::date, starts_on),
    ends_on = COALESCE(sqlc.narg(ends_on)::date, ends_on),
    max_students = COALESCE(sqlc.narg(max_students)::int, max_students),
    is_active = COALESCE(sqlc.narg(is_active)::boolean, is_active)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, name, instructor_id, starts_on, ends_on, max_students, is_active;

-- name: DeleteBatch :exec
UPDATE batches SET deleted_at = now() WHERE id = $1;

-- ─────────────────────────────────────────────────────── sections + lessons

-- name: CreateCourseSection :one
INSERT INTO course_sections (tenant_id, course_id, title, display_order, drip_after_days)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(display_order)::int, 0),
        COALESCE(sqlc.narg(drip_after_days)::int, 0))
RETURNING id, course_id, title, display_order, drip_after_days;

-- name: ListCourseSections :many
SELECT id, title, display_order, drip_after_days
FROM course_sections WHERE course_id = $1 AND deleted_at IS NULL
ORDER BY display_order;

-- name: UpdateCourseSection :exec
UPDATE course_sections SET
    title = COALESCE(sqlc.narg(title)::text, title),
    display_order = COALESCE(sqlc.narg(display_order)::int, display_order),
    drip_after_days = COALESCE(sqlc.narg(drip_after_days)::int, drip_after_days)
WHERE id = $1;

-- name: DeleteCourseSection :exec
UPDATE course_sections SET deleted_at = now() WHERE id = $1;

-- name: CreateContentVideo :one
INSERT INTO content_videos (tenant_id, title, provider, playback_id, duration_sec, drm)
VALUES ($1, $2, COALESCE(sqlc.narg(provider)::text, 'self'), sqlc.narg(playback_id)::text,
        COALESCE(sqlc.narg(duration_sec)::int, 0), COALESCE(sqlc.narg(drm)::boolean, false))
RETURNING id, title, provider, playback_id, duration_sec, drm;

-- name: CreateContentDocument :one
INSERT INTO content_documents (tenant_id, title, file_key, file_size, mime, page_count)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(file_size)::bigint, 0), sqlc.narg(mime)::text, sqlc.narg(page_count)::int)
RETURNING id, title, file_key, file_size, mime, page_count;

-- name: CreateContentLink :one
INSERT INTO content_links (tenant_id, title, url) VALUES ($1, $2, $3)
RETURNING id, title, url;

-- name: GetContentVideo :one
SELECT id, title, provider, playback_id, duration_sec, drm
FROM content_videos WHERE id = $1;

-- name: GetContentDocument :one
SELECT id, title, file_key, file_size, mime, page_count
FROM content_documents WHERE id = $1;

-- name: GetContentLink :one
SELECT id, title, url FROM content_links WHERE id = $1;

-- name: CreateCourseLesson :one
INSERT INTO course_lessons (
    tenant_id, course_id, section_id, title, content_kind, video_id, document_id,
    link_id, live_session_id, is_preview, display_order, available_after_days,
    available_at, status
)
VALUES ($1, $2, sqlc.narg(section_id)::uuid, $3, $4,
        sqlc.narg(video_id)::uuid, sqlc.narg(document_id)::uuid, sqlc.narg(link_id)::uuid,
        sqlc.narg(live_session_id)::uuid, COALESCE(sqlc.narg(is_preview)::boolean, false),
        COALESCE(sqlc.narg(display_order)::int, 0),
        COALESCE(sqlc.narg(available_after_days)::int, 0),
        sqlc.narg(available_at)::timestamptz,
        COALESCE(sqlc.narg(status)::publish_status, 'draft'))
RETURNING id, course_id, section_id, title, content_kind, is_preview, display_order, status;

-- name: GetCourseLesson :one
SELECT id, tenant_id, course_id, section_id, title, content_kind, video_id,
       document_id, link_id, live_session_id, is_preview, display_order,
       available_after_days, available_at, status
FROM course_lessons WHERE id = $1 AND deleted_at IS NULL;

-- name: ListCourseLessons :many
SELECT id, section_id, title, content_kind, video_id, document_id, link_id,
       live_session_id, is_preview, display_order, available_after_days, available_at, status
FROM course_lessons
WHERE course_id = $1 AND deleted_at IS NULL
  AND (sqlc.narg(published_only)::boolean IS NOT TRUE OR status = 'published')
ORDER BY display_order;

-- name: UpdateCourseLesson :exec
UPDATE course_lessons SET
    title = COALESCE(sqlc.narg(title)::text, title),
    section_id = COALESCE(sqlc.narg(section_id)::uuid, section_id),
    is_preview = COALESCE(sqlc.narg(is_preview)::boolean, is_preview),
    display_order = COALESCE(sqlc.narg(display_order)::int, display_order),
    available_after_days = COALESCE(sqlc.narg(available_after_days)::int, available_after_days),
    available_at = COALESCE(sqlc.narg(available_at)::timestamptz, available_at),
    status = COALESCE(sqlc.narg(status)::publish_status, status)
WHERE id = $1;

-- name: DeleteCourseLesson :exec
UPDATE course_lessons SET deleted_at = now() WHERE id = $1;

-- ─────────────────────────────────────────────────── live_sessions + recordings

-- name: CreateLiveSession :one
INSERT INTO live_sessions (tenant_id, course_id, batch_id, instructor_id, schedule_id, title, description, ingest_key, scheduled_start)
VALUES ($1, sqlc.narg(course_id)::uuid, sqlc.narg(batch_id)::uuid, sqlc.narg(instructor_id)::uuid,
        sqlc.narg(schedule_id)::uuid, $2, sqlc.narg(description)::text, $3, sqlc.narg(scheduled_start)::timestamptz)
RETURNING id, tenant_id, course_id, title, status, ingest_key, scheduled_start, created_at;

-- name: GetLiveSession :one
SELECT id, tenant_id, course_id, batch_id, instructor_id, title, description,
       status, ingest_key, scheduled_start, actual_start, actual_end, peak_viewers, thumbnail_url
FROM live_sessions WHERE id = $1 AND deleted_at IS NULL;

-- name: GetLiveSessionByIngestKey :one
SELECT id, tenant_id, course_id, instructor_id, title, status, ingest_key
FROM live_sessions WHERE ingest_key = $1 AND deleted_at IS NULL;

-- name: ListLiveSessions :many
SELECT id, course_id, title, status, scheduled_start, actual_start, peak_viewers
FROM live_sessions
WHERE tenant_id = $1 AND status = ANY(sqlc.arg(statuses)::session_status[]) AND deleted_at IS NULL
ORDER BY scheduled_start DESC NULLS LAST LIMIT $2 OFFSET $3;

-- name: StartLiveSession :one
UPDATE live_sessions SET status = 'live', actual_start = now()
WHERE id = $1 AND status = 'scheduled'
RETURNING id, tenant_id, status, actual_start;

-- name: EndLiveSession :one
UPDATE live_sessions SET status = 'ended', actual_end = now()
WHERE id = $1 AND status = 'live'
RETURNING id, tenant_id, status, actual_end;

-- name: SetLiveSessionPeakViewers :exec
UPDATE live_sessions SET peak_viewers = GREATEST(peak_viewers, $2) WHERE id = $1;

-- name: CreateRecording :one
INSERT INTO recordings (tenant_id, session_id, file_key, status)
VALUES ($1, $2, sqlc.narg(file_key)::text, COALESCE(sqlc.narg(status)::recording_status, 'queued'))
RETURNING id, tenant_id, session_id, file_key, status, created_at;

-- name: GetRecording :one
SELECT id, tenant_id, session_id, video_asset_id, file_key, file_size, duration_sec, status, thumbnail_url
FROM recordings WHERE id = $1;

-- name: UpdateRecording :one
UPDATE recordings SET
    status = COALESCE(sqlc.narg(status)::recording_status, status),
    video_asset_id = COALESCE(sqlc.narg(video_asset_id)::uuid, video_asset_id),
    file_key = COALESCE(sqlc.narg(file_key)::text, file_key),
    file_size = COALESCE(sqlc.narg(file_size)::bigint, file_size),
    duration_sec = COALESCE(sqlc.narg(duration_sec)::int, duration_sec),
    thumbnail_url = COALESCE(sqlc.narg(thumbnail_url)::text, thumbnail_url)
WHERE id = $1
RETURNING id, status, video_asset_id;

-- name: ListRecordingsBySession :many
SELECT id, video_asset_id, file_key, duration_sec, status, thumbnail_url, created_at
FROM recordings WHERE session_id = $1 ORDER BY created_at DESC;

-- ─────────────────────────────────────────────────── video assets / renditions

-- name: CreateVideoAsset :one
INSERT INTO video_assets (tenant_id, source_key, status) VALUES ($1, sqlc.narg(source_key)::text, 'queued')
RETURNING id, tenant_id, source_key, status;

-- name: SetVideoAssetStatus :exec
UPDATE video_assets SET status = $2, duration_sec = COALESCE(sqlc.narg(duration_sec)::int, duration_sec) WHERE id = $1;

-- name: AddVideoRendition :exec
INSERT INTO video_renditions (tenant_id, video_asset_id, height, bitrate_kbps, codec, file_key, file_size)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(bitrate_kbps)::int, 0), COALESCE(sqlc.narg(codec)::text, 'h264'), $4, COALESCE(sqlc.narg(file_size)::bigint, 0))
ON CONFLICT (video_asset_id, height) DO UPDATE SET file_key = EXCLUDED.file_key, file_size = EXCLUDED.file_size;

-- name: ListVideoRenditions :many
SELECT height, bitrate_kbps, codec, file_key, file_size
FROM video_renditions WHERE video_asset_id = $1 ORDER BY height;

-- ────────────────────────────────────────────────────────── class_schedules

-- name: CreateClassSchedule :one
INSERT INTO class_schedules (tenant_id, course_id, batch_id, instructor_id, title, description, by_weekday, start_local, duration_min, timezone, starts_on, ends_on)
VALUES ($1, sqlc.narg(course_id)::uuid, sqlc.narg(batch_id)::uuid, $2, $3, sqlc.narg(description)::text,
        $4, $5, COALESCE(sqlc.narg(duration_min)::int, 60),
        COALESCE(sqlc.narg(timezone)::text, 'Asia/Kolkata'),
        COALESCE(sqlc.narg(starts_on)::date, current_date), sqlc.narg(ends_on)::date)
RETURNING id, tenant_id, title, by_weekday, start_local, duration_min, timezone, starts_on, ends_on, is_active;

-- name: ListClassSchedules :many
SELECT id, course_id, batch_id, instructor_id, title, by_weekday, start_local, duration_min, timezone, starts_on, ends_on, is_active
FROM class_schedules WHERE tenant_id = $1 AND is_active
ORDER BY created_at DESC;

-- name: ListActiveSchedulesForMaterialisation :many
SELECT id, tenant_id, course_id, batch_id, instructor_id, title, by_weekday, start_local, duration_min, timezone, starts_on, ends_on, last_materialised_at
FROM class_schedules WHERE is_active AND (ends_on IS NULL OR ends_on >= current_date);

-- name: SetScheduleActive :exec
UPDATE class_schedules SET is_active = $2 WHERE id = $1;

-- name: TouchScheduleMaterialised :exec
UPDATE class_schedules SET last_materialised_at = now() WHERE id = $1;

-- name: DeleteClassSchedule :exec
DELETE FROM class_schedules WHERE id = $1;

-- ──────────────────────────────────────────────── content_progress + bookmarks

-- name: UpsertContentProgress :one
INSERT INTO content_progress (tenant_id, user_id, lesson_id, watched_sec, position_sec, completed_at, last_at)
VALUES ($1, $2, $3, $4, $5, sqlc.narg(completed_at)::timestamptz, now())
ON CONFLICT (user_id, lesson_id) DO UPDATE SET
    watched_sec = GREATEST(content_progress.watched_sec, EXCLUDED.watched_sec),
    position_sec = EXCLUDED.position_sec,
    completed_at = COALESCE(content_progress.completed_at, EXCLUDED.completed_at),
    last_at = now()
RETURNING id, user_id, lesson_id, watched_sec, position_sec, completed_at;

-- name: ListContentProgressForUser :many
SELECT lesson_id, watched_sec, position_sec, completed_at, last_at
FROM content_progress WHERE tenant_id = $1 AND user_id = $2
ORDER BY last_at DESC LIMIT $3 OFFSET $4;

-- name: CountCompletedLessons :one
SELECT count(*) FROM content_progress cp
WHERE cp.tenant_id = $1 AND cp.user_id = $2 AND cp.completed_at IS NOT NULL
  AND cp.lesson_id IN (SELECT cl.id FROM course_lessons cl WHERE cl.course_id = $3);

-- name: CreateLessonBookmark :one
INSERT INTO lesson_bookmarks (tenant_id, user_id, lesson_id, position_sec, note)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(position_sec)::int, 0), sqlc.narg(note)::text)
RETURNING id, lesson_id, position_sec, note, created_at;

-- name: ListLessonBookmarks :many
SELECT id, lesson_id, position_sec, note, created_at
FROM lesson_bookmarks WHERE tenant_id = $1 AND user_id = $2
ORDER BY created_at DESC;

-- name: DeleteLessonBookmark :exec
DELETE FROM lesson_bookmarks WHERE id = $1 AND user_id = $2;

-- ─────────────────────────────────────────────────────────── certificates

-- name: IssueCertificate :one
INSERT INTO certificates (tenant_id, user_id, course_id, serial, pdf_key)
VALUES ($1, $2, $3, $4, sqlc.narg(pdf_key)::text)
ON CONFLICT (user_id, course_id) DO NOTHING
RETURNING id, serial, status, issued_at;

-- name: GetCertificate :one
SELECT id, user_id, course_id, serial, status, issued_at, revoked_at, pdf_key
FROM certificates WHERE id = $1;

-- name: ListCertificatesForUser :many
SELECT c.id, c.course_id, c.serial, c.status, c.issued_at, co.title
FROM certificates c JOIN courses co ON co.id = c.course_id
WHERE c.tenant_id = $1 AND c.user_id = $2 AND c.status = 'issued'
ORDER BY c.issued_at DESC;
