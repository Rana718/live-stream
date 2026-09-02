// Package admin is the tenant_admin control plane — dashboard stats, member
// management, course approval. schema-v2: users are global, so role/status
// changes act on the tenant_users membership row; a "delete" removes the
// membership rather than the global user.
package admin

import (
	"context"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ q *db.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: db.New(pool)} }

func (s *Service) DashboardStats(ctx context.Context, tenantID uuid.UUID) (db.TenantDashboardStatsRow, error) {
	return s.q.TenantDashboardStats(ctx, utils.UUIDToPg(tenantID))
}

func (s *Service) ListMembers(ctx context.Context, tenantID uuid.UUID, role, q string, limit, offset int32) ([]db.AdminListTenantMembersRow, error) {
	var r db.NullTenantRole
	if role != "" {
		r = db.NullTenantRole{TenantRole: db.TenantRole(role), Valid: true}
	}
	qq := pgtype.Text{}
	if q != "" {
		qq = pgtype.Text{String: q, Valid: true}
	}
	return s.q.AdminListTenantMembers(ctx, db.AdminListTenantMembersParams{
		TenantID: utils.UUIDToPg(tenantID), Role: r, Q: qq, Limit: limit, Offset: offset,
	})
}

func (s *Service) BatchAttendance(ctx context.Context, tenantID uuid.UUID) ([]db.AdminBatchAttendanceRow, error) {
	return s.q.AdminBatchAttendance(ctx, utils.UUIDToPg(tenantID))
}

func (s *Service) ListPendingApproval(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.ListPendingCoursesRow, error) {
	return s.q.ListPendingCourses(ctx, db.ListPendingCoursesParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
}

func (s *Service) ApproveCourse(ctx context.Context, courseID, adminID uuid.UUID) (db.ApproveCourseRow, error) {
	return s.q.ApproveCourse(ctx, db.ApproveCourseParams{
		ID: utils.UUIDToPg(courseID), ApprovedBy: utils.UUIDToPg(adminID),
	})
}

func (s *Service) RejectCourse(ctx context.Context, courseID uuid.UUID, reason string) (db.RejectCourseRow, error) {
	return s.q.RejectCourse(ctx, db.RejectCourseParams{
		ID: utils.UUIDToPg(courseID), RejectionReason: utils.TextToPg(reason),
	})
}

type AdminUpdateUserRequest struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

func (s *Service) UpdateUser(ctx context.Context, tenantID, id uuid.UUID, req AdminUpdateUserRequest) (db.AdminGetTenantMemberRow, error) {
	_, err := s.q.UpdateUserProfileFields(ctx, db.UpdateUserProfileFieldsParams{
		ID:       utils.UUIDToPg(id),
		FullName: utils.TextToPg(req.FullName),
		Email:    utils.TextToPg(req.Email),
		Phone:    utils.TextToPg(req.Phone),
	})
	if err != nil {
		return db.AdminGetTenantMemberRow{}, err
	}
	return s.q.AdminGetTenantMember(ctx, db.AdminGetTenantMemberParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(id),
	})
}

func (s *Service) SetUserRole(ctx context.Context, tenantID, id uuid.UUID, role string) error {
	return s.q.SetTenantUserRole(ctx, db.SetTenantUserRoleParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(id), Role: db.TenantRole(role),
	})
}

func (s *Service) SetUserActive(ctx context.Context, tenantID, id uuid.UUID, active bool) error {
	status := db.MembershipStatus("suspended")
	if active {
		status = db.MembershipStatus("active")
	}
	return s.q.SetTenantUserStatus(ctx, db.SetTenantUserStatusParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(id), Status: status,
	})
}

func (s *Service) ResetUserPassword(ctx context.Context, id uuid.UUID, newHash string) error {
	return s.q.SetUserPassword(ctx, db.SetUserPasswordParams{
		ID: utils.UUIDToPg(id), PasswordHash: utils.TextToPg(newHash),
	})
}

// DeleteUser removes the tenant membership. The global user row is kept —
// they may belong to other tenants.
func (s *Service) DeleteUser(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.q.RemoveTenantUser(ctx, db.RemoveTenantUserParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(id),
	})
}
