// Integration tests for enrollment creation, idempotent re-enrollment,
// batch student-count tracking, and cancellation.
//
// Skipped automatically if TEST_DATABASE_URL is unset, same convention as
// internal/courseorders/service_test.go.
package enrollments_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"live-platform/internal/config"
	"live-platform/internal/database"
	"live-platform/internal/enrollments"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping")
	}
	pool, err := database.NewPostgresPool(&config.DatabaseConfig{
		Host: "localhost", Port: "5432",
		User: "app_user", Password: "app_user_dev_password",
		DBName: "live_platform", SSLMode: "disable",
	})
	if err != nil {
		t.Fatalf("connect as app_user: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// fixtures creates a tenant, a user, a course, and a batch under that
// course (max_students=10, current_students=0).
func fixtures(t *testing.T, pool *pgxpool.Pool) (tenantID, userID, courseID, batchID uuid.UUID) {
	t.Helper()
	superCtx := database.WithSuperAdmin(context.Background())

	tenantID = uuid.New()
	if _, err := pool.Exec(superCtx, `
		INSERT INTO tenants (id, org_code, name, slug, plan, status)
		VALUES ($1, $2, $3, $4, 'starter', 'active')
	`, tenantID, "EN"+tenantID.String()[:6], "en-"+tenantID.String()[:6], "en-"+tenantID.String()[:6]); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	userID = uuid.New()
	if _, err := pool.Exec(superCtx, `INSERT INTO users (id, tenant_id, role) VALUES ($1, $2, 'student')`, userID, tenantID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := pool.QueryRow(superCtx, `
		INSERT INTO courses (tenant_id, title, slug) VALUES ($1, 'Enroll Test Course', $2) RETURNING id
	`, tenantID, "enroll-slug-"+userID.String()[:8]).Scan(&courseID); err != nil {
		t.Fatalf("insert course: %v", err)
	}

	if err := pool.QueryRow(superCtx, `
		INSERT INTO batches (tenant_id, course_id, name, max_students, current_students)
		VALUES ($1, $2, 'Batch A', 10, 0) RETURNING id
	`, tenantID, courseID).Scan(&batchID); err != nil {
		t.Fatalf("insert batch: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(superCtx, "DELETE FROM tenants WHERE id = $1", tenantID)
	})
	return tenantID, userID, courseID, batchID
}

func TestEnroll_CreatesAndIncrementsBatchCount(t *testing.T) {
	pool := testPool(t)
	svc := enrollments.NewService(pool)
	tenantID, userID, courseID, batchID := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	e, err := svc.Enroll(ctx, tenantID, userID, enrollments.EnrollRequest{
		CourseID: courseID,
		BatchID:  &batchID,
	})
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if e.Status.String != "active" {
		t.Fatalf("expected status=active, got %q", e.Status.String)
	}

	var currentStudents int32
	superCtx := database.WithSuperAdmin(context.Background())
	if err := pool.QueryRow(superCtx, "SELECT current_students FROM batches WHERE id = $1", batchID).Scan(&currentStudents); err != nil {
		t.Fatalf("read batch: %v", err)
	}
	if currentStudents != 1 {
		t.Fatalf("expected current_students=1 after enroll, got %d", currentStudents)
	}
}

func TestEnroll_IsIdempotentOnReEnroll(t *testing.T) {
	pool := testPool(t)
	svc := enrollments.NewService(pool)
	tenantID, userID, courseID, _ := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	if _, err := svc.Enroll(ctx, tenantID, userID, enrollments.EnrollRequest{CourseID: courseID}); err != nil {
		t.Fatalf("first Enroll: %v", err)
	}
	if _, err := svc.Enroll(ctx, tenantID, userID, enrollments.EnrollRequest{CourseID: courseID}); err != nil {
		t.Fatalf("second Enroll (re-enroll): %v", err)
	}

	var count int
	superCtx := database.WithSuperAdmin(context.Background())
	if err := pool.QueryRow(superCtx,
		"SELECT count(*) FROM enrollments WHERE user_id = $1 AND course_id = $2",
		userID, courseID).Scan(&count); err != nil {
		t.Fatalf("count enrollments: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 enrollment row after re-enroll, got %d", count)
	}
}

func TestUpdateProgress(t *testing.T) {
	pool := testPool(t)
	svc := enrollments.NewService(pool)
	tenantID, userID, courseID, _ := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	e, err := svc.Enroll(ctx, tenantID, userID, enrollments.EnrollRequest{CourseID: courseID})
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}

	if err := svc.UpdateProgress(ctx, uuid.UUID(e.ID.Bytes), 42.5); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}

	got, err := svc.Get(ctx, userID, courseID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	gotPercent, _ := got.ProgressPercent.Float64Value()
	if gotPercent.Float64 != 42.5 {
		t.Fatalf("expected progress_percent=42.5, got %v", gotPercent.Float64)
	}
}

func TestCancel(t *testing.T) {
	pool := testPool(t)
	svc := enrollments.NewService(pool)
	tenantID, userID, courseID, _ := fixtures(t, pool)
	ctx := database.WithTenant(context.Background(), tenantID.String(), userID.String())

	if _, err := svc.Enroll(ctx, tenantID, userID, enrollments.EnrollRequest{CourseID: courseID}); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if err := svc.Cancel(ctx, userID, courseID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	got, err := svc.Get(ctx, userID, courseID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status.String != "cancelled" {
		t.Fatalf("expected status=cancelled, got %q", got.Status.String)
	}
}
