// Package lectures — schema-v2 adapter. The old standalone `lectures` table
// (topic/chapter/subject-scoped, with view counts + watch tracking) is gone.
// Content is now `course_lessons` (typed: video|document|link|live_session)
// hanging off courses/sections, and progress is `content_progress`.
//
// This adapter keeps the /lectures routes working for the course_id path and
// degrades (empty) for the removed topic/chapter/subject/search filters. The
// full course→section→lesson browsing UI is rebuilt in Phase J.
package lectures

import (
	"context"
	"strings"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type CreateLectureRequest struct {
	CourseID     *uuid.UUID `json:"course_id"`
	SectionID    *uuid.UUID `json:"section_id"`
	TopicID      *uuid.UUID `json:"topic_id"` // legacy; ignored in v2
	Title        string     `json:"title" validate:"required,min=3"`
	Description  string     `json:"description"`
	VideoURL     string     `json:"video_url"`
	DocumentURL  string     `json:"document_url"`
	LinkURL      string     `json:"link_url"`
	ThumbnailURL string     `json:"thumbnail_url"`
	DurationSec  int32      `json:"duration_seconds"`
	IsFree       bool       `json:"is_free"`
	IsPublished  bool       `json:"is_published"`
	DisplayOrder int32      `json:"display_order"`
}

// Lesson is the flattened view returned to the current frontend.
type Lesson struct {
	ID           string     `json:"id"`
	CourseID     string     `json:"course_id"`
	CourseTitle  string     `json:"course_title,omitempty"`
	SectionID    string     `json:"section_id"`
	Title        string     `json:"title"`
	ContentKind  string     `json:"content_kind"`
	VideoURL     string     `json:"video_url,omitempty"`
	HlsURL       string     `json:"hls_url,omitempty"`
	DocumentURL  string     `json:"document_url,omitempty"`
	LinkURL      string     `json:"link_url,omitempty"`
	DurationSec  int32      `json:"duration_seconds"`
	IsPreview    bool       `json:"is_preview"`
	IsFree       bool       `json:"is_free"`
	IsPublished  bool       `json:"is_published"`
	DisplayOrder int32      `json:"display_order"`
	AvailableAt  *time.Time `json:"available_at,omitempty"`
}

func statusFor(pub bool) db.PublishStatus {
	if pub {
		return db.PublishStatusPublished
	}
	return db.PublishStatusDraft
}

func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, req CreateLectureRequest) (Lesson, error) {
	if req.CourseID == nil {
		return Lesson{}, errNoCourse
	}
	kind := db.ContentKindLink
	var videoID, docID, linkID pgtype.UUID

	switch {
	case req.VideoURL != "":
		provider := "self"
		if strings.Contains(req.VideoURL, "youtu") {
			provider = "youtube"
		}
		v, err := s.q.CreateContentVideo(ctx, db.CreateContentVideoParams{
			TenantID:   utils.UUIDToPg(tenantID),
			Title:      req.Title,
			Provider:   pgtype.Text{String: provider, Valid: true},
			PlaybackID: pgtype.Text{String: req.VideoURL, Valid: true},
			DurationSec: pgtype.Int4{Int32: req.DurationSec, Valid: req.DurationSec > 0},
		})
		if err != nil {
			return Lesson{}, err
		}
		kind, videoID = db.ContentKindVideo, v.ID
	case req.DocumentURL != "":
		d, err := s.q.CreateContentDocument(ctx, db.CreateContentDocumentParams{
			TenantID: utils.UUIDToPg(tenantID),
			Title:    req.Title,
			FileKey:  req.DocumentURL,
		})
		if err != nil {
			return Lesson{}, err
		}
		kind, docID = db.ContentKindDocument, d.ID
	default:
		url := req.LinkURL
		if url == "" {
			url = req.VideoURL
		}
		l, err := s.q.CreateContentLink(ctx, db.CreateContentLinkParams{
			TenantID: utils.UUIDToPg(tenantID), Title: req.Title, Url: url,
		})
		if err != nil {
			return Lesson{}, err
		}
		kind, linkID = db.ContentKindLink, l.ID
	}

	row, err := s.q.CreateCourseLesson(ctx, db.CreateCourseLessonParams{
		TenantID:     utils.UUIDToPg(tenantID),
		CourseID:     utils.UUIDToPg(*req.CourseID),
		Title:        req.Title,
		ContentKind:  kind,
		SectionID:    utils.UUIDPtrToPg(req.SectionID),
		VideoID:      videoID,
		DocumentID:   docID,
		LinkID:       linkID,
		IsPreview:    utils.BoolToPg(req.IsFree),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
		Status:       db.NullPublishStatus{PublishStatus: statusFor(req.IsPublished), Valid: true},
	})
	if err != nil {
		return Lesson{}, err
	}
	return Lesson{
		ID: utils.UUIDFromPg(row.ID), CourseID: utils.UUIDFromPg(row.CourseID),
		SectionID: utils.UUIDFromPg(row.SectionID), Title: row.Title,
		ContentKind: string(row.ContentKind), IsPreview: row.IsPreview,
		IsFree: row.IsPreview, IsPublished: row.Status == db.PublishStatusPublished,
		DisplayOrder: row.DisplayOrder,
	}, nil
}

