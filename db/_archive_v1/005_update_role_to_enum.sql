-- +migrate Up
CREATE TYPE user_role AS ENUM ('student', 'admin', 'instructor');

ALTER TABLE users 
    ALTER COLUMN role DROP DEFAULT,
    ALTER COLUMN role TYPE user_role USING role::user_role,
    ALTER COLUMN role SET DEFAULT 'student';

-- Stream status enum
CREATE TYPE stream_status AS ENUM ('scheduled', 'live', 'ended');

ALTER TABLE streams 
    ALTER COLUMN status DROP DEFAULT,
    ALTER COLUMN status TYPE stream_status USING status::stream_status,
    ALTER COLUMN status SET DEFAULT 'scheduled';

-- Recording status enum
CREATE TYPE recording_status AS ENUM ('processing', 'ready', 'failed');

ALTER TABLE recordings 
    ALTER COLUMN status DROP DEFAULT,
    ALTER COLUMN status TYPE recording_status USING status::recording_status,
    ALTER COLUMN status SET DEFAULT 'processing';

-- +migrate Down
-- ALTER TABLE recordings 
--     ALTER COLUMN status DROP DEFAULT,
--     ALTER COLUMN status TYPE VARCHAR(50) USING status::text,
--     ALTER COLUMN status SET DEFAULT 'processing';

-- DROP TYPE recording_status;

-- ALTER TABLE streams 
--     ALTER COLUMN status DROP DEFAULT,
--     ALTER COLUMN status TYPE VARCHAR(50) USING status::text,
--     ALTER COLUMN status SET DEFAULT 'scheduled';

-- DROP TYPE stream_status;

-- ALTER TABLE users 
--     ALTER COLUMN role DROP DEFAULT,
--     ALTER COLUMN role TYPE VARCHAR(50) USING role::text,
--     ALTER COLUMN role SET DEFAULT 'student';

-- DROP TYPE user_role;