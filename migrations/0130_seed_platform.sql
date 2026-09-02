-- 0130_seed_platform.sql
-- Data every environment needs: platform GST rates and a platform
-- super-admin identity. Runs as postgres (BYPASSRLS) so RLS never blocks it.

-- Platform default tax rates (tenant_id NULL). Tenants may override per HSN.
INSERT INTO tax_rates (tenant_id, hsn_sac, name, rate_bps)
SELECT NULL, v.hsn_sac, v.name, v.rate_bps
FROM (VALUES
    ('9992',   'Education services (GST 18%)',      1800),
    ('999293', 'Commercial coaching (GST 18%)',     1800),
    ('998431', 'OIDAR / online content (GST 18%)',  1800),
    ('EXEMPT', 'Exempt supply',                     0)
) AS v(hsn_sac, name, rate_bps)
WHERE NOT EXISTS (
    SELECT 1 FROM tax_rates t WHERE t.tenant_id IS NULL AND t.hsn_sac = v.hsn_sac
);

-- Platform super-admin identity + a login path. v2 auth is phone-OTP / Google
-- only (no password), and a login always resolves a tenant from an org code
-- and a tenant_users membership. So the super-admin needs: a phone identity,
-- and a membership in a dedicated PLATFORM tenant that exists only to give the
-- OTP flow something to resolve. issueTokens() promotes the emitted role to
-- 'super_admin' whenever users.is_platform_super_admin is true, regardless of
-- the tenant_users.role stored here.
DO $$
DECLARE
    super_admin uuid := '00000000-0000-0000-0000-0000000000aa';
    platform    uuid := '00000000-0000-0000-0000-00000000000a';
    super_phone text := '+919000000000';
BEGIN
    INSERT INTO users (id, email, phone, full_name, is_platform_super_admin, status)
    VALUES (super_admin, 'platform-admin@example.com', super_phone,
            'Platform Admin', true, 'active')
    ON CONFLICT (id) DO NOTHING;
    -- Row may predate this migration — backfill the phone.
    UPDATE users SET phone = super_phone
    WHERE id = super_admin AND phone IS NULL;

    INSERT INTO auth_identities (user_id, provider, provider_uid, verified_at)
    VALUES (super_admin, 'phone', super_phone, now())
    ON CONFLICT (provider, provider_uid) DO NOTHING;

    INSERT INTO tenants (id, org_code, name, slug, status, plan, timezone)
    VALUES (platform, 'PLATFORM', 'Platform', 'platform', 'active', 'enterprise', 'Asia/Kolkata')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO tenant_settings (tenant_id, features)
    VALUES (platform, '{}'::jsonb)
    ON CONFLICT (tenant_id) DO NOTHING;

    INSERT INTO tenant_users (tenant_id, user_id, role, status)
    VALUES (platform, super_admin, 'owner', 'active')
    ON CONFLICT (tenant_id, user_id) DO NOTHING;

    UPDATE tenants SET owner_user_id = super_admin
    WHERE id = platform AND owner_user_id IS NULL;
END $$;
