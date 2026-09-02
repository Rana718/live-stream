-- +migrate Up
CREATE TABLE IF NOT EXISTS doubts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lecture_id UUID REFERENCES lectures(id) ON DELETE SET NULL,
    chapter_id UUID REFERENCES chapters(id) ON DELETE SET NULL,
    topic_id UUID REFERENCES topics(id) ON DELETE SET NULL,
    question_text TEXT NOT NULL,
    input_type VARCHAR(20) DEFAULT 'text',
    voice_url TEXT,
    status VARCHAR(20) DEFAULT 'pending',
    language VARCHAR(20) DEFAULT 'en',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_doubts_user ON doubts(user_id);
CREATE INDEX IF NOT EXISTS idx_doubts_lecture ON doubts(lecture_id);
CREATE INDEX IF NOT EXISTS idx_doubts_chapter ON doubts(chapter_id);
CREATE INDEX IF NOT EXISTS idx_doubts_status ON doubts(status);

CREATE TABLE IF NOT EXISTS doubt_answers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doubt_id UUID NOT NULL REFERENCES doubts(id) ON DELETE CASCADE,
    answer_text TEXT NOT NULL,
    answer_type VARCHAR(20) DEFAULT 'ai',
    answered_by UUID REFERENCES users(id) ON DELETE SET NULL,
    is_accepted BOOLEAN DEFAULT FALSE,
    model_name VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_doubt_answers_doubt ON doubt_answers(doubt_id);
CREATE INDEX IF NOT EXISTS idx_doubt_answers_type ON doubt_answers(answer_type);
