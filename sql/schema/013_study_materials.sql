CREATE TABLE IF NOT EXISTS study_materials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chapter_id UUID REFERENCES chapters(id) ON DELETE CASCADE,
    topic_id UUID REFERENCES topics(id) ON DELETE SET NULL,
    lecture_id UUID REFERENCES lectures(id) ON DELETE SET NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    material_type VARCHAR(50) NOT NULL, -- 'pdf' | 'note' | 'ncert_solution' | 'formula_sheet'
    file_path TEXT NOT NULL,
    file_size BIGINT,
    language VARCHAR(20) DEFAULT 'en',
    is_free BOOLEAN DEFAULT FALSE,
    uploaded_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_materials_chapter ON study_materials(chapter_id);
CREATE INDEX IF NOT EXISTS idx_materials_topic ON study_materials(topic_id);
CREATE INDEX IF NOT EXISTS idx_materials_lecture ON study_materials(lecture_id);
CREATE INDEX IF NOT EXISTS idx_materials_type ON study_materials(material_type);
