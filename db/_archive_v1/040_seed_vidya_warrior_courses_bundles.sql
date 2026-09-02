-- 040_seed_vidya_warrior_courses_bundles.sql
-- Seeds sample courses + two bundle deals on the Vidya Warrior tenant
-- (org code RANJAN24) so the new course-based store has something to
-- render on first login. Idempotent — re-runs upsert on the natural
-- keys (slug for courses, title for bundles within the same tenant).

-- Stable IDs so subsequent migrations / seeds can reference them.
-- Pattern: aaaa…bbbb…cccc…dddd…<role>NN
-- where role is c1..c5 for courses and bN for bundles.

INSERT INTO courses (id, tenant_id, title, slug, description, price, language, is_published)
VALUES
    ('aaaaaaaa-bbbb-cccc-dddd-0000000c0001',
     'aaaaaaaa-bbbb-cccc-dddd-000000000001',
     'Hindi Grammar — Foundation',
     'vw-hindi-grammar-foundation',
     '60 lectures + DPP + topic-wise tests. For class 9-10 board exams and SSC aspirants.',
     999.00,
     'hi',
     TRUE),
    ('aaaaaaaa-bbbb-cccc-dddd-0000000c0002',
     'aaaaaaaa-bbbb-cccc-dddd-000000000001',
     'English Grammar — SSC CGL',
     'vw-english-grammar-ssc',
     'Vocab, error spotting, cloze passages. PYQ-driven, last 5 years covered.',
     999.00,
     'en',
     TRUE),
    ('aaaaaaaa-bbbb-cccc-dddd-0000000c0003',
     'aaaaaaaa-bbbb-cccc-dddd-000000000001',
     'Quantitative Aptitude — Banking',
     'vw-quant-banking',
     'IBPS / SBI PO / clerk patterns. Daily live solving + recorded explainers.',
     1499.00,
     'en',
     TRUE),
    ('aaaaaaaa-bbbb-cccc-dddd-0000000c0004',
     'aaaaaaaa-bbbb-cccc-dddd-000000000001',
     'Reasoning — Verbal & Non-Verbal',
     'vw-reasoning-vnv',
     'Sequences, syllogisms, blood relations, puzzles. 200+ solved variants.',
     1299.00,
     'en',
     TRUE),
    ('aaaaaaaa-bbbb-cccc-dddd-0000000c0005',
     'aaaaaaaa-bbbb-cccc-dddd-000000000001',
     'Indian Polity — UPSC Foundation',
     'vw-polity-upsc',
     'Constitution, parliament, judiciary. Maps prelims + mains GS-2 syllabus.',
     1799.00,
     'en',
     TRUE)
ON CONFLICT (slug) DO UPDATE
    SET title        = EXCLUDED.title,
        description  = EXCLUDED.description,
        price        = EXCLUDED.price,
        language     = EXCLUDED.language,
        is_published = EXCLUDED.is_published;

-- Bundle 1 — "any 2 grammar courses for less than the price of two".
-- Hindi + English = ₹1998 individually, bundle ₹1599 → save ₹399.
INSERT INTO course_bundles (id, tenant_id, title, description, price_paise, display_order, is_active)
VALUES (
    'aaaaaaaa-bbbb-cccc-dddd-0000000b0001',
    'aaaaaaaa-bbbb-cccc-dddd-000000000001',
    'Grammar Combo · 2 courses',
    'Hindi + English grammar at a single price. Best for class 9-10 students prepping for SSC.',
    159900,
    10,
    TRUE
)
ON CONFLICT (id) DO UPDATE
    SET title         = EXCLUDED.title,
        description   = EXCLUDED.description,
        price_paise   = EXCLUDED.price_paise,
        display_order = EXCLUDED.display_order,
        is_active     = EXCLUDED.is_active;

INSERT INTO course_bundle_items (bundle_id, course_id) VALUES
    ('aaaaaaaa-bbbb-cccc-dddd-0000000b0001', 'aaaaaaaa-bbbb-cccc-dddd-0000000c0001'),
    ('aaaaaaaa-bbbb-cccc-dddd-0000000b0001', 'aaaaaaaa-bbbb-cccc-dddd-0000000c0002')
ON CONFLICT (bundle_id, course_id) DO NOTHING;

-- Bundle 2 — "banking-exam triple" — Quant + Reasoning + English.
-- Individually ₹1499 + ₹1299 + ₹999 = ₹3797. Bundle ₹2999 → save ₹798.
INSERT INTO course_bundles (id, tenant_id, title, description, price_paise, display_order, is_active)
VALUES (
    'aaaaaaaa-bbbb-cccc-dddd-0000000b0002',
    'aaaaaaaa-bbbb-cccc-dddd-000000000001',
    'Banking Triple · 3 courses',
    'Quant + Reasoning + English. Targets IBPS/SBI prelims + mains. Save ~25% over buying individually.',
    299900,
    20,
    TRUE
)
ON CONFLICT (id) DO UPDATE
    SET title         = EXCLUDED.title,
        description   = EXCLUDED.description,
        price_paise   = EXCLUDED.price_paise,
        display_order = EXCLUDED.display_order,
        is_active     = EXCLUDED.is_active;

INSERT INTO course_bundle_items (bundle_id, course_id) VALUES
    ('aaaaaaaa-bbbb-cccc-dddd-0000000b0002', 'aaaaaaaa-bbbb-cccc-dddd-0000000c0003'),
    ('aaaaaaaa-bbbb-cccc-dddd-0000000b0002', 'aaaaaaaa-bbbb-cccc-dddd-0000000c0004'),
    ('aaaaaaaa-bbbb-cccc-dddd-0000000b0002', 'aaaaaaaa-bbbb-cccc-dddd-0000000c0002')
ON CONFLICT (bundle_id, course_id) DO NOTHING;
