-- +migrate Up
CREATE TABLE IF NOT EXISTS tests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,
    subject_id UUID REFERENCES subjects(id) ON DELETE CASCADE,
    chapter_id UUID REFERENCES chapters(id) ON DELETE CASCADE,
    topic_id UUID REFERENCES topics(id) ON DELETE CASCADE,
    exam_category_id UUID REFERENCES exam_categories(id) ON DELETE SET NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    test_type VARCHAR(20) NOT NULL DEFAULT 'chapter',
    exam_year INTEGER,
    duration_minutes INTEGER DEFAULT 0,
    total_marks NUMERIC(8,2) DEFAULT 0,
    passing_marks NUMERIC(8,2) DEFAULT 0,
    negative_marking BOOLEAN DEFAULT FALSE,
    shuffle_questions BOOLEAN DEFAULT FALSE,
    language VARCHAR(20) DEFAULT 'en',
    is_free BOOLEAN DEFAULT FALSE,
    is_published BOOLEAN DEFAULT FALSE,
    scheduled_at TIMESTAMP,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tests_course ON tests(course_id);
CREATE INDEX IF NOT EXISTS idx_tests_subject ON tests(subject_id);
CREATE INDEX IF NOT EXISTS idx_tests_chapter ON tests(chapter_id);
CREATE INDEX IF NOT EXISTS idx_tests_topic ON tests(topic_id);
CREATE INDEX IF NOT EXISTS idx_tests_type ON tests(test_type);
CREATE INDEX IF NOT EXISTS idx_tests_year ON tests(exam_year);
CREATE INDEX IF NOT EXISTS idx_tests_published ON tests(is_published);

CREATE TABLE IF NOT EXISTS questions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    test_id UUID NOT NULL REFERENCES tests(id) ON DELETE CASCADE,
    topic_id UUID REFERENCES topics(id) ON DELETE SET NULL,
    question_text TEXT NOT NULL,
    question_type VARCHAR(20) DEFAULT 'mcq',
    image_url TEXT,
    marks NUMERIC(6,2) DEFAULT 1,
    negative_marks NUMERIC(6,2) DEFAULT 0,
    difficulty VARCHAR(20) DEFAULT 'medium',
    explanation TEXT,
    correct_numerical_answer NUMERIC(12,4),
    display_order INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_questions_test ON questions(test_id);
CREATE INDEX IF NOT EXISTS idx_questions_topic ON questions(topic_id);
CREATE INDEX IF NOT EXISTS idx_questions_difficulty ON questions(difficulty);

CREATE TABLE IF NOT EXISTS question_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id UUID NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
    option_text TEXT NOT NULL,
    image_url TEXT,
    is_correct BOOLEAN DEFAULT FALSE,
    display_order INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_options_question ON question_options(question_id);