func (s *Service) resolveContent(ctx context.Context, l *Lesson, kind db.ContentKind, videoID, docID, linkID pgtype.UUID) {
	switch kind {
	case db.ContentKindVideo:
		if v, err := s.q.GetContentVideo(ctx, videoID); err == nil {
			l.VideoURL = utils.TextFromPg(v.PlaybackID)
			if v.Provider == "self" {
				l.HlsURL = utils.TextFromPg(v.PlaybackID)
			}
			l.DurationSec = v.DurationSec
		}
	case db.ContentKindDocument:
		if d, err := s.q.GetContentDocument(ctx, docID); err == nil {
			l.DocumentURL = d.FileKey
		}
	case db.ContentKindLink:
		if k, err := s.q.GetContentLink(ctx, linkID); err == nil {
			l.LinkURL = k.Url
			l.VideoURL = k.Url
		}
	}
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Lesson, error) {
	row, err := s.q.GetCourseLesson(ctx, utils.UUIDToPg(id))
	if err != nil {
		return Lesson{}, err
	}
	l := Lesson{
		ID: utils.UUIDFromPg(row.ID), CourseID: utils.UUIDFromPg(row.CourseID),
		SectionID: utils.UUIDFromPg(row.SectionID), Title: row.Title,
		ContentKind: string(row.ContentKind), IsPreview: row.IsPreview, IsFree: row.IsPreview,
		IsPublished: row.Status == db.PublishStatusPublished, DisplayOrder: row.DisplayOrder,
	}
	if row.AvailableAt.Valid {
		t := row.AvailableAt.Time
		l.AvailableAt = &t
	}
	s.resolveContent(ctx, &l, row.ContentKind, row.VideoID, row.DocumentID, row.LinkID)
	return l, nil
}

