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

-- Platform super-admin. Password auth is added out-of-band; this row just
-- needs to exist so someone can be granted access.
INSERT INTO users (id, email, full_name, is_platform_super_admin, status)
VALUES ('00000000-0000-0000-0000-0000000000aa',
        'platform-admin@example.com', 'Platform Admin', true, 'active')
ON CONFLICT (id) DO NOTHING;
