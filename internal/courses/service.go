package courses

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// schema-v2: courses carry no price columns (prices live in the `prices`
// table, wired in Phase D), description is `summary` + `description_rich`
// jsonb, and publish state is the `status` publish_status enum.
type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

type CreateCourseRequest struct {
	ExamCategoryID   *uuid.UUID `json:"exam_category_id"`
	Title            string     `json:"title" validate:"required,min=3"`
	Slug             string     `json:"slug" validate:"required,min=3"`
	Summary          string     `json:"summary"`
	Description      string     `json:"description"` // legacy alias for summary
	ThumbnailURL     string     `json:"thumbnail_url"`
	PromoVideoURL    string     `json:"promo_video_url"`
	Language         string     `json:"language"`
	Level            string     `json:"level"`
	ClassLevel       string     `json:"class_level"`
	ExamGoal         string     `json:"exam_goal"`
	HsnSac           string     `json:"hsn_sac"`
	TaxRateBps       int32      `json:"tax_rate_bps"`
	RefundWindowDays int32      `json:"refund_window_days"`
}

func (r CreateCourseRequest) summary() string {
	if r.Summary != "" {
		return r.Summary
	}
	return r.Description
}

func (s *Service) Create(ctx context.Context, tenantID, creator uuid.UUID, req CreateCourseRequest) (db.CreateCourseRow, error) {
	if req.Language == "" {
		req.Language = "en"
	}
	if req.Level == "" {
		req.Level = "foundation"
	}
	return s.q.CreateCourse(ctx, db.CreateCourseParams{
		TenantID:         utils.UUIDToPg(tenantID),
		Title:            req.Title,
		ExamCategoryID:   utils.UUIDPtrToPg(req.ExamCategoryID),
		Slug:             req.Slug,
		Summary:          utils.TextToPg(req.summary()),
		ThumbnailUrl:     utils.TextToPg(req.ThumbnailURL),
		PromoVideoUrl:    utils.TextToPg(req.PromoVideoURL),
		Language:         utils.TextToPg(req.Language),
		Level:            utils.TextToPg(req.Level),
		ClassLevel:       utils.TextToPg(req.ClassLevel),
		ExamGoal:         utils.TextToPg(req.ExamGoal),
		HsnSac:           utils.TextToPg(req.HsnSac),
		TaxRateBps:       utils.Int4ToPg(req.TaxRateBps),
		RefundWindowDays: utils.Int4ToPg(req.RefundWindowDays),
		CreatedBy:        utils.UUIDToPg(creator),
	})
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.GetCourseRow, error) {
	return s.q.GetCourse(ctx, utils.UUIDToPg(id))
}

func (s *Service) GetBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (db.GetCourseBySlugRow, error) {
	return s.q.GetCourseBySlug(ctx, db.GetCourseBySlugParams{
		TenantID: utils.UUIDToPg(tenantID), Slug: slug,
	})
}

func (s *Service) ListPublished(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.ListPublishedCoursesRow, error) {
	return s.q.ListPublishedCourses(ctx, db.ListPublishedCoursesParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
}

func (s *Service) ListForAdmin(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.ListCoursesForAdminRow, error) {
	return s.q.ListCoursesForAdmin(ctx, db.ListCoursesForAdminParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
}

func (s *Service) ListPending(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.ListPendingCoursesRow, error) {
	return s.q.ListPendingCourses(ctx, db.ListPendingCoursesParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
}

func (s *Service) Search(ctx context.Context, tenantID uuid.UUID, q string, limit, offset int32) ([]db.SearchCoursesRow, error) {
	return s.q.SearchCourses(ctx, db.SearchCoursesParams{
		TenantID: utils.UUIDToPg(tenantID), Q: q, Limit: limit,
	})
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req CreateCourseRequest) (db.UpdateCourseRow, error) {
	return s.q.UpdateCourse(ctx, db.UpdateCourseParams{
		ID:               utils.UUIDToPg(id),
		Title:            utils.TextToPg(req.Title),
		Summary:          utils.TextToPg(req.summary()),
		ThumbnailUrl:     utils.TextToPg(req.ThumbnailURL),
		PromoVideoUrl:    utils.TextToPg(req.PromoVideoURL),
		Language:         utils.TextToPg(req.Language),
		Level:            utils.TextToPg(req.Level),
		ClassLevel:       utils.TextToPg(req.ClassLevel),
		ExamGoal:         utils.TextToPg(req.ExamGoal),
		HsnSac:           utils.TextToPg(req.HsnSac),
		TaxRateBps:       utils.Int4ToPg(req.TaxRateBps),
		RefundWindowDays: utils.Int4ToPg(req.RefundWindowDays),
	})
}

func (s *Service) SetPublished(ctx context.Context, id uuid.UUID, published bool) error {
	status := db.PublishStatus("draft")
	if published {
		status = db.PublishStatusPublished
	}
	return s.q.SetCourseStatus(ctx, db.SetCourseStatusParams{ID: utils.UUIDToPg(id), Status: status})
}

func (s *Service) Approve(ctx context.Context, id, approver uuid.UUID) (db.ApproveCourseRow, error) {
	return s.q.ApproveCourse(ctx, db.ApproveCourseParams{
		ID: utils.UUIDToPg(id), ApprovedBy: utils.UUIDToPg(approver),
	})
}

func (s *Service) Reject(ctx context.Context, id uuid.UUID, reason string) (db.RejectCourseRow, error) {
	return s.q.RejectCourse(ctx, db.RejectCourseParams{
		ID: utils.UUIDToPg(id), RejectionReason: utils.TextToPg(reason),
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteCourse(ctx, utils.UUIDToPg(id))
}
