package grpcserver

import (
	"context"

	examsv1 "live-platform/gen/proto/live/exams/v1"
	"live-platform/internal/exams"
	"live-platform/internal/utils"
)

type ExamCategoryServer struct {
	examsv1.UnimplementedExamCategoryServiceServer
	svc *exams.Service
}

func NewExamCategoryServer(svc *exams.Service) *ExamCategoryServer {
	return &ExamCategoryServer{svc: svc}
}

func examMsg(id, parentID, name, slug string, desc, icon pgText, order int32, active bool) *examsv1.ExamCategory {
	return &examsv1.ExamCategory{
		Id: id, ParentId: parentID, Name: name, Slug: slug,
		Description: utils.TextFromPg(desc), IconUrl: utils.TextFromPg(icon),
		DisplayOrder: order, IsActive: active,
	}
}

func (s *ExamCategoryServer) ListExamCategories(ctx context.Context, _ *examsv1.ListExamCategoriesRequest) (*examsv1.ListExamCategoriesResponse, error) {
	rows, err := s.svc.List(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &examsv1.ListExamCategoriesResponse{}
	for _, r := range rows {
		out.Categories = append(out.Categories, examMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.ParentID), r.Name, r.Slug, r.Description, r.IconUrl, r.DisplayOrder, true))
	}
	return out, nil
}

func (s *ExamCategoryServer) GetExamCategory(ctx context.Context, req *examsv1.GetExamCategoryRequest) (*examsv1.GetExamCategoryResponse, error) {
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &examsv1.GetExamCategoryResponse{Category: examMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.ParentID), r.Name, r.Slug, r.Description, r.IconUrl, 0, true)}, nil
}

func examInput(in *examsv1.ExamCategoryInput) (exams.UpsertCategoryRequest, error) {
	r := exams.UpsertCategoryRequest{
		Name: in.GetName(), Slug: in.GetSlug(), Description: in.GetDescription(),
		IconURL: in.GetIconUrl(), DisplayOrder: in.GetDisplayOrder(),
	}
	if in.IsActive != nil {
		v := in.GetIsActive()
		r.IsActive = &v
	}
	p, err := optUUID(in.GetParentId(), "parent_id")
	if err != nil {
		return r, err
	}
	r.ParentID = p
	return r, nil
}

func (s *ExamCategoryServer) CreateExamCategory(ctx context.Context, req *examsv1.CreateExamCategoryRequest) (*examsv1.CreateExamCategoryResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if !c.Super {
		return nil, permDenied
	}
	in, err := examInput(req.GetCategory())
	if err != nil {
		return nil, err
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	r, err := s.svc.Create(ctx, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &examsv1.CreateExamCategoryResponse{Category: examMsg(utils.UUIDFromPg(r.ID), utils.UUIDFromPg(r.ParentID), r.Name, r.Slug, r.Description, r.IconUrl, r.DisplayOrder, true)}, nil
}

func (s *ExamCategoryServer) UpdateExamCategory(ctx context.Context, req *examsv1.UpdateExamCategoryRequest) (*examsv1.UpdateExamCategoryResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if !c.Super {
		return nil, permDenied
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	in, err := examInput(req.GetCategory())
	if err != nil {
		return nil, err
	}
	r, err := s.svc.Update(ctx, id, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &examsv1.UpdateExamCategoryResponse{Category: examMsg(utils.UUIDFromPg(r.ID), "", r.Name, r.Slug, r.Description, r.IconUrl, r.DisplayOrder, r.IsActive)}, nil
}

func (s *ExamCategoryServer) DeleteExamCategory(ctx context.Context, req *examsv1.DeleteExamCategoryRequest) (*examsv1.DeleteExamCategoryResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if !c.Super {
		return nil, permDenied
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &examsv1.DeleteExamCategoryResponse{}, nil
}
