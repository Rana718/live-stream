CREATE TABLE IF NOT EXISTS recordings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stream_id UUID NOT NULL REFERENCES streams(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    file_size BIGINT,
    duration INTEGER,
    status VARCHAR(50) DEFAULT 'processing',
    thumbnail_url TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_recordings_stream ON recordings(stream_id);
CREATE INDEX idx_recordings_status ON recordings(status);
