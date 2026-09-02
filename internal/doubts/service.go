package doubts

import (
	"context"
	"errors"
	"fmt"

	"live-platform/internal/aiclient"
	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// schema-v2: doubts are tenant-scoped, keyed on lesson_id (was lecture_id),
// with attachment_url (was voice_url) and a doubt_status enum
// (open|answered|resolved|closed).
type Service struct {
	q      *db.Queries
	claude *aiclient.Claude
}

func NewService(pool *pgxpool.Pool, claude *aiclient.Claude) *Service {
	return &Service{q: db.New(pool), claude: claude}
}

type AskDoubtRequest struct {
	LessonID      *uuid.UUID `json:"lesson_id"`
	LectureID     *uuid.UUID `json:"lecture_id"` // legacy alias
	ChapterID     *uuid.UUID `json:"chapter_id"`
	TopicID       *uuid.UUID `json:"topic_id"`
	Question      string     `json:"question"`
	QuestionText  string     `json:"question_text"`
	InputType     string     `json:"input_type"`
	AttachmentURL string     `json:"attachment_url"`
	VoiceURL      string     `json:"voice_url"` // legacy alias
	Language      string     `json:"language"`
	UseAI         bool       `json:"use_ai"`
}

func (r AskDoubtRequest) question() string {
	if r.Question != "" {
		return r.Question
	}
	return r.QuestionText
}
func (r AskDoubtRequest) lesson() *uuid.UUID {
	if r.LessonID != nil {
		return r.LessonID
	}
	return r.LectureID
}
func (r AskDoubtRequest) attachment() string {
	if r.AttachmentURL != "" {
		return r.AttachmentURL
	}
	return r.VoiceURL
}

func (s *Service) Ask(ctx context.Context, tenantID, userID uuid.UUID, req AskDoubtRequest) (db.CreateDoubtRow, *db.AddDoubtAnswerRow, error) {
	if req.InputType == "" {
		req.InputType = "text"
	}
	if req.Language == "" {
		req.Language = "en"
	}
	d, err := s.q.CreateDoubt(ctx, db.CreateDoubtParams{
		TenantID:      utils.UUIDToPg(tenantID),
		UserID:        utils.UUIDToPg(userID),
		LessonID:      utils.UUIDPtrToPg(req.lesson()),
		ChapterID:     utils.UUIDPtrToPg(req.ChapterID),
		TopicID:       utils.UUIDPtrToPg(req.TopicID),
		QuestionText:  req.question(),
		InputType:     utils.TextToPg(req.InputType),
		AttachmentUrl: utils.TextToPg(req.attachment()),
		Language:      utils.TextToPg(req.Language),
	})
	if err != nil {
		return db.CreateDoubtRow{}, nil, err
	}

	if !req.UseAI || s.claude == nil {
		return d, nil, nil
	}

	answer, err := s.aiAnswer(ctx, req.question(), req.Language)
	if err != nil {
		return d, nil, err
	}
	ans, err := s.q.AddDoubtAnswer(ctx, db.AddDoubtAnswerParams{
		TenantID:   utils.UUIDToPg(tenantID),
		DoubtID:    d.ID,
		AnswerText: answer,
		AnswerType: utils.TextToPg("ai"),
		ModelName:  utils.TextToPg(s.claude.Model()),
	})
	if err != nil {
		return d, nil, err
	}
	_ = s.q.SetDoubtStatus(ctx, db.SetDoubtStatusParams{ID: d.ID, Status: db.DoubtStatus("answered")})
	return d, &ans, nil
}

func (s *Service) aiAnswer(ctx context.Context, question, language string) (string, error) {
	if s.claude == nil {
		return "", errors.New("AI client not configured")
	}
	system := `You are a patient, expert tutor helping a student. Explain step-by-step using clear language, break concepts into small parts, and include worked examples where useful. If the question relates to math, show the derivation. If it relates to science, explain the underlying intuition and give units. End with a 1-line summary. Keep the tone encouraging.`
	if language != "" && language != "en" {
		system += fmt.Sprintf("\n\nReply primarily in: %s (but use English terms where they're standard).", language)
	}
	return s.claude.Ask(ctx, system, question)
}

type InstructorAnswerRequest struct {
	DoubtID    uuid.UUID `json:"doubt_id" validate:"required"`
	AnswerText string    `json:"answer_text"`
	Content    string    `json:"content"` // legacy alias
}

func (r InstructorAnswerRequest) text() string {
	if r.AnswerText != "" {
		return r.AnswerText
	}
	return r.Content
}

func (s *Service) AnswerAsInstructor(ctx context.Context, tenantID, instructorID uuid.UUID, req InstructorAnswerRequest) (db.AddDoubtAnswerRow, error) {
	ans, err := s.q.AddDoubtAnswer(ctx, db.AddDoubtAnswerParams{
		TenantID:   utils.UUIDToPg(tenantID),
		DoubtID:    utils.UUIDToPg(req.DoubtID),
		AnswerText: req.text(),
		AnswerType: utils.TextToPg("instructor"),
		AnsweredBy: utils.UUIDToPg(instructorID),
	})
	if err != nil {
		return db.AddDoubtAnswerRow{}, err
	}
	_ = s.q.SetDoubtStatus(ctx, db.SetDoubtStatusParams{
		ID: utils.UUIDToPg(req.DoubtID), Status: db.DoubtStatus("answered"),
	})
	return ans, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.GetDoubtRow, []db.ListDoubtAnswersRow, error) {
	d, err := s.q.GetDoubt(ctx, utils.UUIDToPg(id))
	if err != nil {
		return db.GetDoubtRow{}, nil, err
	}
	ans, err := s.q.ListDoubtAnswers(ctx, utils.UUIDToPg(id))
	if err != nil {
		return d, nil, err
	}
	return d, ans, nil
}

func (s *Service) ListMine(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]db.ListDoubtsForUserRow, error) {
	return s.q.ListDoubtsForUser(ctx, db.ListDoubtsForUserParams{
		TenantID: utils.UUIDToPg(tenantID), UserID: utils.UUIDToPg(userID),
		Limit: limit, Offset: offset,
	})
}

func (s *Service) ListPending(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]db.ListPendingDoubtsRow, error) {
	return s.q.ListPendingDoubts(ctx, db.ListPendingDoubtsParams{
		TenantID: utils.UUIDToPg(tenantID), Limit: limit, Offset: offset,
	})
}

func (s *Service) Accept(ctx context.Context, answerID uuid.UUID) error {
	return s.q.AcceptDoubtAnswer(ctx, utils.UUIDToPg(answerID))
}

func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status string) error {
	return s.q.SetDoubtStatus(ctx, db.SetDoubtStatusParams{
		ID: utils.UUIDToPg(id), Status: db.DoubtStatus(status),
	})
}
