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

type Service struct {
	q      *db.Queries
	claude *aiclient.Claude
}

func NewService(pool *pgxpool.Pool, claude *aiclient.Claude) *Service {
	return &Service{q: db.New(pool), claude: claude}
}

type AskDoubtRequest struct {
	LectureID    *uuid.UUID `json:"lecture_id"`
	ChapterID    *uuid.UUID `json:"chapter_id"`
	TopicID      *uuid.UUID `json:"topic_id"`
	QuestionText string     `json:"question_text" validate:"required,min=3"`
	InputType    string     `json:"input_type"` // "text" | "voice"
	VoiceURL     string     `json:"voice_url"`
	Language     string     `json:"language"`
	UseAI        bool       `json:"use_ai"`
}

// Ask creates a doubt record. If UseAI is true and Claude is configured,
// an AI answer is generated synchronously and persisted.
func (s *Service) Ask(ctx context.Context, userID uuid.UUID, req AskDoubtRequest) (*db.Doubt, *db.DoubtAnswer, error) {
	if req.InputType == "" {
		req.InputType = "text"
	}
	if req.Language == "" {
		req.Language = "en"
	}
	d, err := s.q.CreateDoubt(ctx, db.CreateDoubtParams{
		UserID:       utils.UUIDToPg(userID),
		LectureID:    utils.UUIDPtrToPg(req.LectureID),
		ChapterID:    utils.UUIDPtrToPg(req.ChapterID),
		TopicID:      utils.UUIDPtrToPg(req.TopicID),
		QuestionText: req.QuestionText,
		InputType:    utils.TextToPg(req.InputType),
		VoiceURL:     utils.TextToPg(req.VoiceURL),
		Language:     utils.TextToPg(req.Language),
	})
	if err != nil {
		return nil, nil, err
	}

	if !req.UseAI || s.claude == nil {
		return &d, nil, nil
	}

	answer, err := s.aiAnswer(ctx, req.QuestionText, req.Language)
	if err != nil {
		// Don't fail the whole request if AI is unavailable; leave as pending.
		return &d, nil, err
	}

	ans, err := s.q.CreateDoubtAnswer(ctx, db.CreateDoubtAnswerParams{
		DoubtID:    d.ID,
		AnswerText: answer,
		AnswerType: utils.TextToPg("ai"),
		ModelName:  utils.TextToPg(s.claude.Model()),
	})
	if err != nil {
		return &d, nil, err
	}
	_ = s.q.UpdateDoubtStatus(ctx, db.UpdateDoubtStatusParams{
		ID:     d.ID,
		Status: utils.TextToPg("answered"),
	})
	return &d, &ans, nil
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
	AnswerText string    `json:"answer_text" validate:"required,min=3"`
}

func (s *Service) AnswerAsInstructor(ctx context.Context, instructorID uuid.UUID, req InstructorAnswerRequest) (*db.DoubtAnswer, error) {
	ans, err := s.q.CreateDoubtAnswer(ctx, db.CreateDoubtAnswerParams{
		DoubtID:    utils.UUIDToPg(req.DoubtID),
		AnswerText: req.AnswerText,
		AnswerType: utils.TextToPg("instructor"),
		AnsweredBy: utils.UUIDToPg(instructorID),
	})
	if err != nil {
		return nil, err
	}
	_ = s.q.UpdateDoubtStatus(ctx, db.UpdateDoubtStatusParams{
		ID:     utils.UUIDToPg(req.DoubtID),
		Status: utils.TextToPg("answered"),
	})
	return &ans, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*db.Doubt, []db.DoubtAnswer, error) {
	d, err := s.q.GetDoubtByID(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, nil, err
	}
	ans, err := s.q.ListAnswersByDoubt(ctx, utils.UUIDToPg(id))
	if err != nil {
		return nil, nil, err
	}
	return &d, ans, nil
}

func (s *Service) ListMine(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]db.Doubt, error) {
	return s.q.ListUserDoubts(ctx, db.ListUserDoubtsParams{
		UserID: utils.UUIDToPg(userID),
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Service) ListByLecture(ctx context.Context, lectureID uuid.UUID, limit, offset int32) ([]db.Doubt, error) {
	return s.q.ListDoubtsByLecture(ctx, db.ListDoubtsByLectureParams{
		LectureID: utils.UUIDToPg(lectureID),
		Limit:     limit,
		Offset:    offset,
	})
}

func (s *Service) ListPending(ctx context.Context, limit, offset int32) ([]db.Doubt, error) {
	return s.q.ListPendingDoubts(ctx, db.ListPendingDoubtsParams{
		Limit: limit, Offset: offset,
	})
}

func (s *Service) Accept(ctx context.Context, answerID uuid.UUID) error {
	return s.q.AcceptDoubtAnswer(ctx, utils.UUIDToPg(answerID))
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteDoubt(ctx, utils.UUIDToPg(id))
}
