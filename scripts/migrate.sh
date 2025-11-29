#!/bin/bash

echo "Waiting for PostgreSQL to be ready..."
until docker exec live-platform-postgres pg_isready -U postgres; do
  sleep 2
done

echo "Running database migrations..."
docker exec -i live-platform-postgres psql -U postgres -d live_platform <<-EOSQL
    CREATE TABLE IF NOT EXISTS users (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        email VARCHAR(255) UNIQUE NOT NULL,
        username VARCHAR(100) UNIQUE NOT NULL,
        password_hash VARCHAR(255) NOT NULL,
        full_name VARCHAR(255),
        role VARCHAR(50) DEFAULT 'student',
        is_active BOOLEAN DEFAULT true,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );

    CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
    CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

    CREATE TABLE IF NOT EXISTS streams (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        title VARCHAR(255) NOT NULL,
        description TEXT,
        instructor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        stream_key VARCHAR(255) UNIQUE NOT NULL,
        status VARCHAR(50) DEFAULT 'scheduled',
        scheduled_at TIMESTAMP,
        started_at TIMESTAMP,
        ended_at TIMESTAMP,
        thumbnail_url TEXT,
        viewer_count INTEGER DEFAULT 0,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );

    CREATE INDEX IF NOT EXISTS idx_streams_instructor ON streams(instructor_id);
    CREATE INDEX IF NOT EXISTS idx_streams_status ON streams(status);
    CREATE INDEX IF NOT EXISTS idx_streams_stream_key ON streams(stream_key);

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

    CREATE INDEX IF NOT EXISTS idx_recordings_stream ON recordings(stream_id);
    CREATE INDEX IF NOT EXISTS idx_recordings_status ON recordings(status);

    CREATE TABLE IF NOT EXISTS chat_messages (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        stream_id UUID NOT NULL REFERENCES streams(id) ON DELETE CASCADE,
        user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        message TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );

    CREATE INDEX IF NOT EXISTS idx_chat_stream ON chat_messages(stream_id);
    CREATE INDEX IF NOT EXISTS idx_chat_user ON chat_messages(user_id);
    CREATE INDEX IF NOT EXISTS idx_chat_created ON chat_messages(created_at);
EOSQL

echo "Migrations completed successfully!"
