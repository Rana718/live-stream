CREATE TABLE IF NOT EXISTS lectures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic_id UUID REFERENCES topics(id) ON DELETE CASCADE,
    chapter_id UUID REFERENCES chapters(id) ON DELETE CASCADE,
    subject_id UUID REFERENCES subjects(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    lecture_type VARCHAR(20) DEFAULT 'recorded',
    instructor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    stream_id UUID REFERENCES streams(id) ON DELETE SET NULL,
    recording_id UUID REFERENCES recordings(id) ON DELETE SET NULL,
    thumbnail_url TEXT,
    scheduled_at TIMESTAMP,
    duration_seconds INTEGER DEFAULT 0,
    language VARCHAR(20) DEFAULT 'en',
    is_free BOOLEAN DEFAULT FALSE,
    is_published BOOLEAN DEFAULT FALSE,
    display_order INTEGER DEFAULT 0,
    view_count INTEGER DEFAULT 0,
    search_vector tsvector,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_lectures_topic ON lectures(topic_id);
CREATE INDEX IF NOT EXISTS idx_lectures_chapter ON lectures(chapter_id);
CREATE INDEX IF NOT EXISTS idx_lectures_subject ON lectures(subject_id);
CREATE INDEX IF NOT EXISTS idx_lectures_instructor ON lectures(instructor_id);
CREATE INDEX IF NOT EXISTS idx_lectures_type ON lectures(lecture_type);
CREATE INDEX IF NOT EXISTS idx_lectures_published ON lectures(is_published);
CREATE INDEX IF NOT EXISTS idx_lectures_language ON lectures(language);
CREATE INDEX IF NOT EXISTS idx_lectures_search ON lectures USING gin(search_vector);

CREATE TABLE IF NOT EXISTS lecture_views (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lecture_id UUID NOT NULL REFERENCES lectures(id) ON DELETE CASCADE,
    watched_seconds INTEGER DEFAULT 0,
    completed BOOLEAN DEFAULT FALSE,
    last_watched_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, lecture_id)
);

CREATE INDEX IF NOT EXISTS idx_lecture_views_user ON lecture_views(user_id);
CREATE INDEX IF NOT EXISTS idx_lecture_views_lecture ON lecture_views(lecture_id);
