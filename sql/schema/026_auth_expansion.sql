-- Auth expansion: add mobile + Google to users so the same row can be
-- reached via any login method (email/password, mobile OTP, Google sign-in).
-- `auth_method` records how the account was first created — useful for
-- analytics and for deciding which re-auth paths to offer on settings.
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone_number VARCHAR(20);
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone_verified BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS google_sub VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_method VARCHAR(20) DEFAULT 'email';

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone_number ON users(phone_number)
    WHERE phone_number IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_google_sub ON users(google_sub)
    WHERE google_sub IS NOT NULL;

-- sms_codes holds pending OTP codes. Rows are consumed on verify and expire
-- after a few minutes. `attempts` tracks failed entries so we can lock out
-- brute-force attackers.
CREATE TABLE IF NOT EXISTS sms_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_number VARCHAR(20) NOT NULL,
    code_hash VARCHAR(100) NOT NULL,
    attempts INTEGER DEFAULT 0,
    consumed BOOLEAN DEFAULT FALSE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sms_codes_phone ON sms_codes(phone_number);
CREATE INDEX IF NOT EXISTS idx_sms_codes_expires ON sms_codes(expires_at);
