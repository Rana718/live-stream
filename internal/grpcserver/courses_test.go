package grpcserver_test

import (
	"context"
	"testing"

	coursesv1 "live-platform/gen/proto/live/courses/v1"
	"live-platform/internal/courses"
	"live-platform/internal/grpcserver"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Adapter tests call the server methods directly with an identity-scoped
// context (WithTestIdentity) — gRPC does not propagate context values across
// the wire, so a bufconn client can't carry the test identity. Transport +
// interceptor wiring is covered separately by the grpcurl smoke in CI.
func TestCourseServer_CreateListGet(t *testing.T) {
	pool := testPool(t)
	tenantID, userID := seedTenantAndOwner(t, pool)
	srv := grpcserver.NewCourseServer(courses.NewService(pool))

	ownerCtx := grpcserver.WithTestIdentity(context.Background(), tenantID, userID, "owner")

	created, err := srv.CreateCourse(ownerCtx, &coursesv1.CreateCourseRequest{
		Title: "gRPC Physics", Summary: "adapter test", Language: "en", Level: "beginner",
		PriceMinor: 199900, HsnSac: "999293", TaxRateBps: 1800,
	})
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	if created.GetCourse().GetTitle() != "gRPC Physics" {
		t.Fatalf("title = %q", created.GetCourse().GetTitle())
	}
	if created.GetCourse().GetPrice().GetMinor() != 199900 {
		t.Fatalf("price minor = %d", created.GetCourse().GetPrice().GetMinor())
	}

	list, err := srv.ListCourses(ownerCtx, &coursesv1.ListCoursesRequest{IncludeUnpublished: true})
	if err != nil {
		t.Fatalf("ListCourses: %v", err)
	}
	if len(list.GetCourses()) == 0 {
		t.Fatal("expected >=1 course")
	}

	got, err := srv.GetCourse(ownerCtx, &coursesv1.GetCourseRequest{Id: created.GetCourse().GetId()})
	if err != nil {
		t.Fatalf("GetCourse: %v", err)
	}
	if got.GetCourse().GetId() != created.GetCourse().GetId() {
		t.Fatal("GetCourse returned a different course")
	}

	// student may not create
	studentCtx := grpcserver.WithTestIdentity(context.Background(), tenantID, userID, "student")
	if _, err := srv.CreateCourse(studentCtx, &coursesv1.CreateCourseRequest{Title: "nope", Language: "en", Level: "x"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}

	// missing identity → Unauthenticated
	if _, err := srv.ListCourses(context.Background(), &coursesv1.ListCoursesRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}

	// invalid uuid → InvalidArgument
	if _, err := srv.GetCourse(ownerCtx, &coursesv1.GetCourseRequest{Id: "not-a-uuid"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
}
