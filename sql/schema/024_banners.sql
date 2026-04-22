CREATE TABLE IF NOT EXISTS banners (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255) NOT NULL,
    subtitle TEXT,
    image_url TEXT NOT NULL,
    background_color VARCHAR(16),
    link_type VARCHAR(20),          -- course | lecture | test | url | none
    link_id UUID,                   -- resource id if link_type is course/lecture/test
    link_url TEXT,                  -- external URL if link_type = 'url'
    display_order INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    starts_at TIMESTAMP,
    ends_at TIMESTAMP,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_banners_active ON banners(is_active);
CREATE INDEX IF NOT EXISTS idx_banners_window ON banners(starts_at, ends_at);
CREATE INDEX IF NOT EXISTS idx_banners_order ON banners(display_order);
