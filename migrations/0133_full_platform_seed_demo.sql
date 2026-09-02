-- 0133_full_platform_seed_demo.sql
-- ============================================================================
-- FULL-PLATFORM SHOWCASE SEED  —  dev / staging only.
-- scripts/migrate.sh SKIPS every *_seed_demo.sql when APP_ENV=production.
--
-- Walks the ENTIRE platform lifecycle in order and lands at least one
-- realistic row in every table, so that:
--   • the school-web portal shows real data on every page for every role,
--   • the platform flow is readable straight down this file,
--   • internal/database/seed_completeness_test.go can assert nothing is empty.
--
-- Runs as `postgres` (BYPASSRLS) like 0130/0131, so RLS never blocks the
-- cross-tenant inserts, but every tenant-scoped row still carries an explicit
-- tenant_id.
--
-- Tenant:  org_code VWSTUDY  ("Vidya Warrior Classes")  — separate from the
--          minimal DEMO tenant in 0131.
-- Logins (phone-OTP, dev code 123456):
--   owner/admin  +919000100001   admin        +919000100002
--   instructors  +919000100003 / +919000100004
--   staff        +919000100005   parent       +919000100006
--   students     +919000100010 .. +919000100015
-- Money is bigint paise; GST 18% inclusive.  Paid course ₹4,999 = 499900:
--   taxable 423644 + CGST 38128 + SGST 38128 = 499900.
-- ============================================================================

DO $$
DECLARE
    -- ── anchors (referenced by the completeness test / README) ─────────────
    t_id            uuid := 'f0d00000-0000-0000-0000-000000000001';  -- tenant
    u_owner         uuid := 'f0d00000-0000-0000-0000-0000000000a1';
    u_admin         uuid := 'f0d00000-0000-0000-0000-0000000000a2';
    u_inst_phys     uuid := 'f0d00000-0000-0000-0000-0000000000a3';
    u_inst_chem     uuid := 'f0d00000-0000-0000-0000-0000000000a4';
    u_staff         uuid := 'f0d00000-0000-0000-0000-0000000000a5';
    u_parent        uuid := 'f0d00000-0000-0000-0000-0000000000a6';
    u_stuA          uuid := 'f0d00000-0000-0000-0000-0000000000b0';
    u_stuB          uuid := 'f0d00000-0000-0000-0000-0000000000b1';
    u_stuC          uuid := 'f0d00000-0000-0000-0000-0000000000b2';
    u_stuD          uuid := 'f0d00000-0000-0000-0000-0000000000b3';
    u_stuE          uuid := 'f0d00000-0000-0000-0000-0000000000b4';
    u_stuF          uuid := 'f0d00000-0000-0000-0000-0000000000b5';
    super_admin     uuid := '00000000-0000-0000-0000-0000000000aa';  -- from 0130

    -- taxonomy
    ec_jee uuid; ec_jee_main uuid; ec_neet uuid;
    subj_phys uuid; subj_chem uuid; subj_maths uuid; subj_bio uuid;
    ch_kin uuid; ch_thermo uuid; ch_mole uuid;
    tp_disp uuid; tp_proj uuid; tp_heat uuid; tp_stoich uuid;

    -- catalog
    c_free uuid; c_paid uuid; c_review uuid;
    sec_paid1 uuid; sec_paid2 uuid;
    b_paid uuid;                       -- batch on the paid course
    sched1 uuid;

    -- content
    vid1 uuid; vid2 uuid; doc1 uuid; lnk1 uuid;
    vasset uuid;
    ls_scheduled uuid; ls_ended uuid;
    rec1 uuid;
    les_video uuid; les_doc uuid; les_link uuid; les_live uuid; les_quiz uuid; les_assign uuid;

    -- assessment
    q_mcq1 uuid; q_mcq2 uuid; q_multi uuid; q_num uuid; q_subj uuid; q_match uuid;
    tst_dpp uuid; tst_mock uuid;
    tsec1 uuid;
    att_A uuid;

    -- commerce catalogue
    p_course uuid; p_course_free uuid; p_bundle uuid; p_plan uuid; p_feeplan uuid;
    bndl uuid; plan uuid; feeplan uuid;
    cpn uuid;

    -- purchase flow (student A buys the paid course)
    ord_A uuid; oi_A uuid; pay_A uuid; ent_A uuid; enr_A uuid;
    inv_A uuid; fy text := '2026-27';
    ref_A uuid;                        -- refund on student A's order (partial)
    cn_A uuid;

    -- bundle purchase (student B)
    ord_B uuid; oi_B uuid; pay_B uuid; ent_B1 uuid; ent_B2 uuid; inv_B uuid;

    -- subscription (student C)
    ord_C uuid; pay_C uuid; sub_C uuid; ent_C uuid; inv_C uuid;

    -- fees (student D)
    fa_D uuid; fi_D1 uuid; ord_D uuid; pay_D uuid;

    -- gift (owner gifts free course to student F)
    gift_F uuid; ent_F uuid;

    -- learning / engagement
    thr1 uuid; asg1 uuid;
    wal_A uuid; wtx_A uuid; refev uuid;
    payout1 uuid;
    notif1 uuid;
    mthr1 uuid;
    badge_first uuid; badge_streak uuid;
