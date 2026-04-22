-- +migrate Up
CREATE TABLE IF NOT EXISTS banners (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255) NOT NULL,
    subtitle TEXT,
    image_url TEXT NOT NULL,
    background_color VARCHAR(16),
    link_type VARCHAR(20),
    link_id UUID,
    link_url TEXT,
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

-- Seed two starter banners so the home rail isn't empty on a fresh install.
INSERT INTO banners (title, subtitle, image_url, background_color, link_type, display_order) VALUES
    ('Welcome to Aadesh Academy',
     'Live classes, recorded lectures, AI doubt solving — all in one place',
     'https://images.unsplash.com/photo-1503676260728-1c00da094a0b?w=1200',
     '#6C4AD0',
     'none',
     1),
    ('NEET 2026 Crash Course',
     'Limited time offer · 40% off for early registrations',
     'https://images.unsplash.com/photo-1532094349884-543bc11b234d?w=1200',
     '#FFE8DA',
     'none',
     2)
ON CONFLICT DO NOTHING;
