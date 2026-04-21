-- +migrate Up
ALTER TABLE courses ADD COLUMN IF NOT EXISTS search_vector tsvector;
ALTER TABLE topics ADD COLUMN IF NOT EXISTS search_vector tsvector;
ALTER TABLE chapters ADD COLUMN IF NOT EXISTS search_vector tsvector;

CREATE INDEX IF NOT EXISTS idx_courses_search ON courses USING gin(search_vector);
CREATE INDEX IF NOT EXISTS idx_topics_search ON topics USING gin(search_vector);
CREATE INDEX IF NOT EXISTS idx_chapters_search ON chapters USING gin(search_vector);

-- Seed course search_vector on inserts / updates.
CREATE OR REPLACE FUNCTION courses_search_trigger() RETURNS trigger AS $$
BEGIN
  NEW.search_vector :=
    setweight(to_tsvector('simple', coalesce(NEW.title,'')), 'A') ||
    setweight(to_tsvector('simple', coalesce(NEW.description,'')), 'B');
  RETURN NEW;
END
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_courses_search ON courses;
CREATE TRIGGER trg_courses_search BEFORE INSERT OR UPDATE ON courses
  FOR EACH ROW EXECUTE FUNCTION courses_search_trigger();

CREATE OR REPLACE FUNCTION lectures_search_trigger() RETURNS trigger AS $$
BEGIN
  NEW.search_vector :=
    setweight(to_tsvector('simple', coalesce(NEW.title,'')), 'A') ||
    setweight(to_tsvector('simple', coalesce(NEW.description,'')), 'B');
  RETURN NEW;
END
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_lectures_search ON lectures;
CREATE TRIGGER trg_lectures_search BEFORE INSERT OR UPDATE ON lectures
  FOR EACH ROW EXECUTE FUNCTION lectures_search_trigger();
