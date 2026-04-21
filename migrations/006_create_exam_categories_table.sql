-- +migrate Up
CREATE TABLE IF NOT EXISTS exam_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    icon_url TEXT,
    display_order INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_exam_categories_slug ON exam_categories(slug);
CREATE INDEX IF NOT EXISTS idx_exam_categories_active ON exam_categories(is_active);

-- Seed with common Indian exam categories
INSERT INTO exam_categories (name, slug, description, display_order) VALUES
    ('School (Class 6-12)', 'school', 'School curriculum classes 6 to 12', 1),
    ('JEE', 'jee', 'Joint Entrance Examination for engineering', 2),
    ('NEET', 'neet', 'National Eligibility cum Entrance Test for medical', 3),
    ('UPSC', 'upsc', 'Civil Services Examination', 4),
    ('SSC', 'ssc', 'Staff Selection Commission exams', 5),
    ('GATE', 'gate', 'Graduate Aptitude Test in Engineering', 6),
    ('CA', 'ca', 'Chartered Accountant', 7),
    ('MBA', 'mba', 'MBA entrance exams (CAT, XAT, etc.)', 8),
    ('Law', 'law', 'Law entrance exams (CLAT, AILET)', 9),
    ('Olympiads', 'olympiads', 'Science and Math Olympiads', 10)
ON CONFLICT (slug) DO NOTHING;
