package grpcserver

import (
	"context"

	batchesv1 "live-platform/gen/proto/live/batches/v1"
	"live-platform/internal/batches"

	"github.com/google/uuid"
)

type BatchServer struct {
	batchesv1.UnimplementedBatchServiceServer
	svc *batches.Service
}

func NewBatchServer(svc *batches.Service) *BatchServer { return &BatchServer{svc: svc} }

func batchMsg(b batches.Batch) *batchesv1.Batch {
	return &batchesv1.Batch{
		Id: b.ID, CourseId: b.CourseID, Name: b.Name, Description: b.Description,
		InstructorId: b.InstructorID, StartsOn: tsFromTime(b.StartsOn), EndsOn: tsFromTime(b.EndsOn),
		MaxStudents: b.MaxStudents, IsActive: b.IsActive,
	}
}

func batchInput(in *batchesv1.BatchInput) (batches.CreateBatchRequest, error) {
	r := batches.CreateBatchRequest{
		Name: in.GetName(), Description: in.GetDescription(),
		MaxStudents: in.GetMaxStudents(), IsActive: in.GetIsActive(),
	}
	if in.GetCourseId() != "" {
		cid, err := parseUUID(in.GetCourseId(), "course_id")
		if err != nil {
			return r, err
		}
		r.CourseID = cid
	}
	inst, err := optUUID(in.GetInstructorId(), "instructor_id")
	if err != nil {
		return r, err
	}
	r.InstructorID = inst
	if in.GetStartsOn() != nil {
		t := in.GetStartsOn().AsTime()
		r.StartsOn = &t
	}
	if in.GetEndsOn() != nil {
		t := in.GetEndsOn().AsTime()
		r.EndsOn = &t
	}
	return r, nil
}

func (s *BatchServer) ListBatchesByCourse(ctx context.Context, req *batchesv1.ListBatchesByCourseRequest) (*batchesv1.ListBatchesByCourseResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListByCourse(ctx, c.TenantID, courseID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &batchesv1.ListBatchesByCourseResponse{}
	for _, b := range rows {
		out.Batches = append(out.Batches, batchMsg(b))
	}
	return out, nil
}

func (s *BatchServer) ListMyBatches(ctx context.Context, req *batchesv1.ListMyBatchesRequest) (*batchesv1.ListMyBatchesResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	var instructorID *uuid.UUID
	if c.Role == "instructor" {
		instructorID = &c.UserID
	}
	rows, err := s.svc.ListForTenant(ctx, c.TenantID, instructorID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &batchesv1.ListMyBatchesResponse{}
	for _, b := range rows {
		out.Batches = append(out.Batches, batchMsg(b))
	}
	return out, nil
}

func (s *BatchServer) GetBatch(ctx context.Context, req *batchesv1.GetBatchRequest) (*batchesv1.GetBatchResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	b, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &batchesv1.GetBatchResponse{Batch: batchMsg(b)}, nil
}

func (s *BatchServer) CreateBatch(ctx context.Context, req *batchesv1.CreateBatchRequest) (*batchesv1.CreateBatchResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	in, err := batchInput(req.GetBatch())
	if err != nil {
		return nil, err
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	b, err := s.svc.Create(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &batchesv1.CreateBatchResponse{Batch: batchMsg(b)}, nil
}

func (s *BatchServer) UpdateBatch(ctx context.Context, req *batchesv1.UpdateBatchRequest) (*batchesv1.UpdateBatchResponse, error) {
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
	in, err := batchInput(req.GetBatch())
	if err != nil {
		return nil, err
	}
	b, err := s.svc.Update(ctx, id, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &batchesv1.UpdateBatchResponse{Batch: batchMsg(b)}, nil
}

func (s *BatchServer) DeleteBatch(ctx context.Context, req *batchesv1.DeleteBatchRequest) (*batchesv1.DeleteBatchResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &batchesv1.DeleteBatchResponse{}, nil
}
