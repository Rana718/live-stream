package grpcserver

import (
	"context"
	"time"

	appbuildsv1 "live-platform/gen/proto/live/appbuilds/v1"
	materialsv1 "live-platform/gen/proto/live/materials/v1"
	recordingsv1 "live-platform/gen/proto/live/recordings/v1"
	streamsv1 "live-platform/gen/proto/live/streams/v1"
	"live-platform/internal/appbuilds"
	"live-platform/internal/materials"
	"live-platform/internal/recording"
	"live-platform/internal/stream"
	"live-platform/internal/utils"
)

// ─────────────────────────────────────────────────────────────── materials

type MaterialServer struct {
	materialsv1.UnimplementedMaterialServiceServer
	svc *materials.Service
}

func NewMaterialServer(svc *materials.Service) *MaterialServer { return &MaterialServer{svc: svc} }

func materialMsg(m materials.Material) *materialsv1.Material {
	return &materialsv1.Material{
		Id: m.ID, Title: m.Title, FileKey: m.FileKey, FileSize: m.FileSize, Mime: m.Mime, FileType: m.FileType,
	}
}

func (s *MaterialServer) ListMaterials(ctx context.Context, req *materialsv1.ListMaterialsRequest) (*materialsv1.ListMaterialsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListForTenant(ctx, c.TenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &materialsv1.ListMaterialsResponse{}
	for _, m := range rows {
		out.Materials = append(out.Materials, materialMsg(m))
	}
	return out, nil
}

func (s *MaterialServer) GetMaterial(ctx context.Context, req *materialsv1.GetMaterialRequest) (*materialsv1.GetMaterialResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	m, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &materialsv1.GetMaterialResponse{Material: materialMsg(m)}, nil
}

func (s *MaterialServer) GetMaterialDownloadUrl(ctx context.Context, req *materialsv1.GetMaterialDownloadUrlRequest) (*materialsv1.GetMaterialDownloadUrlResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	url, err := s.svc.GetDownloadURL(ctx, id, ttl)
	if err != nil {
		return nil, toStatus(err)
	}
	return &materialsv1.GetMaterialDownloadUrlResponse{Url: url}, nil
}

func (s *MaterialServer) DeleteMaterial(ctx context.Context, req *materialsv1.DeleteMaterialRequest) (*materialsv1.DeleteMaterialResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, c.TenantID, id); err != nil {
		return nil, toStatus(err)
	}
	return &materialsv1.DeleteMaterialResponse{}, nil
}

// ────────────────────────────────────────────────────────────── recordings

type RecordingServer struct {
	recordingsv1.UnimplementedRecordingServiceServer
	svc *recording.Service
}

func NewRecordingServer(svc *recording.Service) *RecordingServer { return &RecordingServer{svc: svc} }

func recordingMsg(r recording.Recording) *recordingsv1.Recording {
	return &recordingsv1.Recording{
		Id: r.ID, SessionId: r.SessionID, CourseId: r.CourseID, Title: r.Title, Status: r.Status,
		DurationSeconds: r.DurationSec, ThumbnailUrl: r.ThumbnailURL,
	}
}

