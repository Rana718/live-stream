-- 045_seed_super_admin.sql
-- Seeds a platform-level super_admin user for local dev/testing. Lives on
-- the DEFAULT tenant (org code "DEFAULT") per the note in
-- 033_super_admin_role_seed.sql: super_admin normally operates cross-tenant
-- with no tenant_users row, but attaching one to the default tenant lets
-- the phone+OTP+org-code login flow (same as every other role) work for
-- local testing without a separate platform login path.
INSERT INTO users (id, tenant_id, full_name, phone_number, phone_verified, role, is_active, auth_method)
VALUES (
    'aaaaaaaa-bbbb-cccc-dddd-000000000a04',
    '00000000-0000-0000-0000-000000000001',
    'Platform Owner',
    '+919999900000',
    TRUE,
    'super_admin',
    TRUE,
    'phone'
)
ON CONFLICT (id) DO UPDATE
    SET full_name      = EXCLUDED.full_name,
        phone_number   = EXCLUDED.phone_number,
        phone_verified = EXCLUDED.phone_verified,
        role           = EXCLUDED.role,
        is_active      = EXCLUDED.is_active;
