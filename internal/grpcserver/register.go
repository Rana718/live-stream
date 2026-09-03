package grpcserver

import (
	coursesv1 "live-platform/gen/proto/live/courses/v1"
	"live-platform/internal/config"
	"live-platform/internal/courses"

	"google.golang.org/grpc"
)

// registerAll wires every domain adapter onto s. Each service is constructed
// exactly as cmd/server/main.go builds it (same builder chains), reusing the
// shared clients on d.
func registerAll(s *grpc.Server, cfg *config.Config, d Deps) {
	// --- Catalog ---
	coursesv1.RegisterCourseServiceServer(s, NewCourseServer(courses.NewService(d.Pool)))
}
