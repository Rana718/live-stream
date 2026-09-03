package grpcserver

import (
	chaptersv1 "live-platform/gen/proto/live/chapters/v1"
	coursesv1 "live-platform/gen/proto/live/courses/v1"
	examsv1 "live-platform/gen/proto/live/exams/v1"
	subjectsv1 "live-platform/gen/proto/live/subjects/v1"
	topicsv1 "live-platform/gen/proto/live/topics/v1"
	"live-platform/internal/chapters"
	"live-platform/internal/config"
	"live-platform/internal/courses"
	"live-platform/internal/exams"
	"live-platform/internal/subjects"
	"live-platform/internal/topics"

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
}
