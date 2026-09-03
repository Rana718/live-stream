package grpcserver

import (
	"bytes"
	"context"

	bulkimportv1 "live-platform/gen/proto/live/bulkimport/v1"
	downloadsv1 "live-platform/gen/proto/live/downloads/v1"
	sharev1 "live-platform/gen/proto/live/share/v1"
	"live-platform/internal/bulkimport"
	"live-platform/internal/downloads"
	"live-platform/internal/share"
)

// ─────────────────────────────────────────────────────────────── downloads

type DownloadServer struct {
	downloadsv1.UnimplementedDownloadServiceServer
	svc *downloads.Service
}

func NewDownloadServer(svc *downloads.Service) *DownloadServer { return &DownloadServer{svc: svc} }

func renditionMsg(r downloads.Rendition) *downloadsv1.Rendition {
	return &downloadsv1.Rendition{
		Height: r.Height, Quality: r.Quality, BitrateKbps: r.BitrateKbps, Codec: r.Codec,
		FileKey: r.FileKey, FileSize: r.FileSize,
	}
}

func (s *DownloadServer) CreateRendition(ctx context.Context, req *downloadsv1.CreateRenditionRequest) (*downloadsv1.CreateRenditionResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	assetID, err := parseUUID(req.GetVideoAssetId(), "video_asset_id")
	if err != nil {
		return nil, err
	}
	in := downloads.CreateVariantRequest{
		VideoAssetID: assetID, Height: req.GetHeight(), FileKey: req.GetFileKey(), FileSize: req.GetFileSize(),
		BitrateKbps: req.GetBitrateKbps(), Codec: req.GetCodec(),
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	r, err := s.svc.CreateVariant(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &downloadsv1.CreateRenditionResponse{Rendition: renditionMsg(r)}, nil
}

func (s *DownloadServer) ListRenditionsForVideo(ctx context.Context, req *downloadsv1.ListRenditionsForVideoRequest) (*downloadsv1.ListRenditionsForVideoResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	assetID, err := parseUUID(req.GetVideoAssetId(), "video_asset_id")
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListVariantsForLecture(ctx, assetID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &downloadsv1.ListRenditionsForVideoResponse{}
	for _, r := range rows {
		out.Renditions = append(out.Renditions, renditionMsg(r))
	}
	return out, nil
}

// ────────────────────────────────────────────────────────────── bulkimport

type BulkImportServer struct {
	bulkimportv1.UnimplementedBulkImportServiceServer
	svc *bulkimport.Service
}

func NewBulkImportServer(svc *bulkimport.Service) *BulkImportServer {
	return &BulkImportServer{svc: svc}
}

func (s *BulkImportServer) ImportUsers(ctx context.Context, req *bulkimportv1.ImportUsersRequest) (*bulkimportv1.ImportUsersResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	if len(req.GetCsv()) == 0 {
		return nil, invalidArg("csv is required")
	}
	res, err := s.svc.Import(ctx, c.TenantID, bytes.NewReader(req.GetCsv()))
	if err != nil {
		return nil, toStatus(err)
	}
	out := &bulkimportv1.ImportUsersResponse{
		Created: int32(res.Created), Updated: int32(res.Updated), Skipped: int32(res.Skipped),
	}
	for _, e := range res.RowErrors {
		out.RowErrors = append(out.RowErrors, &bulkimportv1.RowError{Row: int32(e.Row), Phone: e.Phone, Error: e.Err})
	}
	return out, nil
}

// ─────────────────────────────────────────────────────────────── share

type ShareServer struct {
	sharev1.UnimplementedShareServiceServer
	svc *share.Service
}

func NewShareServer(svc *share.Service) *ShareServer { return &ShareServer{svc: svc} }

func (s *ShareServer) RenderCoursePoster(ctx context.Context, req *sharev1.RenderCoursePosterRequest) (*sharev1.RenderCoursePosterResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	png, err := s.svc.Render(ctx, courseID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &sharev1.RenderCoursePosterResponse{Png: png}, nil
}
