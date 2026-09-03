package grpcserver

import (
	"context"
	"time"

	attendancev1 "live-platform/gen/proto/live/attendance/v1"
	"live-platform/internal/attendance"
	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
)

type AttendanceServer struct {
	attendancev1.UnimplementedAttendanceServiceServer
	svc *attendance.Service
}

func NewAttendanceServer(svc *attendance.Service) *AttendanceServer {
	return &AttendanceServer{svc: svc}
}

func attRecordMsg(r db.UpsertAttendanceRow) *attendancev1.AttendanceRecord {
	return &attendancev1.AttendanceRecord{
		UserId: utils.UUIDFromPg(r.UserID), SessionId: utils.UUIDFromPg(r.SessionID),
		Status: string(r.Status), WatchedSec: r.WatchedSec, Method: r.Method,
	}
}

func (s *AttendanceServer) AutoMark(ctx context.Context, req *attendancev1.AutoMarkRequest) (*attendancev1.AutoMarkResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	sid, err := optUUID(req.GetSessionId(), "session_id")
	if err != nil {
		return nil, err
	}
	bid, err := optUUID(req.GetBatchId(), "batch_id")
	if err != nil {
		return nil, err
	}
	in := attendance.AutoMarkRequest{SessionID: sid, BatchID: bid, WatchedSeconds: req.GetWatchedSeconds()}
	if req.GetJoinTime() != nil {
		in.JoinTime = req.GetJoinTime().AsTime()
	} else {
		in.JoinTime = time.Now()
	}
	r, err := s.svc.AutoMark(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &attendancev1.AutoMarkResponse{Record: attRecordMsg(r)}, nil
}

func (s *AttendanceServer) ManualMark(ctx context.Context, req *attendancev1.ManualMarkRequest) (*attendancev1.ManualMarkResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	userID, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	sessionID, err := parseUUID(req.GetSessionId(), "session_id")
	if err != nil {
		return nil, err
	}
	in := attendance.ManualMarkRequest{UserID: userID, SessionID: sessionID, Status: req.GetStatus(), Notes: req.GetNotes()}
	if err := validate(&in); err != nil {
		return nil, err
	}
	r, err := s.svc.ManualMark(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &attendancev1.ManualMarkResponse{Record: attRecordMsg(r)}, nil
}

func (s *AttendanceServer) BulkMark(ctx context.Context, req *attendancev1.BulkMarkRequest) (*attendancev1.BulkMarkResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	sessionID, err := parseUUID(req.GetSessionId(), "session_id")
	if err != nil {
		return nil, err
	}
	items := make([]attendance.BulkMarkItem, 0, len(req.GetItems()))
	for _, it := range req.GetItems() {
		uid, perr := uuid.Parse(it.GetUserId())
		if perr != nil {
			continue
		}
		items = append(items, attendance.BulkMarkItem{UserID: uid, Status: it.GetStatus()})
	}
	n, err := s.svc.BulkMark(ctx, c.TenantID, c.UserID, sessionID, items)
	if err != nil {
		return nil, toStatus(err)
	}
	return &attendancev1.BulkMarkResponse{Marked: int32(n)}, nil
}

func (s *AttendanceServer) ListBySession(ctx context.Context, req *attendancev1.ListBySessionRequest) (*attendancev1.ListBySessionResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	sessionID, err := parseUUID(req.GetSessionId(), "session_id")
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListBySession(ctx, c.TenantID, sessionID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &attendancev1.ListBySessionResponse{}
	for _, r := range rows {
		out.Records = append(out.Records, &attendancev1.AttendanceRecord{
			UserId: utils.UUIDFromPg(r.UserID), Status: string(r.Status), WatchedSec: r.WatchedSec,
			Method: r.Method, FullName: utils.TextFromPg(r.FullName),
		})
	}
	return out, nil
}

func (s *AttendanceServer) ListMyAttendance(ctx context.Context, req *attendancev1.ListMyAttendanceRequest) (*attendancev1.ListMyAttendanceResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListMine(ctx, c.TenantID, c.UserID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &attendancev1.ListMyAttendanceResponse{}
	for _, r := range rows {
		out.Rows = append(out.Rows, &attendancev1.ListMyAttendanceResponse_Row{
			SessionId: utils.UUIDFromPg(r.SessionID), Status: string(r.Status), WatchedSec: r.WatchedSec,
			Title: r.Title, ScheduledStart: tsFromPgtz(r.ScheduledStart),
		})
	}
	return out, nil
}

func (s *AttendanceServer) MyStats(ctx context.Context, _ *attendancev1.MyStatsRequest) (*attendancev1.MyStatsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	st, err := s.svc.Stats(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &attendancev1.MyStatsResponse{Stats: &attendancev1.Stats{
		Total: st.Total, Present: st.Present, Absent: st.Absent, Percentage: int32(st.Percentage),
	}}, nil
}

func (s *AttendanceServer) CreateQrCode(ctx context.Context, req *attendancev1.CreateQrCodeRequest) (*attendancev1.CreateQrCodeResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	sessionID, err := parseUUID(req.GetSessionId(), "session_id")
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(req.GetTtlMinutes()) * time.Minute
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	r, err := s.svc.CreateQRCode(ctx, c.TenantID, sessionID, c.UserID, ttl)
	if err != nil {
		return nil, toStatus(err)
	}
	return &attendancev1.CreateQrCodeResponse{Code: r.Code, ExpiresAt: tsFromPgtz(r.ExpiresAt)}, nil
}

func (s *AttendanceServer) QrCheckIn(ctx context.Context, req *attendancev1.QrCheckInRequest) (*attendancev1.QrCheckInResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	in := attendance.QRCheckInRequest{Code: req.GetCode()}
	if err := validate(&in); err != nil {
		return nil, err
	}
	r, err := s.svc.QRCheckIn(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &attendancev1.QrCheckInResponse{Record: attRecordMsg(r)}, nil
}
