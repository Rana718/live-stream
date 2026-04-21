package lectures

import (
	"context"
	"time"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type CreateLectureRequest struct {
	TopicID         *uuid.UUID `json:"topic_id"`
	ChapterID       *uuid.UUID `json:"chapter_id"`
	SubjectID       *uuid.UUID `json:"subject_id"`
	Title           string     `json:"title" validate:"required,min=3"`
	Description     string     `json:"description"`
	LectureType     string     `json:"lecture_type" validate:"required,oneof=live recorded"`
	InstructorID    *uuid.UUID `json:"instructor_id"`
	StreamID        *uuid.UUID `json:"stream_id"`
	RecordingID     *uuid.UUID `json:"recording_id"`
	ThumbnailURL    string     `json:"thumbnail_url"`
	ScheduledAt     *time.Time `json:"scheduled_at"`
	DurationSeconds int32      `json:"duration_seconds"`
	Language        string     `json:"language"`
	IsFree          bool       `json:"is_free"`
	IsPublished     bool       `json:"is_published"`
	DisplayOrder    int32      `json:"display_order"`
}

func (s *Service) Create(ctx context.Context, req CreateLectureRequest) (*db.Lecture, error) {
	if req.Language == "" {
		req.Language = "en"
	}
	l, err := s.q.CreateLecture(ctx, db.CreateLectureParams{
		TopicID:         utils.UUIDPtrToPg(req.TopicID),
		ChapterID:       utils.UUIDPtrToPg(req.ChapterID),
		SubjectID:       utils.UUIDPtrToPg(req.SubjectID),
		Title:           req.Title,
		Description:     utils.TextToPg(req.Description),
		LectureType:     utils.TextToPg(req.LectureType),
		InstructorID:    utils.UUIDPtrToPg(req.InstructorID),
		StreamID:        utils.UUIDPtrToPg(req.StreamID),
		RecordingID:     utils.UUIDPtrToPg(req.RecordingID),
		ThumbnailUrl:    utils.TextToPg(req.ThumbnailURL),
		ScheduledAt:     utils.TimestampPtrToPg(req.ScheduledAt),
		DurationSeconds: utils.Int4ToPg(req.DurationSeconds),
		Language:        utils.TextToPg(req.Language),
		IsFree:          utils.BoolToPg(req.IsFree),
		IsPublished:     utils.BoolToPg(req.IsPublished),
		DisplayOrder:    utils.Int4ToPg(req.DisplayOrder),
	})
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*db.Lecture, error) {
	l, err := s.q.GetLectureByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *Service) ListByTopic(ctx context.Context, topicID uuid.UUID) ([]db.Lecture, error) {
	return s.q.ListLecturesByTopic(ctx, utils.UUIDToPg(topicID))
}

func (s *Service) ListByChapter(ctx context.Context, chapterID uuid.UUID) ([]db.Lecture, error) {
	return s.q.ListLecturesByChapter(ctx, utils.UUIDToPg(chapterID))
}

func (s *Service) ListBySubject(ctx context.Context, subjectID uuid.UUID) ([]db.Lecture, error) {
	return s.q.ListLecturesBySubject(ctx, utils.UUIDToPg(subjectID))
}

func (s *Service) ListLive(ctx context.Context, limit, offset int32) ([]db.Lecture, error) {
	return s.q.ListLiveLectures(ctx, db.ListLiveLecturesParams{Limit: limit, Offset: offset})
}

func (s *Service) ListByInstructor(ctx context.Context, instructorID uuid.UUID, limit, offset int32) ([]db.Lecture, error) {
	return s.q.ListLecturesByInstructor(ctx, db.ListLecturesByInstructorParams{
		InstructorID: utils.UUIDToPg(instructorID),
		Limit:        limit,
		Offset:       offset,
	})
}

func (s *Service) Search(ctx context.Context, q string, limit, offset int32) ([]db.Lecture, error) {
	return s.q.SearchLectures(ctx, db.SearchLecturesParams{
		PlaintoTsquery: q,
		Limit:          limit,
		Offset:         offset,
	})
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req CreateLectureRequest) (*db.Lecture, error) {
	l, err := s.q.UpdateLecture(ctx, db.UpdateLectureParams{
		ID:              utils.UUIDToPg(id),
		Title:           req.Title,
		Description:     utils.TextToPg(req.Description),
		LectureType:     utils.TextToPg(req.LectureType),
		ThumbnailUrl:    utils.TextToPg(req.ThumbnailURL),
		ScheduledAt:     utils.TimestampPtrToPg(req.ScheduledAt),
		DurationSeconds: utils.Int4ToPg(req.DurationSeconds),
		Language:        utils.TextToPg(req.Language),
		IsFree:          utils.BoolToPg(req.IsFree),
		IsPublished:     utils.BoolToPg(req.IsPublished),
		DisplayOrder:    utils.Int4ToPg(req.DisplayOrder),
	})
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteLecture(ctx, utils.UUIDToPg(id))
}

func (s *Service) IncrementView(ctx context.Context, id uuid.UUID) error {
	return s.q.IncrementLectureViewCount(ctx, utils.UUIDToPg(id))
}

type RecordWatchRequest struct {
	LectureID       uuid.UUID `json:"lecture_id" validate:"required"`
	WatchedSeconds  int32     `json:"watched_seconds"`
	Completed       bool      `json:"completed"`
}

func (s *Service) RecordWatch(ctx context.Context, userID uuid.UUID, req RecordWatchRequest) error {
	_, err := s.q.UpsertLectureView(ctx, db.UpsertLectureViewParams{
		UserID:         utils.UUIDToPg(userID),
		LectureID:      utils.UUIDToPg(req.LectureID),
		WatchedSeconds: utils.Int4ToPg(req.WatchedSeconds),
		Completed:      utils.BoolToPg(req.Completed),
	})
	return err
}

func (s *Service) ListHistory(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]db.ListUserLectureHistoryRow, error) {
	return s.q.ListUserLectureHistory(ctx, db.ListUserLectureHistoryParams{
		UserID: utils.UUIDToPg(userID),
		Limit:  limit,
		Offset: offset,
	})
}
