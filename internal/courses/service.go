package courses

import (
	"context"
	"regexp"
	"strings"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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
	Slug             string     `json:"slug"` // auto-derived from title when omitted
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
	// Price: the portal sends rupees in `price`; `price_minor` is the
	// canonical paise field. 0 / omitted leaves the course free.
	Price      float64 `json:"price"`
	PriceMinor int64   `json:"price_minor"`
}

func (r CreateCourseRequest) priceMinor() int64 {
	if r.PriceMinor > 0 {
		return r.PriceMinor
	}
	return int64(r.Price * 100)
}

func (r CreateCourseRequest) summary() string {
	if r.Summary != "" {
		return r.Summary
	}
	return r.Description
}

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	return strings.Trim(nonSlugChars.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func (s *Service) Create(ctx context.Context, tenantID, creator uuid.UUID, req CreateCourseRequest) (db.CreateCourseRow, error) {
	if req.Language == "" {
		req.Language = "en"
	}
	if req.Level == "" {
		req.Level = "foundation"
	}
	if req.Slug == "" {
		req.Slug = slugify(req.Title)
	}
	// Keep it unique per tenant — a colliding slug fails the partial unique
	// index; suffix a short random token.
	if _, err := s.q.GetCourseBySlug(ctx, db.GetCourseBySlugParams{
		TenantID: utils.UUIDToPg(tenantID), Slug: req.Slug,
	}); err == nil {
		req.Slug = req.Slug + "-" + uuid.NewString()[:6]
	}
	row, err := s.q.CreateCourse(ctx, db.CreateCourseParams{
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
	if err != nil {
		return row, err
	}
	if pm := req.priceMinor(); pm > 0 {
		_ = s.SetPrice(ctx, tenantID, uuid.UUID(row.ID.Bytes), pm, req.TaxRateBps)
	}
	return row, nil
}

// SetPrice ensures the course has a product row and an active price. Runs
// non-transactionally — a stale price is a display bug, not lost money
// (checkout re-reads it).
func (s *Service) SetPrice(ctx context.Context, tenantID, courseID uuid.UUID, amountMinor int64, taxRateBps int32) error {
	var productID pgtype.UUID
	if p, err := s.q.GetProductForCourse(ctx, utils.UUIDToPg(courseID)); err == nil {
		productID = p.ID
	} else {
		p, err := s.q.CreateProduct(ctx, db.CreateProductParams{
			TenantID:   utils.UUIDToPg(tenantID),
			Kind:       db.ProductKind("course"),
			CourseID:   utils.UUIDToPg(courseID),
			TaxRateBps: pgtype.Int4{Int32: taxRateBps, Valid: true},
		})
		if err != nil {
			return err
		}
		productID = p.ID
	}
	if cur, err := s.q.GetActivePrice(ctx, productID); err == nil && cur.AmountMinor == amountMinor {
		return nil
	}
	_ = s.q.DeactivatePricesForProduct(ctx, productID)
	_, err := s.q.UpsertActivePrice(ctx, db.UpsertActivePriceParams{
		TenantID: utils.UUIDToPg(tenantID), ProductID: productID, AmountMinor: amountMinor,
	})
	return err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.GetCourseRow, error) {
	return s.q.GetCourse(ctx, utils.UUIDToPg(id))
}

// GetPrice returns the course's active price in paise (0 if free/unpriced).
func (s *Service) GetPrice(ctx context.Context, courseID uuid.UUID) (int64, error) {
	p, err := s.q.GetProductForCourse(ctx, utils.UUIDToPg(courseID))
	if err != nil {
		return 0, err
	}
	pr, err := s.q.GetActivePrice(ctx, p.ID)
	if err != nil {
		return 0, err
	}
	return pr.AmountMinor, nil
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

func (s *Service) Update(ctx context.Context, tenantID, id uuid.UUID, req CreateCourseRequest) (db.UpdateCourseRow, error) {
	row, err := s.q.UpdateCourse(ctx, db.UpdateCourseParams{
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
	if err != nil {
		return row, err
	}
	if pm := req.priceMinor(); pm > 0 {
		_ = s.SetPrice(ctx, tenantID, id, pm, req.TaxRateBps)
	}
	return row, nil
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
