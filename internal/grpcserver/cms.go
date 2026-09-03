package grpcserver

import (
	"context"

	cmsv1 "live-platform/gen/proto/live/cms/v1"
	"live-platform/internal/cms"
	"live-platform/internal/utils"
)

type CmsServer struct {
	cmsv1.UnimplementedCmsServiceServer
	svc *cms.Service
}

func NewCmsServer(svc *cms.Service) *CmsServer { return &CmsServer{svc: svc} }

func (s *CmsServer) requireSuper(ctx context.Context) error {
	c, err := requireTenant(ctx)
	if err != nil {
		return err
	}
	if !c.Super {
		return permDenied
	}
	return nil
}

func (s *CmsServer) ListPublishedPosts(ctx context.Context, req *cmsv1.ListPublishedPostsRequest) (*cmsv1.ListPublishedPostsResponse, error) {
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.ListPublishedPosts(ctx, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &cmsv1.ListPublishedPostsResponse{}
	for _, p := range rows {
		out.Posts = append(out.Posts, &cmsv1.BlogPost{
			Id: utils.UUIDFromPg(p.ID), Slug: p.Slug, Title: p.Title, Excerpt: utils.TextFromPg(p.Excerpt),
			CoverUrl: utils.TextFromPg(p.CoverUrl), AuthorName: utils.TextFromPg(p.AuthorName), Tags: p.Tags,
			MinutesRead: p.MinutesRead, PublishedAt: tsFromPgtz(p.PublishedAt),
		})
	}
	return out, nil
}

func (s *CmsServer) GetPostBySlug(ctx context.Context, req *cmsv1.GetPostBySlugRequest) (*cmsv1.GetPostBySlugResponse, error) {
	p, err := s.svc.GetPostBySlug(ctx, req.GetSlug())
	if err != nil {
		return nil, toStatus(err)
	}
	return &cmsv1.GetPostBySlugResponse{Post: &cmsv1.BlogPost{
		Id: utils.UUIDFromPg(p.ID), Slug: p.Slug, Title: p.Title, Excerpt: utils.TextFromPg(p.Excerpt),
		BodyHtml: p.BodyHtml, CoverUrl: utils.TextFromPg(p.CoverUrl), AuthorName: utils.TextFromPg(p.AuthorName),
		Tags: p.Tags, MinutesRead: p.MinutesRead, PublishedAt: tsFromPgtz(p.PublishedAt),
	}}, nil
}

func (s *CmsServer) AdminListPosts(ctx context.Context, req *cmsv1.AdminListPostsRequest) (*cmsv1.AdminListPostsResponse, error) {
	if err := s.requireSuper(ctx); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.AdminListPosts(ctx, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &cmsv1.AdminListPostsResponse{}
	for _, p := range rows {
		out.Posts = append(out.Posts, &cmsv1.BlogPost{
			Id: utils.UUIDFromPg(p.ID), Slug: p.Slug, Title: p.Title, Excerpt: utils.TextFromPg(p.Excerpt),
			CoverUrl: utils.TextFromPg(p.CoverUrl), AuthorName: utils.TextFromPg(p.AuthorName), Tags: p.Tags,
			MinutesRead: p.MinutesRead, PublishedAt: tsFromPgtz(p.PublishedAt),
		})
	}
	return out, nil
}

func (s *CmsServer) UpsertPost(ctx context.Context, req *cmsv1.UpsertPostRequest) (*cmsv1.UpsertPostResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if !c.Super {
		return nil, permDenied
	}
	in := cms.PostInput{
		Slug: req.GetSlug(), Title: req.GetTitle(), Excerpt: req.GetExcerpt(), BodyJSON: req.GetBodyJson(),
		BodyHTML: req.GetBodyHtml(), CoverURL: req.GetCoverUrl(), AuthorName: req.GetAuthorName(), Tags: req.GetTags(),
		MinutesRead: req.GetMinutesRead(), SeoTitle: req.GetSeoTitle(), SeoDesc: req.GetSeoDesc(),
	}
	if req.GetPublishedAt() != nil {
		t := req.GetPublishedAt().AsTime()
		in.PublishedAt = &t
	}
	if req.GetId() == "" {
		r, err := s.svc.CreatePost(ctx, in, c.UserID)
		if err != nil {
			return nil, toStatus(err)
		}
		return &cmsv1.UpsertPostResponse{Post: &cmsv1.BlogPost{Id: utils.UUIDFromPg(r.ID), Slug: r.Slug, Title: r.Title, PublishedAt: tsFromPgtz(r.PublishedAt)}}, nil
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.UpdatePost(ctx, id, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &cmsv1.UpsertPostResponse{Post: &cmsv1.BlogPost{Id: utils.UUIDFromPg(r.ID), Slug: r.Slug, Title: r.Title, PublishedAt: tsFromPgtz(r.PublishedAt)}}, nil
}

func (s *CmsServer) DeletePost(ctx context.Context, req *cmsv1.DeletePostRequest) (*cmsv1.DeletePostResponse, error) {
	if err := s.requireSuper(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.DeletePost(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &cmsv1.DeletePostResponse{}, nil
}

func (s *CmsServer) ListFaqs(ctx context.Context, req *cmsv1.ListFaqsRequest) (*cmsv1.ListFaqsResponse, error) {
	out := &cmsv1.ListFaqsResponse{}
	if req.GetAdmin() {
		if err := s.requireSuper(ctx); err != nil {
			return nil, err
		}
		rows, err := s.svc.AdminListFaqs(ctx)
		if err != nil {
			return nil, toStatus(err)
		}
		for _, f := range rows {
			out.Faqs = append(out.Faqs, &cmsv1.Faq{
				Id: utils.UUIDFromPg(f.ID), Category: f.Category, Question: f.Question, AnswerHtml: f.AnswerHtml,
				ShowOnHome: f.ShowOnHome, IsActive: f.IsActive, DisplayOrder: f.DisplayOrder,
			})
		}
		return out, nil
	}
	if req.GetHomeOnly() {
		hs, err := s.svc.ListHomepageFaqs(ctx)
		if err != nil {
			return nil, toStatus(err)
		}
		for _, f := range hs {
			out.Faqs = append(out.Faqs, &cmsv1.Faq{
				Id: utils.UUIDFromPg(f.ID), Category: f.Category, Question: f.Question, AnswerHtml: f.AnswerHtml,
				ShowOnHome: f.ShowOnHome, IsActive: true, DisplayOrder: f.DisplayOrder,
			})
		}
		return out, nil
	}
	fs, err := s.svc.ListFaqs(ctx, req.GetCategory())
	if err != nil {
		return nil, toStatus(err)
	}
	for _, f := range fs {
		out.Faqs = append(out.Faqs, &cmsv1.Faq{
			Id: utils.UUIDFromPg(f.ID), Category: f.Category, Question: f.Question, AnswerHtml: f.AnswerHtml,
			ShowOnHome: f.ShowOnHome, IsActive: true, DisplayOrder: f.DisplayOrder,
		})
	}
	return out, nil
}

func (s *CmsServer) UpsertFaq(ctx context.Context, req *cmsv1.UpsertFaqRequest) (*cmsv1.UpsertFaqResponse, error) {
	if err := s.requireSuper(ctx); err != nil {
		return nil, err
	}
	in := cms.FaqInput{
		Category: req.GetCategory(), Question: req.GetQuestion(), AnswerHTML: req.GetAnswerHtml(),
		ShowOnHome: req.GetShowOnHome(), DisplayOrder: req.GetDisplayOrder(),
	}
	if req.IsActive != nil {
		v := req.GetIsActive()
		in.IsActive = &v
	}
	if req.GetId() == "" {
		r, err := s.svc.CreateFaq(ctx, in)
		if err != nil {
			return nil, toStatus(err)
		}
		return &cmsv1.UpsertFaqResponse{Faq: faqMsg(utils.UUIDFromPg(r.ID), r.Category, r.Question, r.AnswerHtml, r.ShowOnHome, r.IsActive, r.DisplayOrder)}, nil
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	r, err := s.svc.UpdateFaq(ctx, id, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &cmsv1.UpsertFaqResponse{Faq: faqMsg(utils.UUIDFromPg(r.ID), r.Category, r.Question, r.AnswerHtml, r.ShowOnHome, r.IsActive, r.DisplayOrder)}, nil
}

func faqMsg(id, cat, q, a string, home, active bool, order int32) *cmsv1.Faq {
	return &cmsv1.Faq{Id: id, Category: cat, Question: q, AnswerHtml: a, ShowOnHome: home, IsActive: active, DisplayOrder: order}
}

func (s *CmsServer) DeleteFaq(ctx context.Context, req *cmsv1.DeleteFaqRequest) (*cmsv1.DeleteFaqResponse, error) {
	if err := s.requireSuper(ctx); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.DeleteFaq(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &cmsv1.DeleteFaqResponse{}, nil
}

func (s *CmsServer) GetCmsPage(ctx context.Context, req *cmsv1.GetCmsPageRequest) (*cmsv1.GetCmsPageResponse, error) {
	p, err := s.svc.GetCmsPage(ctx, req.GetSlug())
	if err != nil {
		return nil, toStatus(err)
	}
	return &cmsv1.GetCmsPageResponse{Page: &cmsv1.CmsPage{
		Slug: p.Slug, Title: p.Title, BodyHtml: p.BodyHtml, SeoTitle: utils.TextFromPg(p.SeoTitle),
		SeoDesc: utils.TextFromPg(p.SeoDesc), IsPublished: true,
	}}, nil
}

func (s *CmsServer) UpsertCmsPage(ctx context.Context, req *cmsv1.UpsertCmsPageRequest) (*cmsv1.UpsertCmsPageResponse, error) {
	if err := s.requireSuper(ctx); err != nil {
		return nil, err
	}
	in := cms.CmsPageInput{
		Slug: req.GetSlug(), Title: req.GetTitle(), BodyJSON: req.GetBodyJson(), BodyHTML: req.GetBodyHtml(),
		SeoTitle: req.GetSeoTitle(), SeoDesc: req.GetSeoDesc(),
	}
	if req.IsPublished != nil {
		v := req.GetIsPublished()
		in.IsPublished = &v
	}
	r, err := s.svc.UpsertCmsPage(ctx, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &cmsv1.UpsertCmsPageResponse{Page: &cmsv1.CmsPage{
		Slug: r.Slug, Title: r.Title, BodyHtml: r.BodyHtml, SeoTitle: utils.TextFromPg(r.SeoTitle),
		SeoDesc: utils.TextFromPg(r.SeoDesc), IsPublished: r.IsPublished,
	}}, nil
}