func (s *Service) ListByCourse(ctx context.Context, courseID uuid.UUID) ([]Lesson, error) {
	rows, err := s.q.ListCourseLessons(ctx, db.ListCourseLessonsParams{
		CourseID: utils.UUIDToPg(courseID),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Lesson, 0, len(rows))
	for _, r := range rows {
		l := Lesson{
			ID: utils.UUIDFromPg(r.ID), SectionID: utils.UUIDFromPg(r.SectionID),
			Title: r.Title, ContentKind: string(r.ContentKind), IsPreview: r.IsPreview,
			IsFree: r.IsPreview, IsPublished: r.Status == db.PublishStatusPublished,
			DisplayOrder: r.DisplayOrder,
		}
		out = append(out, l)
	}
	return out, nil
}

// ListForTenant is the flat "all lectures" index (optionally filtered by
// course). Fills CourseID/CourseTitle so the UI can group.
func (s *Service) ListForTenant(ctx context.Context, tenantID uuid.UUID, courseID *uuid.UUID, limit, offset int32) ([]Lesson, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.q.ListCourseLessonsForTenant(ctx, db.ListCourseLessonsForTenantParams{
		TenantID: utils.UUIDToPg(tenantID),
		CourseID: utils.UUIDPtrToPg(courseID),
		Limit:    limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Lesson, 0, len(rows))
	for _, r := range rows {
		l := Lesson{
			ID: utils.UUIDFromPg(r.ID), CourseID: utils.UUIDFromPg(r.CourseID),
			SectionID: utils.UUIDFromPg(r.SectionID), Title: r.Title,
			CourseTitle: r.CourseTitle, ContentKind: string(r.ContentKind),
			IsPreview: r.IsPreview, IsFree: r.IsPreview,
			IsPublished: r.Status == db.PublishStatusPublished, DisplayOrder: r.DisplayOrder,
		}
		s.resolveContent(ctx, &l, r.ContentKind, r.VideoID, r.DocumentID, r.LinkID)
		out = append(out, l)
	}
	return out, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req CreateLectureRequest) error {
	return s.q.UpdateCourseLesson(ctx, db.UpdateCourseLessonParams{
		ID:           utils.UUIDToPg(id),
		Title:        pgtype.Text{String: req.Title, Valid: req.Title != ""},
		IsPreview:    utils.BoolToPg(req.IsFree),
		DisplayOrder: utils.Int4ToPg(req.DisplayOrder),
		Status:       db.NullPublishStatus{PublishStatus: statusFor(req.IsPublished), Valid: true},
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteCourseLesson(ctx, utils.UUIDToPg(id))
}

// ── course sections ─────────────────────────────────────────────────

type Section struct {
	ID            string `json:"id"`
	CourseID      string `json:"course_id"`
	Title         string `json:"title"`
	DisplayOrder  int32  `json:"display_order"`
	DripAfterDays int32  `json:"drip_after_days"`
}

type SectionRequest struct {
	CourseID      *uuid.UUID `json:"course_id"`
	Title         string     `json:"title" validate:"required,min=1"`
	DisplayOrder  int32      `json:"display_order"`
	DripAfterDays int32      `json:"drip_after_days"`
}

func (s *Service) ListSections(ctx context.Context, courseID uuid.UUID) ([]Section, error) {
	rows, err := s.q.ListCourseSections(ctx, utils.UUIDToPg(courseID))
	if err != nil {
		return nil, err
	}
	out := make([]Section, len(rows))
	for i, r := range rows {
		out[i] = Section{
			ID: utils.UUIDFromPg(r.ID), CourseID: courseID.String(), Title: r.Title,
			DisplayOrder: r.DisplayOrder, DripAfterDays: r.DripAfterDays,
		}
	}
	return out, nil
}

func (s *Service) CreateSection(ctx context.Context, tenantID uuid.UUID, req SectionRequest) (Section, error) {
	if req.CourseID == nil {
		return Section{}, errNoCourse
	}
	row, err := s.q.CreateCourseSection(ctx, db.CreateCourseSectionParams{
		TenantID:      utils.UUIDToPg(tenantID),
		CourseID:      utils.UUIDToPg(*req.CourseID),
		Title:         req.Title,
		DisplayOrder:  utils.Int4ToPg(req.DisplayOrder),
		DripAfterDays: utils.Int4ToPg(req.DripAfterDays),
	})
	if err != nil {
		return Section{}, err
	}
	return Section{
		ID: utils.UUIDFromPg(row.ID), CourseID: utils.UUIDFromPg(row.CourseID),
		Title: row.Title, DisplayOrder: row.DisplayOrder, DripAfterDays: row.DripAfterDays,
	}, nil
}

func (s *Service) UpdateSection(ctx context.Context, id uuid.UUID, req SectionRequest) error {
	return s.q.UpdateCourseSection(ctx, db.UpdateCourseSectionParams{
		ID:            utils.UUIDToPg(id),
		Title:         pgtype.Text{String: req.Title, Valid: req.Title != ""},
		DisplayOrder:  utils.Int4ToPg(req.DisplayOrder),
		DripAfterDays: utils.Int4ToPg(req.DripAfterDays),
	})
}

func (s *Service) DeleteSection(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteCourseSection(ctx, utils.UUIDToPg(id))
}

type RecordWatchRequest struct {
	LessonID       *uuid.UUID `json:"lesson_id"`
	LectureID      *uuid.UUID `json:"lecture_id"` // legacy alias
	WatchedSeconds int32      `json:"watched_seconds"`
	PositionSec    int32      `json:"position_seconds"`
	Completed      bool       `json:"completed"`
}

func (r RecordWatchRequest) lesson() *uuid.UUID {
	if r.LessonID != nil {
		return r.LessonID
	}
	return r.LectureID
}

func (s *Service) RecordWatch(ctx context.Context, tenantID, userID uuid.UUID, req RecordWatchRequest) error {
	lid := req.lesson()
	if lid == nil {
		return errNoLesson
	}
	completed := pgtype.Timestamptz{}
	if req.Completed {
		completed = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	_, err := s.q.UpsertContentProgress(ctx, db.UpsertContentProgressParams{
		TenantID:    utils.UUIDToPg(tenantID),
		UserID:      utils.UUIDToPg(userID),
		LessonID:    utils.UUIDToPg(*lid),
		WatchedSec:  req.WatchedSeconds,
		PositionSec: req.PositionSec,
		CompletedAt: completed,
	})
	return err
}

func (s *Service) ListHistory(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]db.ListContentProgressForUserRow, error) {
	return s.q.ListContentProgressForUser(ctx, db.ListContentProgressForUserParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
		Limit: limit, Offset: offset,
	})
}

var (
	errNoCourse = &lecErr{"course_id is required"}
	errNoLesson = &lecErr{"lesson_id is required"}
)

type lecErr struct{ s string }

func (e *lecErr) Error() string { return e.s }
