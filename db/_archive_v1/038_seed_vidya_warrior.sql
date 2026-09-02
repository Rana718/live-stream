-- 038_seed_vidya_warrior.sql
-- Seeds the demo tenant "Vidya Warrior" (org code RANJAN24) plus a single
-- admin user. Idempotent — re-runs no-op via ON CONFLICT.
--
-- This is safe to keep in the migration chain because it's gated on the
-- org code being absent. Production deploys can either keep it (the
-- tenant becomes a "founder demo" account) or delete it before running.
--
-- The org code RANJAN24 was chosen by the user; the tenant name is what
-- we publicly market as the platform-level brand. In a multi-tenant
-- world a tenant happens to be named the same as the platform — that's
-- fine, the platform-level admin lives under the super_admin role on
-- a *different* tenant entirely.

-- 1. Tenant row.
INSERT INTO tenants (
    id,
    org_code,
    name,
    slug,
    plan,
    status,
    custom_domain,
    theme,
    app_config
)
VALUES (
    'aaaaaaaa-bbbb-cccc-dddd-000000000001',
    'RANJAN24',
    'Vidya Warrior',
    'vidya-warrior',
    'premium',
    'active',
    'vidyawarrior.com',
    -- Theme tokens consumed by the school-web ThemeInjector. Hex values
    -- match Vidya Warrior's brand: deep indigo + warm amber accent.
    '{
        "primary": "#2456db",
        "primaryDark": "#1c44b3",
        "primarySoft": "#eef4ff",
        "accent": "#f59e0b",
        "background": "#ffffff",
        "textPrimary": "#0f172a",
        "textSecondary": "#475569"
    }'::jsonb,
    -- App config consumed by the Codemagic per-tenant build pipeline.
    '{
        "package_id": "com.vidyawarrior.student",
        "app_name": "Vidya Warrior",
        "support_email": "support@vidyawarrior.com"
    }'::jsonb
)
ON CONFLICT (org_code) DO UPDATE
    SET name          = EXCLUDED.name,
        slug          = EXCLUDED.slug,
        custom_domain = EXCLUDED.custom_domain,
        theme         = EXCLUDED.theme,
        app_config    = EXCLUDED.app_config;

-- 2. Feature flags. Premium tier unlocks the lot.
INSERT INTO tenant_features (tenant_id, features)
VALUES (
    'aaaaaaaa-bbbb-cccc-dddd-000000000001',
    '{
        "live": true,
        "store": true,
        "tests": true,
        "ai_doubts": true,
        "downloads": true,
        "watermark": true,
        "multi_branch": false
    }'::jsonb
)
ON CONFLICT (tenant_id) DO NOTHING;

-- 3. First admin. Phone-OTP login is the only path on the new platform,
--    so we don't seed a password — the user signs in by entering this
--    phone number on /login (after the org code) and getting an OTP.
--    During local dev the OTP is logged to stdout instead of sent over
--    SMS (see internal/otp).
--
--    The tenant_id is NOT NULL on users since migration 028, so we
--    populate it directly. email is left NULL (phone-primary identity).
INSERT INTO users (
    id,
    tenant_id,
    full_name,
    phone_number,
    phone_verified,
    role,
    is_active,
    auth_method
)
VALUES (
    'aaaaaaaa-bbbb-cccc-dddd-000000000a01',
    'aaaaaaaa-bbbb-cccc-dddd-000000000001',
    'Ranjan Sir',
    '+919999900001',
    TRUE,
    'admin',
    TRUE,
    'phone'
)
ON CONFLICT (id) DO UPDATE
    SET full_name      = EXCLUDED.full_name,
        phone_number   = EXCLUDED.phone_number,
        phone_verified = EXCLUDED.phone_verified,
        role           = EXCLUDED.role,
        is_active      = EXCLUDED.is_active,
        auth_method    = EXCLUDED.auth_method;

-- 4. Tenant ↔ user membership. The `users.role` column is the fallback;
--    `tenant_users` is the source of truth when a user spans multiple
--    tenants. For this seed they coincide.
INSERT INTO tenant_users (tenant_id, user_id, role, status)
VALUES (
    'aaaaaaaa-bbbb-cccc-dddd-000000000001',
    'aaaaaaaa-bbbb-cccc-dddd-000000000a01',
    'admin',
    'active'
)
ON CONFLICT (tenant_id, user_id, role) DO NOTHING;

-- 5. Mark the admin as the tenant owner. Useful for billing emails and
--    "primary contact" UI affordances.
UPDATE tenants
   SET owner_user_id = 'aaaaaaaa-bbbb-cccc-dddd-000000000a01'
 WHERE id = 'aaaaaaaa-bbbb-cccc-dddd-000000000001'
   AND owner_user_id IS NULL;

-- 6. A demo student so the admin has someone to look at on first login.
--    Same idempotency pattern as the admin row.
INSERT INTO users (
    id,
    tenant_id,
    full_name,
    phone_number,
    phone_verified,
    role,
    is_active,
    auth_method
)
VALUES (
    'aaaaaaaa-bbbb-cccc-dddd-000000000a02',
    'aaaaaaaa-bbbb-cccc-dddd-000000000001',
    'Demo Student',
    '+919999900002',
    TRUE,
    'student',
    TRUE,
    'phone'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO tenant_users (tenant_id, user_id, role, status)
VALUES (
    'aaaaaaaa-bbbb-cccc-dddd-000000000001',
    'aaaaaaaa-bbbb-cccc-dddd-000000000a02',
    'student',
    'active'
)
ON CONFLICT (tenant_id, user_id, role) DO NOTHING;
