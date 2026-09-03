package grpcserver

import (
	adminv1 "live-platform/gen/proto/live/admin/v1"
	assignmentsv1 "live-platform/gen/proto/live/assignments/v1"
	attendancev1 "live-platform/gen/proto/live/attendance/v1"
	auditv1 "live-platform/gen/proto/live/audit/v1"
	batchesv1 "live-platform/gen/proto/live/batches/v1"
	bookmarksv1 "live-platform/gen/proto/live/bookmarks/v1"
	chaptersv1 "live-platform/gen/proto/live/chapters/v1"
	coursesv1 "live-platform/gen/proto/live/courses/v1"
	devicesv1 "live-platform/gen/proto/live/devices/v1"
	doubtsv1 "live-platform/gen/proto/live/doubts/v1"
	enrollmentsv1 "live-platform/gen/proto/live/enrollments/v1"
	examsv1 "live-platform/gen/proto/live/exams/v1"
	lecturesv1 "live-platform/gen/proto/live/lectures/v1"
	subjectsv1 "live-platform/gen/proto/live/subjects/v1"
	topicsv1 "live-platform/gen/proto/live/topics/v1"
	usersv1 "live-platform/gen/proto/live/users/v1"
	"live-platform/internal/admin"
	"live-platform/internal/assignments"
	"live-platform/internal/attendance"
	"live-platform/internal/audit"
	"live-platform/internal/batches"
	"live-platform/internal/bookmarks"
	"live-platform/internal/chapters"
	"live-platform/internal/config"
	"live-platform/internal/courses"
	"live-platform/internal/devices"
	"live-platform/internal/doubts"
	"live-platform/internal/enrollments"
	"live-platform/internal/exams"
	"live-platform/internal/lectures"
	"live-platform/internal/subjects"
	"live-platform/internal/topics"
	"live-platform/internal/users"

	"google.golang.org/grpc"
)

// registerAll wires every domain adapter onto s. Each service is constructed
// exactly as cmd/server/main.go builds it (same builder chains), reusing the
// shared clients on d.
func registerAll(s *grpc.Server, cfg *config.Config, d Deps) {
	// --- Catalog / taxonomy ---
	coursesv1.RegisterCourseServiceServer(s, NewCourseServer(courses.NewService(d.Pool)))
	subjectsv1.RegisterSubjectServiceServer(s, NewSubjectServer(subjects.NewService(d.Pool)))
	chaptersv1.RegisterChapterServiceServer(s, NewChapterServer(chapters.NewService(d.Pool)))
	topicsv1.RegisterTopicServiceServer(s, NewTopicServer(topics.NewService(d.Pool)))
	examsv1.RegisterExamCategoryServiceServer(s, NewExamCategoryServer(exams.NewService(d.Pool)))
	batchesv1.RegisterBatchServiceServer(s, NewBatchServer(batches.NewService(d.Pool)))

	// --- Learning ---
	enrollmentsv1.RegisterEnrollmentServiceServer(s, NewEnrollmentServer(enrollments.NewService(d.Pool)))
	bookmarksv1.RegisterBookmarkServiceServer(s, NewBookmarkServer(bookmarks.NewService(d.Pool)))
	lecturesv1.RegisterLectureServiceServer(s, NewLectureServer(lectures.NewService(d.Pool)))
	doubtsv1.RegisterDoubtServiceServer(s, NewDoubtServer(doubts.NewService(d.Pool, d.Claude)))
	attendancev1.RegisterAttendanceServiceServer(s, NewAttendanceServer(attendance.NewService(d.Pool)))
	assignmentsv1.RegisterAssignmentServiceServer(s, NewAssignmentServer(assignments.NewService(d.Pool)))

	// --- Identity / tenant ops ---
	usersv1.RegisterUserServiceServer(s, NewUserServer(users.NewService(d.Pool)))
	devicesv1.RegisterDeviceServiceServer(s, NewDeviceServer(devices.NewService(d.Pool)))
	auditv1.RegisterAuditServiceServer(s, NewAuditServer(audit.NewService(d.Pool)))
	adminv1.RegisterAdminServiceServer(s, NewAdminServer(admin.NewService(d.Pool)))
}
