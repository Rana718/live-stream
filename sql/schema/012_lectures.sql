CREATE TABLE IF NOT EXISTS lectures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chapter_id UUID REFERENCES chapters(id) ON DELETE CASCADE,
    topic_id UUID REFERENCES topics(id) ON DELETE SET NULL,
    stream_id UUID REFERENCES streams(id) ON DELETE SET NULL,
    recording_id UUID REFERENCES recordings(id) ON DELETE SET NULL,
    instructor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    lecture_type VARCHAR(30) NOT NULL DEFAULT 'recorded', -- 'live' | 'recorded'
    language VARCHAR(20) DEFAULT 'en',
    duration_seconds INTEGER DEFAULT 0,
    thumbnail_url TEXT,
    sort_order INTEGER DEFAULT 0,
    scheduled_at TIMESTAMP,
    is_free BOOLEAN DEFAULT FALSE,
    is_published BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_lectures_chapter ON lectures(chapter_id);
CREATE INDEX IF NOT EXISTS idx_lectures_topic ON lectures(topic_id);
CREATE INDEX IF NOT EXISTS idx_lectures_instructor ON lectures(instructor_id);
CREATE INDEX IF NOT EXISTS idx_lectures_type ON lectures(lecture_type);
CREATE INDEX IF NOT EXISTS idx_lectures_published ON lectures(is_published);
