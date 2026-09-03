package grpcserver

import (
	"context"

	assignmentsv1 "live-platform/gen/proto/live/assignments/v1"
	"live-platform/internal/assignments"
	"live-platform/internal/utils"

	"github.com/google/uuid"
)

type AssignmentServer struct {
	assignmentsv1.UnimplementedAssignmentServiceServer
	svc *assignments.Service
}

func NewAssignmentServer(svc *assignments.Service) *AssignmentServer {
	return &AssignmentServer{svc: svc}
}

func (s *AssignmentServer) ListAssignments(ctx context.Context, req *assignmentsv1.ListAssignmentsRequest) (*assignmentsv1.ListAssignmentsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
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
	var mine *uuid.UUID
	if req.GetMineOnly() && c.require(rolesInstructorUp...) == nil {
		u := c.UserID
		mine = &u
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.List(ctx, c.TenantID, courseID, batchID, mine, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &assignmentsv1.ListAssignmentsResponse{}
	for _, a := range rows {
		out.Assignments = append(out.Assignments, &assignmentsv1.Assignment{
			Id: utils.UUIDFromPg(a.ID), CourseId: utils.UUIDFromPg(a.CourseID), BatchId: utils.UUIDFromPg(a.BatchID),
			Title: a.Title, Description: utils.TextFromPg(a.Description), AttachmentUrl: utils.TextFromPg(a.AttachmentUrl),
			DueAt: tsFromPgtz(a.DueAt), MaxMarks: utils.NumericToFloat(a.MaxMarks), Status: string(a.Status),
		})
	}
	return out, nil
}

func (s *AssignmentServer) GetAssignment(ctx context.Context, req *assignmentsv1.GetAssignmentRequest) (*assignmentsv1.GetAssignmentResponse, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	a, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &assignmentsv1.GetAssignmentResponse{Assignment: &assignmentsv1.Assignment{
		Id: utils.UUIDFromPg(a.ID), CourseId: utils.UUIDFromPg(a.CourseID), BatchId: utils.UUIDFromPg(a.BatchID),
		Title: a.Title, Description: utils.TextFromPg(a.Description), AttachmentUrl: utils.TextFromPg(a.AttachmentUrl),
		DueAt: tsFromPgtz(a.DueAt), MaxMarks: utils.NumericToFloat(a.MaxMarks), Status: string(a.Status),
	}}, nil
}

func assignmentInput(in *assignmentsv1.AssignmentInput) (assignments.CreateAssignmentRequest, error) {
	r := assignments.CreateAssignmentRequest{
		Title: in.GetTitle(), Description: in.GetDescription(), AttachmentURL: in.GetAttachmentUrl(),
		MaxMarks: in.GetMaxMarks(), IsPublished: in.GetIsPublished(),
	}
	cid, err := optUUID(in.GetCourseId(), "course_id")
	if err != nil {
		return r, err
	}
	bid, err := optUUID(in.GetBatchId(), "batch_id")
	if err != nil {
		return r, err
	}
	lid, err := optUUID(in.GetLessonId(), "lesson_id")
	if err != nil {
		return r, err
	}
	r.CourseID, r.BatchID, r.LessonID = cid, bid, lid
	if in.GetDueAt() != nil {
		t := in.GetDueAt().AsTime()
		r.DueAt = &t
	}
	return r, nil
}

func (s *AssignmentServer) CreateAssignment(ctx context.Context, req *assignmentsv1.CreateAssignmentRequest) (*assignmentsv1.CreateAssignmentResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	in, err := assignmentInput(req.GetAssignment())
	if err != nil {
		return nil, err
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	a, err := s.svc.Create(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &assignmentsv1.CreateAssignmentResponse{Assignment: &assignmentsv1.Assignment{
		Id: utils.UUIDFromPg(a.ID), Title: a.Title, DueAt: tsFromPgtz(a.DueAt),
		MaxMarks: utils.NumericToFloat(a.MaxMarks), Status: string(a.Status),
	}}, nil
}

func (s *AssignmentServer) UpdateAssignment(ctx context.Context, req *assignmentsv1.UpdateAssignmentRequest) (*assignmentsv1.UpdateAssignmentResponse, error) {
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
	in, err := assignmentInput(req.GetAssignment())
	if err != nil {
		return nil, err
	}
	a, err := s.svc.Update(ctx, id, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &assignmentsv1.UpdateAssignmentResponse{Assignment: &assignmentsv1.Assignment{
		Id: utils.UUIDFromPg(a.ID), Title: a.Title, Description: utils.TextFromPg(a.Description),
		AttachmentUrl: utils.TextFromPg(a.AttachmentUrl), DueAt: tsFromPgtz(a.DueAt),
		MaxMarks: utils.NumericToFloat(a.MaxMarks), Status: string(a.Status),
	}}, nil
}

func (s *AssignmentServer) SetAssignmentPublished(ctx context.Context, req *assignmentsv1.SetAssignmentPublishedRequest) (*assignmentsv1.SetAssignmentPublishedResponse, error) {
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
	if err := s.svc.SetPublished(ctx, id, req.GetPublished()); err != nil {
		return nil, toStatus(err)
	}
	return &assignmentsv1.SetAssignmentPublishedResponse{}, nil
}

func (s *AssignmentServer) DeleteAssignment(ctx context.Context, req *assignmentsv1.DeleteAssignmentRequest) (*assignmentsv1.DeleteAssignmentResponse, error) {
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
	if err := s.svc.Delete(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &assignmentsv1.DeleteAssignmentResponse{}, nil
}

func (s *AssignmentServer) SubmitAssignment(ctx context.Context, req *assignmentsv1.SubmitAssignmentRequest) (*assignmentsv1.SubmitAssignmentResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	assignmentID, err := parseUUID(req.GetAssignmentId(), "assignment_id")
	if err != nil {
		return nil, err
	}
	in := assignments.SubmitRequest{AssignmentID: assignmentID, SubmissionText: req.GetSubmissionText(), FileKey: req.GetFileKey()}
	if err := validate(&in); err != nil {
		return nil, err
	}
	r, err := s.svc.Submit(ctx, c.TenantID, c.UserID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &assignmentsv1.SubmitAssignmentResponse{Submission: &assignmentsv1.Submission{
		Id: utils.UUIDFromPg(r.ID), AssignmentId: utils.UUIDFromPg(r.AssignmentID), UserId: utils.UUIDFromPg(r.UserID),
		Status: r.Status, SubmittedAt: tsFromPgtz(r.SubmittedAt),
	}}, nil
}

func (s *AssignmentServer) GetMySubmission(ctx context.Context, req *assignmentsv1.GetMySubmissionRequest) (*assignmentsv1.GetMySubmissionResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	assignmentID, err := parseUUID(req.GetAssignmentId(), "assignment_id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.GetMySubmission(ctx, c.UserID, assignmentID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &assignmentsv1.GetMySubmissionResponse{Submission: &assignmentsv1.Submission{
		Id: utils.UUIDFromPg(r.ID), SubmissionText: utils.TextFromPg(r.SubmissionText), FileKey: utils.TextFromPg(r.FileKey),
		MarksObtained: utils.NumericToFloat(r.MarksObtained), Feedback: utils.TextFromPg(r.Feedback),
		Status: r.Status, SubmittedAt: tsFromPgtz(r.SubmittedAt),
	}}, nil
}

func (s *AssignmentServer) ListSubmissions(ctx context.Context, req *assignmentsv1.ListSubmissionsRequest) (*assignmentsv1.ListSubmissionsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	assignmentID, err := parseUUID(req.GetAssignmentId(), "assignment_id")
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListSubmissions(ctx, assignmentID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &assignmentsv1.ListSubmissionsResponse{}
	for _, r := range rows {
		out.Submissions = append(out.Submissions, &assignmentsv1.Submission{
			Id: utils.UUIDFromPg(r.ID), UserId: utils.UUIDFromPg(r.UserID), MarksObtained: utils.NumericToFloat(r.MarksObtained),
			Status: r.Status, FullName: utils.TextFromPg(r.FullName), SubmittedAt: tsFromPgtz(r.SubmittedAt),
		})
	}
	return out, nil
}

func (s *AssignmentServer) ListMySubmissions(ctx context.Context, req *assignmentsv1.ListMySubmissionsRequest) (*assignmentsv1.ListMySubmissionsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListMySubmissions(ctx, c.TenantID, c.UserID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &assignmentsv1.ListMySubmissionsResponse{}
	for _, r := range rows {
		out.Submissions = append(out.Submissions, &assignmentsv1.Submission{
			Id: utils.UUIDFromPg(r.ID), AssignmentId: utils.UUIDFromPg(r.AssignmentID), SubmissionText: utils.TextFromPg(r.SubmissionText),
			FileKey: utils.TextFromPg(r.FileKey), MarksObtained: utils.NumericToFloat(r.MarksObtained),
			Feedback: utils.TextFromPg(r.Feedback), Status: r.Status, SubmittedAt: tsFromPgtz(r.SubmittedAt),
		})
	}
	return out, nil
}

func (s *AssignmentServer) GradeSubmission(ctx context.Context, req *assignmentsv1.GradeSubmissionRequest) (*assignmentsv1.GradeSubmissionResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	submissionID, err := parseUUID(req.GetSubmissionId(), "submission_id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.Grade(ctx, c.UserID, submissionID, assignments.GradeRequest{
		MarksObtained: req.GetMarksObtained(), Feedback: req.GetFeedback(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &assignmentsv1.GradeSubmissionResponse{Submission: &assignmentsv1.Submission{
		Id: utils.UUIDFromPg(r.ID), AssignmentId: utils.UUIDFromPg(r.AssignmentID), UserId: utils.UUIDFromPg(r.UserID),
		MarksObtained: utils.NumericToFloat(r.MarksObtained), Status: r.Status,
	}}, nil
}