func (s *RecordingServer) ListMyRecordings(ctx context.Context, req *recordingsv1.ListMyRecordingsRequest) (*recordingsv1.ListMyRecordingsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ForUser(ctx, c.TenantID, c.UserID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &recordingsv1.ListMyRecordingsResponse{}
	for _, r := range rows {
		out.Recordings = append(out.Recordings, recordingMsg(r))
	}
	return out, nil
}

func (s *RecordingServer) ListSessionRecordings(ctx context.Context, req *recordingsv1.ListSessionRecordingsRequest) (*recordingsv1.ListSessionRecordingsResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	sessionID, err := parseUUID(req.GetSessionId(), "session_id")
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.BySession(ctx, sessionID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &recordingsv1.ListSessionRecordingsResponse{}
	for _, r := range rows {
		out.Recordings = append(out.Recordings, recordingMsg(r))
	}
	return out, nil
}

func (s *RecordingServer) GetRecording(ctx context.Context, req *recordingsv1.GetRecordingRequest) (*recordingsv1.GetRecordingResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &recordingsv1.GetRecordingResponse{Recording: recordingMsg(r)}, nil
}

func (s *RecordingServer) GetRecordingPlayUrl(ctx context.Context, req *recordingsv1.GetRecordingPlayUrlRequest) (*recordingsv1.GetRecordingPlayUrlResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	url, err := s.svc.GetURL(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &recordingsv1.GetRecordingPlayUrlResponse{Url: url}, nil
}

// ─────────────────────────────────────────────────────────────── streams

type StreamServer struct {
	streamsv1.UnimplementedStreamServiceServer
	svc *stream.Service
}

func NewStreamServer(svc *stream.Service) *StreamServer { return &StreamServer{svc: svc} }

func sessionMsg(v stream.Session) *streamsv1.Session {
	return &streamsv1.Session{
		Id: v.ID, CourseId: v.CourseID, Title: v.Title, Description: v.Description, Status: v.Status,
		IngestKey: v.IngestKey, HlsUrl: v.HLSURL, RtmpUrl: v.RTMPURL, PeakViewers: v.PeakViewers,
		InstructorId: v.InstructorID, ScheduledAt: tsFromTime(v.ScheduledAt),
		StartedAt: tsFromTime(v.StartedAt), EndedAt: tsFromTime(v.EndedAt),
	}
}

func (s *StreamServer) CreateStream(ctx context.Context, req *streamsv1.CreateStreamRequest) (*streamsv1.CreateStreamResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	courseID, err := optUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	batchID, err := optUUID(req.GetBatchId(), "batch_id")
	if err != nil {
		return nil, err
	}
	in := stream.CreateStreamRequest{CourseID: courseID, BatchID: batchID, Title: req.GetTitle(), Description: req.GetDescription()}
	if req.GetScheduledAt() != nil {
		in.ScheduledAt = req.GetScheduledAt().AsTime()
	} else {
		in.ScheduledAt = time.Now()
	}
	v, err := s.svc.CreateStream(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &streamsv1.CreateStreamResponse{Session: sessionMsg(v)}, nil
}

func (s *StreamServer) GetStream(ctx context.Context, req *streamsv1.GetStreamRequest) (*streamsv1.GetStreamResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	v, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &streamsv1.GetStreamResponse{Session: sessionMsg(v)}, nil
}

func (s *StreamServer) ListLiveStreams(ctx context.Context, _ *streamsv1.ListLiveStreamsRequest) (*streamsv1.ListLiveStreamsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListLive(ctx, c.TenantID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &streamsv1.ListLiveStreamsResponse{}
	for _, v := range rows {
		out.Sessions = append(out.Sessions, sessionMsg(v))
	}
	return out, nil
}

func (s *StreamServer) StartStream(ctx context.Context, req *streamsv1.StartStreamRequest) (*streamsv1.StartStreamResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.Start(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &streamsv1.StartStreamResponse{}, nil
}

func (s *StreamServer) EndStream(ctx context.Context, req *streamsv1.EndStreamRequest) (*streamsv1.EndStreamResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.End(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &streamsv1.EndStreamResponse{}, nil
}

func (s *StreamServer) UpdateViewerCount(ctx context.Context, req *streamsv1.UpdateViewerCountRequest) (*streamsv1.UpdateViewerCountResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.UpdateViewerCount(ctx, id, req.GetCount()); err != nil {
		return nil, toStatus(err)
	}
	return &streamsv1.UpdateViewerCountResponse{}, nil
}

// ─────────────────────────────────────────────────────────────── appbuilds

type AppBuildServer struct {
	appbuildsv1.UnimplementedAppBuildServiceServer
	svc *appbuilds.Service
}

func NewAppBuildServer(svc *appbuilds.Service) *AppBuildServer { return &AppBuildServer{svc: svc} }

func (s *AppBuildServer) super(ctx context.Context) error {
	c, ok := callerFrom(ctx)
	if !ok || !c.Super {
		return permDenied
	}
	return nil
}

func (s *AppBuildServer) TriggerBuild(ctx context.Context, req *appbuildsv1.TriggerBuildRequest) (*appbuildsv1.TriggerBuildResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	tenantID, err := parseUUID(req.GetTenantId(), "tenant_id")
	if err != nil {
		return nil, err
	}
	in := appbuilds.TriggerInput{Platform: req.GetPlatform(), PackageID: req.GetPackageId(), VersionName: req.GetVersionName()}
	if err := validate(&in); err != nil {
		return nil, err
	}
	row, err := s.svc.Trigger(ctx, tenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &appbuildsv1.TriggerBuildResponse{Build: &appbuildsv1.Build{
		Id: utils.UUIDFromPg(row.ID), TenantId: utils.UUIDFromPg(row.TenantID), Platform: string(row.Platform),
		Status: string(row.Status), PackageId: utils.TextFromPg(row.PackageID), VersionName: utils.TextFromPg(row.VersionName),
		CreatedAt: tsFromPgtz(row.CreatedAt),
	}}, nil
}

func (s *AppBuildServer) ListBuilds(ctx context.Context, req *appbuildsv1.ListBuildsRequest) (*appbuildsv1.ListBuildsResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.List(ctx, req.GetStatus(), limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &appbuildsv1.ListBuildsResponse{}
	for _, b := range rows {
		out.Builds = append(out.Builds, &appbuildsv1.Build{
			Id: utils.UUIDFromPg(b.ID), TenantId: utils.UUIDFromPg(b.TenantID), Platform: string(b.Platform),
			Status: string(b.Status), PackageId: utils.TextFromPg(b.PackageID), VersionName: utils.TextFromPg(b.VersionName),
			BuildUrl: utils.TextFromPg(b.BuildUrl), StoreUrl: utils.TextFromPg(b.StoreUrl),
			TenantName: b.TenantName, OrgCode: b.OrgCode, CreatedAt: tsFromPgtz(b.CreatedAt),
		})
	}
	return out, nil
}

func (s *AppBuildServer) SetBuildStatus(ctx context.Context, req *appbuildsv1.SetBuildStatusRequest) (*appbuildsv1.SetBuildStatusResponse, error) {
	if err := s.super(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	in := appbuilds.SetStatusInput{
		Status: req.GetStatus(), BuildURL: req.GetBuildUrl(), StoreURL: req.GetStoreUrl(), ErrorLog: req.GetErrorLog(),
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	row, err := s.svc.SetStatus(ctx, id, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &appbuildsv1.SetBuildStatusResponse{Build: &appbuildsv1.Build{
		Id: utils.UUIDFromPg(row.ID), TenantId: utils.UUIDFromPg(row.TenantID), Platform: string(row.Platform),
		Status: string(row.Status), BuildUrl: utils.TextFromPg(row.BuildUrl), StoreUrl: utils.TextFromPg(row.StoreUrl),
		VersionName: utils.TextFromPg(row.VersionName),
	}}, nil
}
