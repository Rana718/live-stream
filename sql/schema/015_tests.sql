CREATE TABLE IF NOT EXISTS tests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,
    subject_id UUID REFERENCES subjects(id) ON DELETE SET NULL,
    chapter_id UUID REFERENCES chapters(id) ON DELETE SET NULL,
    topic_id UUID REFERENCES topics(id) ON DELETE SET NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    test_type VARCHAR(30) NOT NULL, -- 'mock' | 'chapter' | 'dpp' | 'pyq' | 'custom'
    duration_minutes INTEGER DEFAULT 0,
    total_marks NUMERIC(8,2) DEFAULT 0,
    passing_marks NUMERIC(8,2) DEFAULT 0,
    negative_marking NUMERIC(4,2) DEFAULT 0,
    is_published BOOLEAN DEFAULT FALSE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tests_course ON tests(course_id);
CREATE INDEX IF NOT EXISTS idx_tests_subject ON tests(subject_id);
CREATE INDEX IF NOT EXISTS idx_tests_chapter ON tests(chapter_id);
CREATE INDEX IF NOT EXISTS idx_tests_type ON tests(test_type);
