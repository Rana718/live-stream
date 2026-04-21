-- Full-text search support across core content entities.
ALTER TABLE courses ADD COLUMN IF NOT EXISTS search_vector tsvector;
ALTER TABLE topics ADD COLUMN IF NOT EXISTS search_vector tsvector;
ALTER TABLE chapters ADD COLUMN IF NOT EXISTS search_vector tsvector;

CREATE INDEX IF NOT EXISTS idx_courses_search ON courses USING gin(search_vector);
CREATE INDEX IF NOT EXISTS idx_topics_search ON topics USING gin(search_vector);
CREATE INDEX IF NOT EXISTS idx_chapters_search ON chapters USING gin(search_vector);
