package courses

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type CreateCourseRequest struct {
	ExamCategoryID  *uuid.UUID `json:"exam_category_id"`
	Title           string     `json:"title" validate:"required,min=3"`
	Slug            string     `json:"slug" validate:"required,min=3"`
	Description     string     `json:"description"`
	ThumbnailURL    string     `json:"thumbnail_url"`
	Price           float64    `json:"price"`
	DiscountedPrice float64    `json:"discounted_price"`
	DurationMonths  int32      `json:"duration_months"`
	Language        string     `json:"language"`
	Level           string     `json:"level"`
	IsFree          bool       `json:"is_free"`
	IsPublished     bool       `json:"is_published"`
}

func (s *Service) Create(ctx context.Context, creator uuid.UUID, req CreateCourseRequest) (*db.Course, error) {
	if req.Language == "" {
		req.Language = "en"
	}
	if req.Level == "" {
		req.Level = "foundation"
	}
	c, err := s.q.CreateCourse(ctx, db.CreateCourseParams{
		ExamCategoryID:  utils.UUIDPtrToPg(req.ExamCategoryID),
		Title:           req.Title,
		Slug:            req.Slug,
		Description:     utils.TextToPg(req.Description),
		ThumbnailUrl:    utils.TextToPg(req.ThumbnailURL),
		Price:           utils.NumericFromFloat(req.Price),
		DiscountedPrice: utils.NumericFromFloat(req.DiscountedPrice),
		DurationMonths:  utils.Int4ToPg(req.DurationMonths),
		Language:        utils.TextToPg(req.Language),
		Level:           utils.TextToPg(req.Level),
		IsFree:          utils.BoolToPg(req.IsFree),
		IsPublished:     utils.BoolToPg(req.IsPublished),
		CreatedBy:       utils.UUIDToPg(creator),
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*db.Course, error) {
	c, err := s.q.GetCourseByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (*db.Course, error) {
	c, err := s.q.GetCourseBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) ListPublished(ctx context.Context, limit, offset int32) ([]db.Course, error) {
	return s.q.ListPublishedCourses(ctx, db.ListPublishedCoursesParams{Limit: limit, Offset: offset})
}

func (s *Service) ListByExamCategory(ctx context.Context, examID uuid.UUID, limit, offset int32) ([]db.Course, error) {
	return s.q.ListCoursesByExamCategory(ctx, db.ListCoursesByExamCategoryParams{
		ExamCategoryID: utils.UUIDToPg(examID),
		Limit:          limit,
		Offset:         offset,
	})
}

func (s *Service) ListByLanguage(ctx context.Context, lang string, limit, offset int32) ([]db.Course, error) {
	return s.q.ListCoursesByLanguage(ctx, db.ListCoursesByLanguageParams{
		Language: utils.TextToPg(lang),
		Limit:    limit,
		Offset:   offset,
	})
}

func (s *Service) Search(ctx context.Context, q string, limit, offset int32) ([]db.Course, error) {
	return s.q.SearchCourses(ctx, db.SearchCoursesParams{
		PlaintoTsquery: q,
		Limit:          limit,
		Offset:         offset,
	})
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req CreateCourseRequest) (*db.Course, error) {
	c, err := s.q.UpdateCourse(ctx, db.UpdateCourseParams{
		ID:              utils.UUIDToPg(id),
		Title:           req.Title,
		Description:     utils.TextToPg(req.Description),
		ThumbnailUrl:    utils.TextToPg(req.ThumbnailURL),
		Price:           utils.NumericFromFloat(req.Price),
		DiscountedPrice: utils.NumericFromFloat(req.DiscountedPrice),
		DurationMonths:  utils.Int4ToPg(req.DurationMonths),
		Language:        utils.TextToPg(req.Language),
		Level:           utils.TextToPg(req.Level),
		IsFree:          utils.BoolToPg(req.IsFree),
		IsPublished:     utils.BoolToPg(req.IsPublished),
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteCourse(ctx, utils.UUIDToPg(id))
}