BEGIN
    IF EXISTS (SELECT 1 FROM tenants WHERE org_code = 'VWSTUDY') THEN
        RAISE NOTICE '0133: full-platform demo already seeded — skipping';
        RETURN;
    END IF;

    -- ========================================================================
    -- PHASE 0 — PLATFORM.  Marketing content the tenant sees before signup,
    --           the plan we bill THEM on, and their white-label mobile app.
    -- ========================================================================
    INSERT INTO tax_rates (tenant_id, hsn_sac, name, rate_bps) VALUES
        (NULL, '999299', 'Other education services (GST 18%)', 1800)
    ON CONFLICT DO NOTHING;

    INSERT INTO blog_posts (slug, title, excerpt, body_html, author_name, tags, minutes_read, published_at, created_by)
    VALUES ('launch-your-coaching-online',
            'How to launch your coaching institute online in a weekend',
            'A step-by-step guide from org code to first paid enrolment.',
            '<p>Sign up, add your courses, set a price, share your link.</p>',
            'Team ClassPlus-grade', ARRAY['guide','onboarding'], 6, now() - interval '20 days', super_admin);

    INSERT INTO faqs (category, question, answer_html, show_on_home, display_order) VALUES
        ('pricing', 'Do you charge per student?', '<p>No — flat monthly plan + payment-gateway fees only.</p>', true, 1),
        ('gst',     'Do you generate GST invoices?', '<p>Yes, a gapless tax invoice per paid order, CGST/SGST or IGST computed from place of supply.</p>', true, 2);

    INSERT INTO cms_pages (slug, title, body_html, is_published) VALUES
        ('terms',   'Terms of Service', '<p>Standard SaaS terms.</p>', true),
        ('privacy', 'Privacy Policy',   '<p>We are a data processor for each tenant.</p>', true);

    -- ========================================================================
    -- PHASE 1 — LEAD → SELF-SERVE TENANT ONBOARDING.
    --           A marketing enquiry converts; the tenant + its settings +
    --           custom domain + owner account are created.
    -- ========================================================================
    INSERT INTO leads (tenant_id, name, phone, email, institute_name, city, students_count, source, utm, status, notes)
    VALUES (NULL, 'Rahul Verma', '+919000100001', 'rahul@vidyawarrior.in',
            'Vidya Warrior Classes', 'Kota', 480, 'website',
            '{"utm_source":"google","utm_campaign":"kota-neet"}'::jsonb, 'converted',
            'Demo booked, signed up same day.');

    INSERT INTO users (id, phone, email, full_name, status)
    VALUES (u_owner, '+919000100001', 'rahul@vidyawarrior.in', 'Rahul Verma (Owner)', 'active');
    INSERT INTO auth_identities (user_id, provider, provider_uid, verified_at)
    VALUES (u_owner, 'phone', '+919000100001', now());

    INSERT INTO tenants (id, org_code, name, slug, status, plan, legal_name, gstin, pan,
                         registered_address, place_of_supply, billing_email, timezone, owner_user_id, trial_ends_at)
    VALUES (t_id, 'VWSTUDY', 'Vidya Warrior Classes', 'vidya-warrior', 'active', 'pro',
            'Vidya Warrior Eduventures Pvt Ltd', '08AABCV1234C1Z5', 'AABCV1234C',
            '{"line1":"12 Talwandi","city":"Kota","state":"Rajasthan","pincode":"324005"}'::jsonb,
            '08', 'accounts@vidyawarrior.in', 'Asia/Kolkata', u_owner, now() + interval '4 days');

    INSERT INTO tenant_settings (tenant_id, features, theme, payment_config, notification_config)
    VALUES (t_id,
            '{"live":true,"store":true,"tests":true,"ai_doubts":true,"downloads":true,"referrals":true}'::jsonb,
            '{"primary":"#4f46e5","accent":"#f59e0b"}'::jsonb,
            '{"gateway":"razorpay","route_enabled":true}'::jsonb,
            '{"digest":"daily"}'::jsonb);

    INSERT INTO tenant_domains (tenant_id, domain, is_primary, verified_at, ssl_status)
    VALUES (t_id, 'app.vidyawarrior.in', true, now(), 'active');

    INSERT INTO tenant_users (tenant_id, user_id, role, status, joined_at)
    VALUES (t_id, u_owner, 'owner', 'active', now() - interval '20 days');

    -- Our (platform) billing of this tenant.
    INSERT INTO platform_subscriptions (tenant_id, plan, status, amount_minor, current_period_start, current_period_end)
    VALUES (t_id, 'pro', 'active', 499900, date_trunc('month', now()), date_trunc('month', now()) + interval '1 month');

    -- Their white-label Android app build.
    INSERT INTO app_builds (tenant_id, platform, status, package_id, version_code, version_name, store_url, completed_at)
    VALUES (t_id, 'android', 'published', 'in.vidyawarrior.app', 7, '1.4.0',
            'https://play.google.com/store/apps/details?id=in.vidyawarrior.app', now() - interval '5 days');

    -- ========================================================================
    -- PHASE 2 — TENANT BUILDS ITS TEAM.  Admin, two instructors, a staff
    --           member, and a parent account.
    -- ========================================================================
    INSERT INTO users (id, phone, email, full_name, status) VALUES
        (u_admin,     '+919000100002', 'anita@vidyawarrior.in',  'Anita Sharma (Admin)',     'active'),
        (u_inst_phys, '+919000100003', 'suresh@vidyawarrior.in', 'Dr. Suresh Iyer (Physics)','active'),
        (u_inst_chem, '+919000100004', 'meena@vidyawarrior.in',  'Meena Nair (Chemistry)',   'active'),
        (u_staff,     '+919000100005', 'kiran@vidyawarrior.in',  'Kiran Rao (Front Desk)',   'active'),
        (u_parent,    '+919000100006', NULL,                     'Parent of Aarav',          'active');

    INSERT INTO auth_identities (user_id, provider, provider_uid, verified_at) VALUES
        (u_admin,     'phone', '+919000100002', now()),
        (u_inst_phys, 'phone', '+919000100003', now()),
        (u_inst_chem, 'phone', '+919000100004', now()),
        (u_inst_chem, 'google','meena.nair.g',  now()),
        (u_staff,     'phone', '+919000100005', now()),
        (u_parent,    'phone', '+919000100006', now());

    INSERT INTO tenant_users (tenant_id, user_id, role, status, invited_by, joined_at) VALUES
        (t_id, u_admin,     'admin',      'active', u_owner, now() - interval '19 days'),
        (t_id, u_inst_phys, 'instructor', 'active', u_admin, now() - interval '18 days'),
        (t_id, u_inst_chem, 'instructor', 'active', u_admin, now() - interval '18 days'),
        (t_id, u_staff,     'staff',      'active', u_admin, now() - interval '17 days'),
        (t_id, u_parent,    'parent',     'active', u_admin, now() - interval '3 days');

    INSERT INTO user_profiles (tenant_id, user_id, onboarding_completed) VALUES
        (t_id, u_owner, true), (t_id, u_admin, true),
        (t_id, u_inst_phys, true), (t_id, u_inst_chem, true), (t_id, u_staff, true);

    INSERT INTO notification_preferences (tenant_id, user_id, channel, category, enabled) VALUES
        (t_id, u_admin,     'email',  'billing',  true),
        (t_id, u_inst_phys, 'push',   'doubts',   true),
        (t_id, u_inst_chem, 'push',   'doubts',   true);

    INSERT INTO device_tokens (tenant_id, user_id, token, platform) VALUES
        (t_id, u_admin,     'fcm-admin-'||gen_random_uuid(),  'web'),
        (t_id, u_inst_phys, 'fcm-suresh-'||gen_random_uuid(), 'android');

    -- ========================================================================
    -- PHASE 3 — STUDENTS SIGN UP.  Six students onboard (one via a referral
    --           link — see PHASE 17), each gets a profile, wallet and code.
    -- ========================================================================
    INSERT INTO users (id, phone, full_name, status) VALUES
        (u_stuA, '+919000100010', 'Aarav Gupta',   'active'),
        (u_stuB, '+919000100011', 'Diya Patel',    'active'),
        (u_stuC, '+919000100012', 'Rohan Mehta',   'active'),
        (u_stuD, '+919000100013', 'Ishita Singh',  'active'),
        (u_stuE, '+919000100014', 'Kabir Khan',    'active'),
        (u_stuF, '+919000100015', 'Ananya Reddy',  'active');

    INSERT INTO auth_identities (user_id, provider, provider_uid, verified_at)
    SELECT id, 'phone', phone, now() FROM users WHERE id IN (u_stuA,u_stuB,u_stuC,u_stuD,u_stuE,u_stuF);

    INSERT INTO tenant_users (tenant_id, user_id, role, status, joined_at)
    SELECT t_id, id, 'student', 'active', now() - (random()*15 || ' days')::interval
    FROM users WHERE id IN (u_stuA,u_stuB,u_stuC,u_stuD,u_stuE,u_stuF);

    INSERT INTO user_profiles (tenant_id, user_id, class_level, board, exam_goal, onboarding_completed, guardian_name, guardian_phone) VALUES
        (t_id, u_stuA, '12', 'CBSE',      'JEE Advanced 2027', true, 'Parent of Aarav', '+919000100006'),
        (t_id, u_stuB, '12', 'CBSE',      'NEET 2027',         true, 'Mrs. Patel',      '+919812345671'),
        (t_id, u_stuC, '11', 'CBSE',      'JEE Main 2028',     true, 'Mr. Mehta',       '+919812345672'),
        (t_id, u_stuD, '12', 'RBSE',      'NEET 2027',         true, 'Mrs. Singh',      '+919812345673'),
        (t_id, u_stuE, '12', 'CBSE',      'JEE Advanced 2027', true, 'Mr. Khan',        '+919812345674'),
        (t_id, u_stuF, '11', 'CBSE',      'JEE Main 2028',     false,'Mrs. Reddy',      '+919812345675');

    INSERT INTO wallets (tenant_id, user_id)
    SELECT t_id, id FROM users WHERE id IN (u_stuA,u_stuB,u_stuC,u_stuD,u_stuE,u_stuF);

    INSERT INTO referral_codes (tenant_id, user_id, code) VALUES
        (t_id, u_stuA, 'AARAV50'), (t_id, u_stuB, 'DIYA50');

    INSERT INTO notification_preferences (tenant_id, user_id, channel, category)
    SELECT t_id, id, 'push', 'classes' FROM users WHERE id IN (u_stuA,u_stuB,u_stuC);
    INSERT INTO device_tokens (tenant_id, user_id, token, platform)
    SELECT t_id, id, 'fcm-stu-'||gen_random_uuid(), 'android' FROM users WHERE id IN (u_stuA,u_stuB);

    -- ========================================================================
    -- PHASE 4 — CURRICULUM TAXONOMY.  Exam categories (platform) → subjects →
    --           chapters → topics (per tenant).
    -- ========================================================================
    INSERT INTO exam_categories (name, slug, display_order) VALUES ('JEE', 'jee', 1) RETURNING id INTO ec_jee;
    INSERT INTO exam_categories (parent_id, name, slug, display_order) VALUES (ec_jee, 'JEE Main', 'jee-main', 1) RETURNING id INTO ec_jee_main;
    INSERT INTO exam_categories (name, slug, display_order) VALUES ('NEET', 'neet', 2) RETURNING id INTO ec_neet;

    INSERT INTO subjects (tenant_id, exam_category_id, name, code) VALUES
        (t_id, ec_jee,  'Physics',   'PHY')  RETURNING id INTO subj_phys;
    INSERT INTO subjects (tenant_id, exam_category_id, name, code) VALUES
        (t_id, ec_jee,  'Chemistry', 'CHE')  RETURNING id INTO subj_chem;
    INSERT INTO subjects (tenant_id, exam_category_id, name, code) VALUES
        (t_id, ec_jee,  'Mathematics','MAT') RETURNING id INTO subj_maths;
    INSERT INTO subjects (tenant_id, exam_category_id, name, code) VALUES
        (t_id, ec_neet, 'Biology',   'BIO')  RETURNING id INTO subj_bio;

    INSERT INTO chapters (tenant_id, subject_id, name, display_order, is_free) VALUES
        (t_id, subj_phys, 'Kinematics', 1, true)  RETURNING id INTO ch_kin;
    INSERT INTO chapters (tenant_id, subject_id, name, display_order) VALUES
        (t_id, subj_phys, 'Thermodynamics', 2)    RETURNING id INTO ch_thermo;
    INSERT INTO chapters (tenant_id, subject_id, name, display_order) VALUES
        (t_id, subj_chem, 'Mole Concept', 1)      RETURNING id INTO ch_mole;

    INSERT INTO topics (tenant_id, chapter_id, name, display_order, is_free) VALUES
        (t_id, ch_kin, 'Displacement vs Distance', 1, true) RETURNING id INTO tp_disp;
    INSERT INTO topics (tenant_id, chapter_id, name, display_order) VALUES
        (t_id, ch_kin, 'Projectile Motion', 2)              RETURNING id INTO tp_proj;
    INSERT INTO topics (tenant_id, chapter_id, name, display_order) VALUES
        (t_id, ch_thermo, 'Heat & Work', 1)                 RETURNING id INTO tp_heat;
    INSERT INTO topics (tenant_id, chapter_id, name, display_order) VALUES
        (t_id, ch_mole, 'Stoichiometry', 1)                 RETURNING id INTO tp_stoich;

    -- ========================================================================
    -- PHASE 5 — CATALOG.  Three courses: a free taster, the flagship paid
    --           course (published + approved), one still in review.
    --           Plus a batch and a recurring weekly schedule.
    -- ========================================================================
    INSERT INTO courses (id, tenant_id, exam_category_id, title, slug, summary, language, level,
                         class_level, exam_goal, status, approval_status, approved_by, approved_at,
                         hsn_sac, tax_rate_bps, refund_window_days, created_by)
    VALUES (gen_random_uuid(), t_id, ec_jee, 'Kinematics Crash Course (Free)', 'kinematics-free',
            'A free 5-lesson intro to motion in 1D and 2D.', 'en', 'foundation', '11', 'JEE Main 2028',
            'published', 'approved', u_owner, now() - interval '15 days', '999293', 1800, 0, u_inst_phys)
    RETURNING id INTO c_free;

    INSERT INTO courses (id, tenant_id, exam_category_id, title, slug, summary, description_rich, language, level,
                         class_level, exam_goal, status, approval_status, approved_by, approved_at,
                         hsn_sac, tax_rate_bps, refund_window_days, starts_on, seats, created_by)
    VALUES (gen_random_uuid(), t_id, ec_jee, 'JEE Physics 2027 — Full Course', 'jee-physics-2027',
            'Complete Class 11 + 12 physics for JEE, live + recorded, weekly tests.',
            '{"blocks":[{"type":"p","text":"180 hours of teaching, 40 DPPs, 12 full mocks."}]}'::jsonb,
            'en', 'advanced', '12', 'JEE Advanced 2027',
            'published', 'approved', u_owner, now() - interval '10 days', '999293', 1800, 7,
            current_date - 30, 120, u_inst_phys)
    RETURNING id INTO c_paid;

    INSERT INTO courses (id, tenant_id, exam_category_id, title, slug, summary, language, level,
                         class_level, exam_goal, status, approval_status, created_by)
    VALUES (gen_random_uuid(), t_id, ec_neet, 'NEET Biology 2027', 'neet-biology-2027',
            'Botany + Zoology for NEET.', 'en', 'advanced', '12', 'NEET 2027',
            'in_review', 'pending', u_inst_chem)
    RETURNING id INTO c_review;

    INSERT INTO course_instructors (tenant_id, course_id, user_id, role, revenue_share_bps) VALUES
        (t_id, c_paid, u_inst_phys, 'owner',      6000),
        (t_id, c_paid, u_inst_chem, 'instructor', 1500),
        (t_id, c_free, u_inst_phys, 'owner',      0);

    INSERT INTO course_sections (tenant_id, course_id, title, display_order, drip_after_days) VALUES
        (t_id, c_paid, 'Module 1 — Mechanics', 1, 0) RETURNING id INTO sec_paid1;
    INSERT INTO course_sections (tenant_id, course_id, title, display_order, drip_after_days) VALUES
        (t_id, c_paid, 'Module 2 — Thermodynamics', 2, 14) RETURNING id INTO sec_paid2;

    INSERT INTO batches (id, tenant_id, course_id, name, instructor_id, starts_on, ends_on, max_students)
    VALUES (gen_random_uuid(), t_id, c_paid, 'JEE Physics 2027 — Morning Batch', u_inst_phys,
            current_date - 30, current_date + 300, 60)
    RETURNING id INTO b_paid;

    INSERT INTO class_schedules (id, tenant_id, course_id, batch_id, instructor_id, title, description,
                                 by_weekday, start_local, duration_min, timezone, starts_on, last_materialised_at)
    VALUES (gen_random_uuid(), t_id, c_paid, b_paid, u_inst_phys, 'Physics — Mon/Wed/Fri live',
            'Live problem-solving after each recorded module.', ARRAY[1,3,5]::smallint[], '18:00', 90,
            'Asia/Kolkata', current_date - 30, now() - interval '1 day')
    RETURNING id INTO sched1;

    -- ========================================================================
    -- PHASE 6 — CONTENT AUTHORING.  Typed content bodies, a video asset with
    --           renditions, two live sessions (one ended → recording + chat),
    --           and one course_lesson of EVERY content_kind.
    -- ========================================================================
    INSERT INTO content_videos (tenant_id, title, provider, playback_id, duration_sec) VALUES
        (t_id, 'Displacement vs Distance — lecture', 'self', 'vw/kin/disp.m3u8', 1840) RETURNING id INTO vid1;
    INSERT INTO content_videos (tenant_id, title, provider, playback_id, duration_sec) VALUES
        (t_id, 'Projectile Motion — full derivation', 'youtube', 'dQw4w9WgXcQ', 2510) RETURNING id INTO vid2;
    INSERT INTO content_documents (tenant_id, title, file_key, file_size, mime, page_count) VALUES
        (t_id, 'Kinematics formula sheet', 'vw/docs/kin-formulae.pdf', 284000, 'application/pdf', 4) RETURNING id INTO doc1;
    INSERT INTO content_links (tenant_id, title, url) VALUES
        (t_id, 'PhET — Projectile Motion simulation', 'https://phet.colorado.edu/sims/html/projectile-motion') RETURNING id INTO lnk1;

    INSERT INTO video_assets (tenant_id, source_key, status, duration_sec) VALUES
        (t_id, 'vw/raw/kin-disp.mp4', 'ready', 1840) RETURNING id INTO vasset;
    INSERT INTO video_renditions (tenant_id, video_asset_id, height, bitrate_kbps, codec, file_key, file_size) VALUES
        (t_id, vasset, 360,  700,  'h264', 'vw/hls/kin-disp/360.m3u8',  92000000),
        (t_id, vasset, 720,  2500, 'h264', 'vw/hls/kin-disp/720.m3u8',  240000000),
        (t_id, vasset, 1080, 5000, 'h264', 'vw/hls/kin-disp/1080.m3u8', 460000000);

    INSERT INTO live_sessions (id, tenant_id, course_id, batch_id, instructor_id, schedule_id, title,
                               status, ingest_key, scheduled_start)
    VALUES (gen_random_uuid(), t_id, c_paid, b_paid, u_inst_phys, sched1, 'Live — Kinematics problem set',
            'scheduled', 'ik-'||replace(gen_random_uuid()::text,'-',''), now() + interval '2 days')
    RETURNING id INTO ls_scheduled;

    INSERT INTO live_sessions (id, tenant_id, course_id, batch_id, instructor_id, schedule_id, title,
                               status, ingest_key, scheduled_start, actual_start, actual_end, peak_viewers)
    VALUES (gen_random_uuid(), t_id, c_paid, b_paid, u_inst_phys, sched1, 'Live — Newton''s laws recap',
            'ended', 'ik-'||replace(gen_random_uuid()::text,'-',''), now() - interval '2 days',
            now() - interval '2 days', now() - interval '2 days' + interval '92 min', 47)
    RETURNING id INTO ls_ended;

    INSERT INTO recordings (id, tenant_id, session_id, video_asset_id, file_key, file_size, duration_sec, status, thumbnail_url)
    VALUES (gen_random_uuid(), t_id, ls_ended, vasset, 'vw/rec/newton-recap.mp4', 610000000, 5520, 'ready',
            'vw/rec/newton-recap.jpg')
    RETURNING id INTO rec1;

    INSERT INTO session_messages (tenant_id, session_id, user_id, kind, body) VALUES
        (t_id, ls_ended, u_stuA, 'chat',   'Sir, doubt in Q3 — why is tension the same on both sides?'),
        (t_id, ls_ended, u_inst_phys, 'chat', 'Massless string + frictionless pulley → tension is uniform.'),
        (t_id, ls_ended, u_inst_phys, 'pinned', 'DPP-4 is due Sunday 11pm.');

    INSERT INTO qr_check_ins (tenant_id, session_id, code, expires_at, created_by)
    VALUES (t_id, ls_ended, 'QR-'||upper(substr(replace(gen_random_uuid()::text,'-',''),1,8)),
            now() - interval '2 days' + interval '15 min', u_inst_phys);

    -- one lesson of every kind, hung off the paid course
    INSERT INTO course_lessons (id, tenant_id, course_id, section_id, title, content_kind, video_id, is_preview, display_order, status)
    VALUES (gen_random_uuid(), t_id, c_paid, sec_paid1, 'Displacement vs Distance', 'video', vid1, true, 1, 'published') RETURNING id INTO les_video;
    INSERT INTO course_lessons (id, tenant_id, course_id, section_id, title, content_kind, document_id, display_order, status)
    VALUES (gen_random_uuid(), t_id, c_paid, sec_paid1, 'Formula sheet (PDF)', 'document', doc1, 2, 'published') RETURNING id INTO les_doc;
    INSERT INTO course_lessons (id, tenant_id, course_id, section_id, title, content_kind, link_id, display_order, status)
    VALUES (gen_random_uuid(), t_id, c_paid, sec_paid1, 'Projectile simulation', 'link', lnk1, 3, 'published') RETURNING id INTO les_link;
    INSERT INTO course_lessons (id, tenant_id, course_id, section_id, title, content_kind, live_session_id, display_order, status)
    VALUES (gen_random_uuid(), t_id, c_paid, sec_paid1, 'Live — Newton''s laws recap', 'live_session', ls_ended, 4, 'published') RETURNING id INTO les_live;
    INSERT INTO course_lessons (id, tenant_id, course_id, section_id, title, content_kind, display_order, status)
    VALUES (gen_random_uuid(), t_id, c_paid, sec_paid1, 'Module 1 quiz', 'quiz', 5, 'published') RETURNING id INTO les_quiz;
    INSERT INTO course_lessons (id, tenant_id, course_id, section_id, title, content_kind, display_order, status)
    VALUES (gen_random_uuid(), t_id, c_paid, sec_paid2, 'Thermo assignment', 'assignment', 6, 'published') RETURNING id INTO les_assign;

    -- ========================================================================
    -- PHASE 7 — ASSESSMENT AUTHORING.  A reusable question bank (all 5 kinds)
    --           and two tests (a DPP + a full mock) built from it.
    -- ========================================================================
    INSERT INTO question_bank (id, tenant_id, subject_id, topic_id, kind, stem_rich, solution_rich, difficulty, default_marks, negative_marks, created_by)
    VALUES (gen_random_uuid(), t_id, subj_phys, tp_disp, 'mcq_single',
            '{"text":"A body moves 3 m east then 4 m north. Displacement magnitude?"}'::jsonb,
            '{"text":"5 m (3-4-5 triangle)."}'::jsonb, 'easy', 4, 1, u_inst_phys) RETURNING id INTO q_mcq1;
    INSERT INTO question_bank (id, tenant_id, subject_id, topic_id, kind, stem_rich, difficulty, default_marks, negative_marks, created_by)
    VALUES (gen_random_uuid(), t_id, subj_phys, tp_proj, 'mcq_single',
            '{"text":"Range is maximum at launch angle?"}'::jsonb, 'easy', 4, 1, u_inst_phys) RETURNING id INTO q_mcq2;
    INSERT INTO question_bank (id, tenant_id, subject_id, topic_id, kind, stem_rich, difficulty, default_marks, negative_marks, created_by)
    VALUES (gen_random_uuid(), t_id, subj_phys, tp_proj, 'mcq_multi',
            '{"text":"Which are vector quantities?"}'::jsonb, 'medium', 4, 2, u_inst_phys) RETURNING id INTO q_multi;
    INSERT INTO question_bank (id, tenant_id, subject_id, topic_id, kind, stem_rich, difficulty, default_marks, numeric_answer, numeric_tolerance, created_by)
    VALUES (gen_random_uuid(), t_id, subj_phys, tp_disp, 'numeric',
            '{"text":"Speed (m/s) if 100 m covered in 8 s? (1 dp)"}'::jsonb, 'medium', 4, 12.5, 0.1, u_inst_phys) RETURNING id INTO q_num;
    INSERT INTO question_bank (id, tenant_id, subject_id, topic_id, kind, stem_rich, difficulty, default_marks, created_by)
    VALUES (gen_random_uuid(), t_id, subj_phys, tp_heat, 'subjective',
            '{"text":"State and explain the first law of thermodynamics."}'::jsonb, 'hard', 6, u_inst_phys) RETURNING id INTO q_subj;
    INSERT INTO question_bank (id, tenant_id, subject_id, topic_id, kind, stem_rich, difficulty, default_marks, created_by)
    VALUES (gen_random_uuid(), t_id, subj_chem, tp_stoich, 'match',
            '{"text":"Match the quantity to its unit."}'::jsonb, 'medium', 4, u_inst_chem) RETURNING id INTO q_match;

    INSERT INTO question_options (tenant_id, question_id, label, is_correct, display_order) VALUES
        (t_id, q_mcq1, '5 m',  true,  1), (t_id, q_mcq1, '7 m', false, 2), (t_id, q_mcq1, '1 m', false, 3),
        (t_id, q_mcq2, '45°',  true,  1), (t_id, q_mcq2, '30°', false, 2), (t_id, q_mcq2, '60°', false, 3),
        (t_id, q_multi,'Velocity', true, 1), (t_id, q_multi,'Force', true, 2), (t_id, q_multi,'Speed', false, 3), (t_id, q_multi,'Mass', false, 4);

    INSERT INTO tests (id, tenant_id, course_id, subject_id, chapter_id, exam_category_id, title, kind,
                       duration_min, total_marks, pass_marks, negative_marking, attempts_allowed, is_free, status, created_by)
    VALUES (gen_random_uuid(), t_id, c_paid, subj_phys, ch_kin, ec_jee, 'DPP 1 — Kinematics', 'dpp',
            20, 12, 4, true, 3, false, 'published', u_inst_phys) RETURNING id INTO tst_dpp;
    INSERT INTO tests (id, tenant_id, course_id, subject_id, exam_category_id, title, kind, exam_year,
                       duration_min, total_marks, pass_marks, negative_marking, shuffle_questions, max_tab_switches, attempts_allowed, status, created_by)
    VALUES (gen_random_uuid(), t_id, c_paid, subj_phys, ec_jee, 'Full Mock — Physics Paper 1', 'mock', 2026,
            180, 22, 8, true, true, 3, 1, 'published', u_inst_phys) RETURNING id INTO tst_mock;

    INSERT INTO test_sections (tenant_id, test_id, title, display_order, marks_per_q, negative_per_q)
    VALUES (t_id, tst_mock, 'Section A — Single correct', 1, 4, 1) RETURNING id INTO tsec1;

    INSERT INTO test_questions (tenant_id, test_id, question_id, display_order, marks, negative) VALUES
        (t_id, tst_dpp,  q_mcq1, 1, 4, 1),
        (t_id, tst_dpp,  q_mcq2, 2, 4, 1),
        (t_id, tst_dpp,  q_num,  3, 4, 0);
    INSERT INTO test_questions (tenant_id, test_id, section_id, question_id, display_order, marks, negative) VALUES
        (t_id, tst_mock, tsec1, q_mcq1,  1, 4, 1),
        (t_id, tst_mock, tsec1, q_mcq2,  2, 4, 1),
        (t_id, tst_mock, tsec1, q_multi, 3, 4, 2),
        (t_id, tst_mock, tsec1, q_num,   4, 4, 0),
        (t_id, tst_mock, tsec1, q_subj,  5, 6, 0);

    -- ========================================================================
    -- PHASE 8 — COMMERCE CATALOGUE.  A product+price for each sellable thing
    --           (course, bundle, plan, fee-plan), a 2-course bundle, a
    --           subscription plan, a fee plan, and a launch coupon.
    -- ========================================================================
    INSERT INTO products (id, tenant_id, kind, course_id, hsn_sac, tax_rate_bps) VALUES
        (gen_random_uuid(), t_id, 'course', c_paid, '999293', 1800) RETURNING id INTO p_course;
    INSERT INTO products (id, tenant_id, kind, course_id, hsn_sac, tax_rate_bps) VALUES
        (gen_random_uuid(), t_id, 'course', c_free, '999293', 1800) RETURNING id INTO p_course_free;

    INSERT INTO course_bundles (id, tenant_id, title, description) VALUES
        (gen_random_uuid(), t_id, 'JEE Physics + Free Kinematics Combo', 'Flagship course, kinematics crash included.')
    RETURNING id INTO bndl;
    INSERT INTO products (id, tenant_id, kind, bundle_id, hsn_sac, tax_rate_bps) VALUES
        (gen_random_uuid(), t_id, 'bundle', bndl, '999293', 1800) RETURNING id INTO p_bundle;
    INSERT INTO bundle_items (tenant_id, bundle_product_id, item_product_id, position) VALUES
        (t_id, p_bundle, p_course, 1), (t_id, p_bundle, p_course_free, 2);

    INSERT INTO subscription_plans (id, tenant_id, name, slug, description, interval, interval_days, trial_days, features, hsn_sac, tax_rate_bps)
    VALUES (gen_random_uuid(), t_id, 'All-Access Monthly', 'all-access-monthly',
            'Every course + every test for 30 days.', 'monthly', 30, 3,
            '["all courses","all tests","priority doubts"]'::jsonb, '998431', 1800)
    RETURNING id INTO plan;
    INSERT INTO products (id, tenant_id, kind, plan_id, hsn_sac, tax_rate_bps) VALUES
        (gen_random_uuid(), t_id, 'plan', plan, '998431', 1800) RETURNING id INTO p_plan;

    INSERT INTO fee_plans (id, tenant_id, course_id, batch_id, name, total_minor, installments_count, gap_days, late_fee_minor, hsn_sac, tax_rate_bps)
    VALUES (gen_random_uuid(), t_id, c_paid, b_paid, 'JEE Physics — 3-instalment fee plan',
            1500000, 3, 30, 50000, '999293', 1800)
    RETURNING id INTO feeplan;
    INSERT INTO products (id, tenant_id, kind, fee_plan_id, hsn_sac, tax_rate_bps) VALUES
        (gen_random_uuid(), t_id, 'fee_plan', feeplan, '999293', 1800) RETURNING id INTO p_feeplan;

    INSERT INTO prices (tenant_id, product_id, amount_minor, compare_at_minor) VALUES
        (t_id, p_course, 499900, 799900),
        (t_id, p_course_free, 0, NULL),
        (t_id, p_bundle, 549900, 899900),
        (t_id, p_plan, 99900, NULL),
        (t_id, p_feeplan, 1500000, NULL);

    INSERT INTO coupons (id, tenant_id, code, type, percent_bps, max_discount_minor, min_order_minor, applies_to, usage_limit, per_user_limit)
    VALUES (gen_random_uuid(), t_id, 'LAUNCH10', 'percent', 1000, 100000, 0, 'products', 100, 1)
    RETURNING id INTO cpn;
    INSERT INTO coupon_products (tenant_id, coupon_id, product_id) VALUES (t_id, cpn, p_course);

    -- ========================================================================
    -- PHASE 9 — PAID COURSE PURCHASE (the core money flow).  Student A buys
    --           JEE Physics with LAUNCH10:
    --   order(paid) → order_item(GST split) → payment(captured) + Route split
    --   → entitlement(purchase) → enrollment → GST invoice (+ line item)
    --   → coupon_redemption → webhook_event → outbox event → audit log.
    -- Prices are GST-inclusive.  ₹4,999 − 10% (₹499.90 → capped ₹1,000? no,
    -- 10% of 4999 = 499.90 < 1000 cap) = ₹4,499.10 = 449910 paise.
    --   taxable 381280, CGST 34315, SGST 34315  (sum 449910).
    -- ========================================================================
    INSERT INTO orders (id, tenant_id, user_id, code, status, subtotal_minor, discount_minor, tax_minor, total_minor,
                        coupon_id, gateway, gateway_order_id, place_of_supply, notes, placed_at, paid_at, refund_deadline_at)
    VALUES (gen_random_uuid(), t_id, u_stuA, 'VW-ORD-1001', 'paid', 499900, 49990, 0, 449910,
            cpn, 'razorpay', 'order_devseed1001', '08',
            '{"product":"jee-physics-2027"}'::jsonb, now() - interval '9 days', now() - interval '9 days',
            now() - interval '9 days' + interval '7 days')
    RETURNING id INTO ord_A;

    INSERT INTO order_items (id, tenant_id, order_id, product_id, product_kind, title, hsn_sac,
                             unit_minor, qty, line_subtotal_minor, discount_minor, taxable_minor,
                             cgst_minor, sgst_minor, total_minor, grants_entitlement, entitlement_days)
    VALUES (gen_random_uuid(), t_id, ord_A, p_course, 'course', 'JEE Physics 2027 — Full Course', '999293',
            499900, 1, 499900, 49990, 381280, 34315, 34315, 449910, true, 365)
    RETURNING id INTO oi_A;

    INSERT INTO payments (id, tenant_id, order_id, user_id, gateway, gateway_order_id, gateway_payment_id,
                          method, status, amount_minor, gateway_fee_minor, captured_at, raw)
    VALUES (gen_random_uuid(), t_id, ord_A, u_stuA, 'razorpay', 'order_devseed1001', 'pay_devseed1001',
            'upi', 'captured', 449910, 10618, now() - interval '9 days',
            '{"acquirer":"HDFC","vpa":"aarav@upi"}'::jsonb)
    RETURNING id INTO pay_A;

    INSERT INTO payment_splits (tenant_id, payment_id, linked_account_id, amount_minor, on_hold, gateway_transfer_id, settled_at)
    VALUES (t_id, pay_A, 'acc_VWLINKED01', 382423, false, 'trf_devseed1001', now() - interval '7 days');

    INSERT INTO entitlements (id, tenant_id, user_id, product_id, product_kind, source, order_item_id, granted_at, expires_at)
    VALUES (gen_random_uuid(), t_id, u_stuA, p_course, 'course', 'purchase', oi_A,
            now() - interval '9 days', now() + interval '356 days')
    RETURNING id INTO ent_A;

    INSERT INTO enrollments (id, tenant_id, user_id, course_id, batch_id, entitlement_id, status, progress_bps, started_at)
    VALUES (gen_random_uuid(), t_id, u_stuA, c_paid, b_paid, ent_A, 'active', 2600, now() - interval '9 days')
    RETURNING id INTO enr_A;

    INSERT INTO coupon_redemptions (tenant_id, coupon_id, user_id, order_id, amount_off_minor)
    VALUES (t_id, cpn, u_stuA, ord_A, 49990);
    UPDATE coupons SET used_count = used_count + 1 WHERE id = cpn;

    -- GST invoice — gapless per-tenant per-FY numbering.
    INSERT INTO invoice_number_series (tenant_id, fin_year, prefix, next_seq)
    VALUES (t_id, fy, 'VW', 2)  -- allocates seq 1, next is 2
    ON CONFLICT (tenant_id, fin_year) DO NOTHING;

    INSERT INTO invoices (id, tenant_id, order_id, number, fin_year, status, supply_type, place_of_supply,
                          buyer_snapshot, seller_snapshot, taxable_minor, cgst_minor, sgst_minor, round_off_minor, total_minor, issued_at)
    VALUES (gen_random_uuid(), t_id, ord_A, 'VW/'||fy||'/000001', fy, 'issued', 'intra_state', '08',
            '{"name":"Aarav Gupta","phone":"+919000100010","place_of_supply":"08"}'::jsonb,
            '{"name":"Vidya Warrior Classes","legal_name":"Vidya Warrior Eduventures Pvt Ltd","gstin":"08AABCV1234C1Z5","place_of_supply":"08"}'::jsonb,
            381280, 34315, 34315, 0, 449910, now() - interval '9 days')
    RETURNING id INTO inv_A;
    INSERT INTO invoice_line_items (tenant_id, invoice_id, description, hsn_sac, qty, unit_minor, taxable_minor, rate_bps, cgst_minor, sgst_minor, total_minor)
    VALUES (t_id, inv_A, 'JEE Physics 2027 — Full Course', '999293', 1, 449910, 381280, 1800, 34315, 34315, 449910);

    INSERT INTO webhook_events (gateway, event_id, event_type, payload, signature_ok, processed_at)
    VALUES ('razorpay', 'payment.captured:pay_devseed1001', 'payment.captured',
            '{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_devseed1001","order_id":"order_devseed1001"}}}}'::jsonb,
            true, now() - interval '9 days');

    INSERT INTO outbox (aggregate_type, aggregate_id, event_type, tenant_id, payload, published_at)
    VALUES ('order', ord_A, 'course.purchased', t_id,
            jsonb_build_object('order_id', ord_A, 'user_id', u_stuA, 'course_id', c_paid), now() - interval '9 days');

    INSERT INTO audit_logs (tenant_id, actor_user_id, actor_role, action, entity_type, entity_id, after, ip)
    VALUES (t_id, u_stuA, 'student', 'order.paid', 'order', ord_A,
            jsonb_build_object('total_minor', 449910, 'code', 'VW-ORD-1001'), '203.0.113.10');

    -- ========================================================================
    -- PHASE 10 — PARTIAL REFUND.  Student A requests a ₹500 goodwill refund.
    --   refund → credit note (gapless CN numbering) → order → partially_refunded.
    --   Entitlement + enrolment stay (partial).
    -- ========================================================================
    INSERT INTO refunds (id, tenant_id, payment_id, order_item_id, amount_minor, reason, status, gateway_refund_id, initiated_by, processed_at)
    VALUES (gen_random_uuid(), t_id, pay_A, oi_A, 50000, 'goodwill', 'processed', 'rfnd_devseed1001', u_admin, now() - interval '4 days')
    RETURNING id INTO ref_A;
    UPDATE orders SET status = 'partially_refunded' WHERE id = ord_A;

    INSERT INTO credit_note_number_series (tenant_id, fin_year, prefix, next_seq)
    VALUES (t_id, fy, 'VWCN', 2) ON CONFLICT (tenant_id, fin_year) DO NOTHING;

    INSERT INTO credit_notes (id, tenant_id, invoice_id, refund_id, number, fin_year, reason,
                              taxable_minor, cgst_minor, sgst_minor, round_off_minor, total_minor, issued_at)
    VALUES (gen_random_uuid(), t_id, inv_A, ref_A, 'VWCN/'||fy||'/000001', fy, 'goodwill',
            42373, 3814, 3813, 0, 50000, now() - interval '4 days')
    RETURNING id INTO cn_A;
    INSERT INTO credit_note_line_items (tenant_id, credit_note_id, description, hsn_sac, qty, unit_minor, taxable_minor, rate_bps, cgst_minor, sgst_minor, total_minor)
    VALUES (t_id, cn_A, 'Partial refund — goodwill', '999293', 1, 50000, 42373, 1800, 3814, 3813, 50000);

    -- ========================================================================
    -- PHASE 11 — BUNDLE PURCHASE (entitlement fan-out).  Student B buys the
    --   combo → ONE order → TWO entitlements (paid course + free course) →
    --   TWO enrolments → invoice.
    -- ========================================================================
    INSERT INTO orders (id, tenant_id, user_id, code, status, subtotal_minor, discount_minor, tax_minor, total_minor,
                        gateway, gateway_order_id, place_of_supply, placed_at, paid_at)
    VALUES (gen_random_uuid(), t_id, u_stuB, 'VW-ORD-1002', 'paid', 549900, 0, 0, 549900,
            'razorpay', 'order_devseed1002', '27', now() - interval '6 days', now() - interval '6 days')
    RETURNING id INTO ord_B;
    INSERT INTO order_items (id, tenant_id, order_id, product_id, product_kind, title, hsn_sac,
                             unit_minor, qty, line_subtotal_minor, taxable_minor, igst_minor, total_minor, grants_entitlement)
    VALUES (gen_random_uuid(), t_id, ord_B, p_bundle, 'bundle', 'JEE Physics + Free Kinematics Combo', '999293',
            549900, 1, 549900, 466017, 83883, 549900, true)
    RETURNING id INTO oi_B;
    INSERT INTO payments (id, tenant_id, order_id, user_id, gateway, gateway_payment_id, method, status, amount_minor, captured_at)
    VALUES (gen_random_uuid(), t_id, ord_B, u_stuB, 'razorpay', 'pay_devseed1002', 'card', 'captured', 549900, now() - interval '6 days')
    RETURNING id INTO pay_B;

    INSERT INTO entitlements (id, tenant_id, user_id, product_id, product_kind, source, order_item_id, granted_at)
    VALUES (gen_random_uuid(), t_id, u_stuB, p_course, 'course', 'bundle', oi_B, now() - interval '6 days') RETURNING id INTO ent_B1;
    INSERT INTO entitlements (id, tenant_id, user_id, product_id, product_kind, source, order_item_id, granted_at)
    VALUES (gen_random_uuid(), t_id, u_stuB, p_course_free, 'course', 'bundle', oi_B, now() - interval '6 days') RETURNING id INTO ent_B2;

    INSERT INTO enrollments (id, tenant_id, user_id, course_id, entitlement_id, status, progress_bps, started_at) VALUES
        (gen_random_uuid(), t_id, u_stuB, c_paid, ent_B1, 'active', 800,  now() - interval '6 days'),
        (gen_random_uuid(), t_id, u_stuB, c_free, ent_B2, 'active', 4500, now() - interval '6 days');

    INSERT INTO invoices (id, tenant_id, order_id, number, fin_year, status, supply_type, place_of_supply,
                          buyer_snapshot, seller_snapshot, taxable_minor, igst_minor, round_off_minor, total_minor)
    VALUES (gen_random_uuid(), t_id, ord_B, 'VW/'||fy||'/000002', fy, 'issued', 'inter_state', '27',
            '{"name":"Diya Patel","place_of_supply":"27"}'::jsonb,
            '{"name":"Vidya Warrior Classes","gstin":"08AABCV1234C1Z5","place_of_supply":"08"}'::jsonb,
            466017, 83883, 0, 549900)
    RETURNING id INTO inv_B;
    INSERT INTO invoice_line_items (tenant_id, invoice_id, description, hsn_sac, qty, unit_minor, taxable_minor, rate_bps, igst_minor, total_minor)
    VALUES (t_id, inv_B, 'JEE Physics + Free Kinematics Combo', '999293', 1, 549900, 466017, 1800, 83883, 549900);
    UPDATE invoice_number_series SET next_seq = 3 WHERE tenant_id = t_id AND fin_year = fy;

    -- ========================================================================
    -- PHASE 12 — SUBSCRIPTION.  Student C takes All-Access Monthly.
    --   subscription(active) + order + payment + entitlement(subscription) + invoice.
    -- ========================================================================
    INSERT INTO orders (id, tenant_id, user_id, code, status, subtotal_minor, tax_minor, total_minor, gateway, gateway_order_id, place_of_supply, placed_at, paid_at)
    VALUES (gen_random_uuid(), t_id, u_stuC, 'VW-ORD-1003', 'paid', 99900, 0, 99900, 'razorpay', 'order_devseed1003', '08', now() - interval '5 days', now() - interval '5 days')
    RETURNING id INTO ord_C;
    INSERT INTO order_items (id, tenant_id, order_id, product_id, product_kind, title, hsn_sac, unit_minor, qty, line_subtotal_minor, taxable_minor, cgst_minor, sgst_minor, total_minor, grants_entitlement, entitlement_days)
    VALUES (gen_random_uuid(), t_id, ord_C, p_plan, 'plan', 'All-Access Monthly', '998431', 99900, 1, 99900, 84661, 7620, 7619, 99900, true, 30);
    INSERT INTO payments (id, tenant_id, order_id, user_id, gateway, gateway_payment_id, method, status, amount_minor, captured_at)
    VALUES (gen_random_uuid(), t_id, ord_C, u_stuC, 'razorpay', 'pay_devseed1003', 'netbanking', 'captured', 99900, now() - interval '5 days')
    RETURNING id INTO pay_C;
    INSERT INTO entitlements (id, tenant_id, user_id, product_id, product_kind, source, granted_at, expires_at)
    VALUES (gen_random_uuid(), t_id, u_stuC, p_plan, 'plan', 'subscription', now() - interval '5 days', now() + interval '25 days')
    RETURNING id INTO ent_C;
    INSERT INTO subscriptions (id, tenant_id, user_id, plan_id, status, current_period_start, current_period_end, trial_end, origin_order_id, latest_order_id, entitlement_id)
    VALUES (gen_random_uuid(), t_id, u_stuC, plan, 'active', now() - interval '5 days', now() + interval '25 days', now() - interval '2 days', ord_C, ord_C, ent_C)
    RETURNING id INTO sub_C;
    INSERT INTO invoices (id, tenant_id, order_id, number, fin_year, status, supply_type, place_of_supply, buyer_snapshot, seller_snapshot, taxable_minor, cgst_minor, sgst_minor, round_off_minor, total_minor)
    VALUES (gen_random_uuid(), t_id, ord_C, 'VW/'||fy||'/000003', fy, 'issued', 'intra_state', '08',
            '{"name":"Rohan Mehta"}'::jsonb, '{"name":"Vidya Warrior Classes","gstin":"08AABCV1234C1Z5"}'::jsonb,
            84661, 7620, 7619, 0, 99900)
    RETURNING id INTO inv_C;
    INSERT INTO invoice_line_items (tenant_id, invoice_id, description, hsn_sac, qty, unit_minor, taxable_minor, rate_bps, cgst_minor, sgst_minor, total_minor)
    VALUES (t_id, inv_C, 'All-Access Monthly', '998431', 1, 99900, 84661, 1800, 7620, 7619, 99900);
    UPDATE invoice_number_series SET next_seq = 4 WHERE tenant_id = t_id AND fin_year = fy;

    -- ========================================================================
    -- PHASE 13 — FEE PLAN.  Student D is on the 3-instalment fee plan;
    --   instalment 1 is paid (its own order + payment), 2 & 3 pending.
    -- ========================================================================
    INSERT INTO fee_accounts (id, tenant_id, user_id, fee_plan_id, course_id, batch_id, total_minor, paid_minor, status, due_on)
    VALUES (gen_random_uuid(), t_id, u_stuD, feeplan, c_paid, b_paid, 1500000, 500000, 'partial', current_date + 30)
    RETURNING id INTO fa_D;

    INSERT INTO orders (id, tenant_id, user_id, code, status, subtotal_minor, total_minor, gateway, gateway_order_id, place_of_supply, placed_at, paid_at)
    VALUES (gen_random_uuid(), t_id, u_stuD, 'VW-FEE-1001', 'paid', 500000, 500000, 'razorpay', 'order_devseedfee1', '08', now() - interval '8 days', now() - interval '8 days')
    RETURNING id INTO ord_D;
    INSERT INTO order_items (id, tenant_id, order_id, product_id, product_kind, title, unit_minor, qty, line_subtotal_minor, taxable_minor, total_minor, grants_entitlement)
    VALUES (gen_random_uuid(), t_id, ord_D, p_feeplan, 'fee_plan', 'JEE Physics fee — instalment 1/3', 500000, 1, 500000, 423729, 500000, false);
    INSERT INTO payments (id, tenant_id, order_id, user_id, gateway, gateway_payment_id, method, status, amount_minor, captured_at)
    VALUES (gen_random_uuid(), t_id, ord_D, u_stuD, 'razorpay', 'pay_devseedfee1', 'upi', 'captured', 500000, now() - interval '8 days')
    RETURNING id INTO pay_D;

    INSERT INTO fee_installments (id, tenant_id, fee_account_id, seq, amount_minor, due_on, status, paid_at, order_id) VALUES
        (gen_random_uuid(), t_id, fa_D, 1, 500000, current_date - 8,  'paid',    now() - interval '8 days', ord_D),
        (gen_random_uuid(), t_id, fa_D, 2, 500000, current_date + 22, 'pending', NULL, NULL),
        (gen_random_uuid(), t_id, fa_D, 3, 500000, current_date + 52, 'pending', NULL, NULL);

    -- entitlement + enrolment from the fee plan
    INSERT INTO entitlements (tenant_id, user_id, product_id, product_kind, source, granted_at)
    VALUES (t_id, u_stuD, p_course, 'course', 'fee_plan', now() - interval '8 days');
    INSERT INTO enrollments (tenant_id, user_id, course_id, batch_id, status, progress_bps, started_at)
    VALUES (t_id, u_stuD, c_paid, b_paid, 'active', 1200, now() - interval '8 days');

    -- Admin comps a scholarship student into the paid course — no order.
    INSERT INTO entitlements (tenant_id, user_id, product_id, product_kind, source, granted_at, created_by)
    VALUES (t_id, u_stuE, p_course, 'course', 'manual_grant', now() - interval '3 days', u_admin);
    INSERT INTO enrollments (tenant_id, user_id, course_id, batch_id, status, started_at)
    VALUES (t_id, u_stuE, c_paid, b_paid, 'active', now() - interval '3 days');

    -- ========================================================================
    -- PHASE 14 — COURSE GIFT.  The owner gifts the free course to student F,
    --   who redeems the code → entitlement(gift) + enrolment.
    -- ========================================================================
    INSERT INTO entitlements (id, tenant_id, user_id, product_id, product_kind, source, granted_at)
    VALUES (gen_random_uuid(), t_id, u_stuF, p_course_free, 'course', 'gift', now() - interval '2 days')
    RETURNING id INTO ent_F;
    INSERT INTO course_gifts (id, tenant_id, sender_id, product_id, recipient_phone, redemption_code, redeemed_by, redeemed_at, entitlement_id, message)
    VALUES (gen_random_uuid(), t_id, u_owner, p_course_free, '+919000100015',
            'GIFT-'||upper(substr(replace(gen_random_uuid()::text,'-',''),1,8)),
            u_stuF, now() - interval '2 days', ent_F, 'Welcome to Vidya Warrior!')
    RETURNING id INTO gift_F;
    INSERT INTO enrollments (tenant_id, user_id, course_id, entitlement_id, status, started_at)
    VALUES (t_id, u_stuF, c_free, ent_F, 'active', now() - interval '2 days');

    -- ========================================================================
    -- PHASE 15 — LEARNING ACTIVITY.  Progress, bookmarks, a graded test
    --   attempt, attendance, doubts (AI + instructor), an assignment
    --   submission, and a completion certificate.
    -- ========================================================================
    INSERT INTO content_progress (tenant_id, user_id, lesson_id, watched_sec, position_sec, completed_at, last_at) VALUES
        (t_id, u_stuA, les_video, 1840, 1840, now() - interval '3 days', now() - interval '3 days'),
        (t_id, u_stuA, les_doc,   120,  0,    NULL,                      now() - interval '3 days'),
        (t_id, u_stuB, les_video, 900,  900,  NULL,                      now() - interval '1 day');

    INSERT INTO lesson_bookmarks (tenant_id, user_id, lesson_id, position_sec, note) VALUES
        (t_id, u_stuA, les_video, 640, 'Revisit the 3-4-5 shortcut'),
        (t_id, u_stuB, les_link,  0,   'Play with launch angle');

    INSERT INTO test_attempts (id, tenant_id, test_id, user_id, attempt_no, status, score, max_score, correct_count, wrong_count, skipped_count, duration_sec, started_at, submitted_at, graded_at)
    VALUES (gen_random_uuid(), t_id, tst_dpp, u_stuA, 1, 'graded', 8, 12, 2, 1, 0, 740,
            now() - interval '3 days', now() - interval '3 days' + interval '12 min', now() - interval '3 days' + interval '12 min')
    RETURNING id INTO att_A;
    INSERT INTO test_responses (tenant_id, attempt_id, question_id, selected_option_ids, is_correct, marks, time_sec) VALUES
        (t_id, att_A, q_mcq1, ARRAY[(SELECT id FROM question_options WHERE question_id=q_mcq1 AND is_correct)]::uuid[], true, 4, 60),
        (t_id, att_A, q_mcq2, ARRAY[(SELECT id FROM question_options WHERE question_id=q_mcq2 AND is_correct=false LIMIT 1)]::uuid[], false, -1, 90);
    INSERT INTO test_responses (tenant_id, attempt_id, question_id, numeric_answer, is_correct, marks, time_sec)
    VALUES (t_id, att_A, q_num, 12.5, true, 4, 120);

    INSERT INTO attendance (tenant_id, user_id, session_id, batch_id, status, join_time, leave_time, watched_sec, is_auto, method, marked_by) VALUES
        (t_id, u_stuA, ls_ended, b_paid, 'present', now() - interval '2 days', now() - interval '2 days' + interval '90 min', 5400, true, 'auto', NULL),
        (t_id, u_stuB, ls_ended, b_paid, 'late',    now() - interval '2 days' + interval '20 min', now() - interval '2 days' + interval '90 min', 4200, true, 'auto', NULL),
        (t_id, u_stuD, ls_ended, b_paid, 'absent',  NULL, NULL, 0, false, 'manual', u_inst_phys);

    INSERT INTO doubts (id, tenant_id, user_id, lesson_id, topic_id, question_text, input_type, status)
    VALUES (gen_random_uuid(), t_id, u_stuA, les_video, tp_disp, 'Why is displacement a vector but distance a scalar?', 'text', 'resolved')
    RETURNING id INTO thr1;
    INSERT INTO doubt_answers (tenant_id, doubt_id, answer_text, answer_type, answered_by, is_accepted, model_name) VALUES
        (t_id, thr1, 'Displacement has direction (initial→final), distance is just path length.', 'ai', NULL, false, 'claude-sonnet'),
        (t_id, thr1, 'Exactly — and displacement can be zero on a closed loop while distance is not.', 'instructor', u_inst_phys, true, NULL);

    INSERT INTO assignments (id, tenant_id, course_id, batch_id, lesson_id, title, description, due_at, max_marks, status, created_by)
    VALUES (gen_random_uuid(), t_id, c_paid, b_paid, les_assign, 'Thermodynamics — 10 numericals',
            'Solve Q1–Q10 from the module sheet; upload a scan.', now() + interval '3 days', 20, 'published', u_inst_phys)
    RETURNING id INTO asg1;
    INSERT INTO assignment_submissions (tenant_id, assignment_id, user_id, submission_text, file_key, submitted_at, graded_at, marks_obtained, feedback, graded_by, status) VALUES
        (t_id, asg1, u_stuA, 'Attached my solutions.', 'vw/subs/aarav-thermo.pdf', now() - interval '1 day', now() - interval '6 hours', 17, 'Neat work, revise Q7.', u_inst_phys, 'graded'),
        (t_id, asg1, u_stuB, 'Done.', 'vw/subs/diya-thermo.pdf', now() - interval '20 hours', NULL, NULL, NULL, NULL, 'submitted');

    INSERT INTO certificates (tenant_id, user_id, course_id, serial, status, issued_at, pdf_key)
    VALUES (t_id, u_stuB, c_free, 'VW-CERT-'||upper(substr(replace(gen_random_uuid()::text,'-',''),1,10)),
            'issued', now() - interval '1 day', 'vw/certs/diya-kinematics.pdf');

    -- ========================================================================
    -- PHASE 16 — ENGAGEMENT.  Reviews, forum, in-course chat, gamification.
    -- ========================================================================
    INSERT INTO course_reviews (tenant_id, course_id, user_id, rating, body) VALUES
        (t_id, c_paid, u_stuA, 5, 'Best physics teaching I''ve had. DPPs are gold.'),
        (t_id, c_paid, u_stuB, 4, 'Great content, wish the live classes were earlier in the day.'),
        (t_id, c_free, u_stuF, 5, 'Perfect free intro — signed up for the full course after.');

    INSERT INTO forum_threads (id, tenant_id, course_id, user_id, title, body, reply_count, last_reply_at)
    VALUES (gen_random_uuid(), t_id, c_paid, u_stuC, 'Doubt: sign convention in projectile problems',
            'When do we take g as +9.8 vs -9.8?', 2, now() - interval '5 hours')
    RETURNING id INTO thr1;
    INSERT INTO forum_posts (tenant_id, thread_id, user_id, body, is_instructor_highlight) VALUES
        (t_id, thr1, u_stuA, 'I always take up as positive, so g = -9.8.', false),
        (t_id, thr1, u_inst_phys, 'Pick a convention and stay consistent within the problem. Up-positive ⇒ g negative.', true);

    INSERT INTO course_chat_messages (tenant_id, course_id, user_id, body) VALUES
        (t_id, c_paid, u_stuB, 'Is today''s live recorded?'),
        (t_id, c_paid, u_inst_chem, 'Yes, uploaded within an hour of ending.');

    INSERT INTO badges (name, code, description, icon, points) VALUES
        ('First Steps', 'first_steps', 'Completed your first lesson', '🎯', 10) RETURNING id INTO badge_first;
    INSERT INTO badges (name, code, description, icon, points) VALUES
        ('7-Day Streak', 'streak_7', 'Studied 7 days in a row', '🔥', 50) RETURNING id INTO badge_streak;
    INSERT INTO badge_grants (tenant_id, user_id, badge_id, earned_at) VALUES
        (t_id, u_stuA, badge_first,  now() - interval '3 days'),
        (t_id, u_stuA, badge_streak, now() - interval '1 hour'),
        (t_id, u_stuB, badge_first,  now() - interval '1 day');

    INSERT INTO learning_streaks (tenant_id, user_id, last_active_date, current_streak, longest_streak, total_points) VALUES
        (t_id, u_stuA, current_date, 7, 9, 260),
        (t_id, u_stuB, current_date - 1, 2, 4, 60);

    INSERT INTO wishlists (tenant_id, user_id, course_id) VALUES
        (t_id, u_stuC, c_paid),
        (t_id, u_stuE, c_paid);

    -- ========================================================================
    -- PHASE 17 — REFERRAL REWARD + PAYOUTS.  Student E signed up on student
    --   A's referral link and then bought → A gets a ₹500 wallet credit.
    --   Instructor payout for their revenue share is queued.
    -- ========================================================================
    UPDATE tenant_users SET invited_by = u_stuA WHERE tenant_id = t_id AND user_id = u_stuE;
    SELECT id INTO wal_A FROM wallets WHERE tenant_id = t_id AND user_id = u_stuA;

    INSERT INTO wallet_transactions (id, tenant_id, wallet_id, user_id, amount_minor, kind, ref_type, balance_after_minor, note)
    VALUES (gen_random_uuid(), t_id, wal_A, u_stuA, 50000, 'referral_reward', 'referral_event', 50000, 'Referred Kabir Khan')
    RETURNING id INTO wtx_A;
    UPDATE wallets SET balance_minor = 50000 WHERE id = wal_A;

    INSERT INTO referral_events (id, tenant_id, code, referrer_user_id, referred_user_id, status, reward_minor, qualifying_order_id, wallet_transaction_id, rewarded_at)
    VALUES (gen_random_uuid(), t_id, 'AARAV50', u_stuA, u_stuE, 'rewarded', 50000, NULL, wtx_A, now() - interval '1 hour')
    RETURNING id INTO refev;
    UPDATE referral_codes SET uses = uses + 1 WHERE tenant_id = t_id AND user_id = u_stuA;

    INSERT INTO payouts (id, tenant_id, payee_user_id, kind, amount_minor, tds_minor, net_minor, status, method, note, requested_at)
    VALUES (gen_random_uuid(), t_id, u_inst_phys, 'instructor_revshare', 269946, 26995, 242951, 'processing', 'bank_transfer',
            '60% rev-share on VW-ORD-1001', now() - interval '2 days')
    RETURNING id INTO payout1;
    INSERT INTO payout_items (tenant_id, payout_id, ref_type, ref_id, amount_minor)
    VALUES (t_id, payout1, 'order_item', oi_A, 269946);

    -- ========================================================================
    -- PHASE 18 — COMMUNICATION.  In-app notifications (one read) + per-channel
    --   deliveries, tenant + batch announcements, a 2-way WhatsApp thread.
    -- ========================================================================
    INSERT INTO notifications (id, tenant_id, user_id, template_key, title, body, data, entity_type, entity_id, read_at) VALUES
        (gen_random_uuid(), t_id, u_stuA, 'order.paid', 'Payment received', 'Your enrolment in JEE Physics 2027 is active.',
         '{"order_code":"VW-ORD-1001"}'::jsonb, 'order', ord_A, now() - interval '9 days' + interval '2 min'),
        (gen_random_uuid(), t_id, u_stuA, 'live.starting', 'Live class in 15 min', 'Newton''s laws recap starts at 6 PM.',
         '{}'::jsonb, 'live_session', ls_ended, NULL);
    SELECT id INTO notif1 FROM notifications WHERE tenant_id = t_id AND user_id = u_stuA AND template_key = 'live.starting';
    INSERT INTO notification_deliveries (tenant_id, notification_id, channel, status, provider_message_id, sent_at, delivered_at) VALUES
        (t_id, notif1, 'push',  'delivered', 'fcm-msg-'||gen_random_uuid(), now() - interval '2 days', now() - interval '2 days'),
        (t_id, notif1, 'sms',   'sent',      'msg91-'||gen_random_uuid(),   now() - interval '2 days', NULL);

    INSERT INTO announcements (tenant_id, created_by, title, body, priority, published_at) VALUES
        (t_id, u_admin, 'Diwali break', 'No live classes 20–23 Oct. Recordings stay available.', 'high', now() - interval '2 days');
    INSERT INTO announcements (tenant_id, created_by, course_id, batch_id, title, body, published_at) VALUES
        (t_id, u_inst_phys, c_paid, b_paid, 'DPP-4 released', 'Due Sunday 11 PM. Covers projectile + relative motion.', now() - interval '1 day');

    INSERT INTO messaging_threads (id, tenant_id, user_id, channel, phone, last_message_at, unread_count)
    VALUES (gen_random_uuid(), t_id, u_stuF, 'whatsapp', '+919000100015', now() - interval '3 hours', 1)
    RETURNING id INTO mthr1;
    INSERT INTO messaging_messages (tenant_id, thread_id, direction, body, status, provider_id) VALUES
        (t_id, mthr1, 'outbound', 'Hi Ananya! Your free course is ready. Reply YES for the full-course offer.', 'delivered', 'gupshup-out-1'),
        (t_id, mthr1, 'inbound',  'YES please send details', 'delivered', 'gupshup-in-1');

    -- ========================================================================
    -- PHASE 19 — PLATFORM-OPS LEFTOVERS.  Homepage banner, a background job,
    --   an active refresh-token session, a consumed OTP, and a final audit row.
    -- ========================================================================
    INSERT INTO banners (tenant_id, title, subtitle, image_url, link_type, link_id, link_url, display_order, is_active, created_by)
    VALUES (t_id, 'JEE Physics 2027 — enrol now', 'Live + recorded, weekly mocks, ₹4,999',
            'vw/banners/jee-physics.jpg', 'course', c_paid, NULL, 1, true, u_admin);

    INSERT INTO jobs (kind, tenant_id, payload, status, run_after) VALUES
        ('materialise_schedule', t_id, jsonb_build_object('schedule_id', sched1), 'pending', now() + interval '1 hour');

    INSERT INTO refresh_tokens (user_id, tenant_id, family_id, token_hash, user_agent, ip, expires_at)
    VALUES (u_stuA, t_id, gen_random_uuid(), encode(digest('seed-token-aarav','sha256'),'hex'),
            'VidyaWarrior/1.4.0 (Android 14)', '203.0.113.10', now() + interval '7 days');

    INSERT INTO otp_codes (channel, purpose, destination, code_hash, attempts, consumed_at, expires_at, ip)
    VALUES ('sms', 'login', '+919000100010', encode(digest('123456','sha256'),'hex'), 1,
            now() - interval '9 days', now() - interval '9 days' + interval '5 min', '203.0.113.10');

    INSERT INTO audit_logs (tenant_id, actor_user_id, actor_role, action, entity_type, entity_id, after)
    VALUES (t_id, u_admin, 'admin', 'refund.processed', 'refund', ref_A, jsonb_build_object('amount_minor', 50000, 'reason', 'goodwill'));
    INSERT INTO audit_logs (tenant_id, actor_user_id, actor_role, action, entity_type, entity_id, after)
    VALUES (NULL, super_admin, 'super_admin', 'tenant.plan_changed', 'tenant', t_id, jsonb_build_object('plan', 'pro'));

    RAISE NOTICE '0133: seeded tenant VWSTUDY (%). Logins: owner +919000100001, admin +919000100002, instructors +919000100003/4, students +919000100010..15. Dev OTP 123456.', t_id;
END $$;
