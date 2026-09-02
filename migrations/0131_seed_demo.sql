-- 0131_seed_demo.sql
-- Dev / staging only. scripts/migrate.sh SKIPS this file when
-- APP_ENV=production. Seeds one demo tenant + an admin membership so the
-- app has a tenant to log into locally.

DO $$
DECLARE
    demo_tenant uuid := '00000000-0000-0000-0000-0000000000d0';
    demo_admin  uuid := '00000000-0000-0000-0000-0000000000d1';
BEGIN
    INSERT INTO tenants (id, org_code, name, slug, status, plan, place_of_supply, timezone)
    VALUES (demo_tenant, 'DEMO', 'Demo Coaching', 'demo', 'active', 'growth', '09', 'Asia/Kolkata')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO tenant_settings (tenant_id, features)
    VALUES (demo_tenant, '{"live": true, "store": true, "tests": true, "ai_doubts": true, "downloads": true}'::jsonb)
    ON CONFLICT (tenant_id) DO NOTHING;

    INSERT INTO users (id, phone, full_name, status)
    VALUES (demo_admin, '+919000000001', 'Demo Admin', 'active')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO auth_identities (user_id, provider, provider_uid, verified_at)
    VALUES (demo_admin, 'phone', '+919000000001', now())
    ON CONFLICT (provider, provider_uid) DO NOTHING;

    INSERT INTO tenant_users (tenant_id, user_id, role, status)
    VALUES (demo_tenant, demo_admin, 'owner', 'active')
    ON CONFLICT (tenant_id, user_id) DO NOTHING;

    UPDATE tenants SET owner_user_id = demo_admin WHERE id = demo_tenant AND owner_user_id IS NULL;
END $$;
