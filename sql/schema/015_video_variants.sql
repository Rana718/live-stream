CREATE TABLE IF NOT EXISTS video_variants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recording_id UUID REFERENCES recordings(id) ON DELETE CASCADE,
    lecture_id UUID REFERENCES lectures(id) ON DELETE CASCADE,
    quality VARCHAR(10) NOT NULL,
    file_path TEXT NOT NULL,
    file_size BIGINT DEFAULT 0,
    bitrate_kbps INTEGER DEFAULT 0,
    width INTEGER DEFAULT 0,
    height INTEGER DEFAULT 0,
    codec VARCHAR(30) DEFAULT 'h264',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_variants_recording ON video_variants(recording_id);
CREATE INDEX IF NOT EXISTS idx_variants_lecture ON video_variants(lecture_id);
CREATE INDEX IF NOT EXISTS idx_variants_quality ON video_variants(quality);

CREATE TABLE IF NOT EXISTS download_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    resource_type VARCHAR(30) NOT NULL,
    resource_id UUID NOT NULL,
    token VARCHAR(128) UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tokens_user ON download_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_tokens_token ON download_tokens(token);
CREATE INDEX IF NOT EXISTS idx_tokens_expires ON download_tokens(expires_at);
