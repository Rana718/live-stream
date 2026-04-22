-- Demo seed data for local development.
-- Idempotent: every block uses ON CONFLICT or WHERE NOT EXISTS so it's safe to
-- re-run.

BEGIN;

-- =============================================================
-- Pick an instructor + a student for ownership fields. Creates
-- them if they don't already exist. Password hashes are bcrypt for 'pass1234'.
-- =============================================================
INSERT INTO users (id, email, username, password_hash, full_name, role)
VALUES
  ('00000000-0000-0000-0000-000000000001', 'instructor@aadesh.local', 'instructor',
   '$2a$10$nhotSKfP8IyE5itnRD84pu7QpNFrg5V8pCZH1fp1UHhUtQalGmwSG',
   'Anita Sharma', 'instructor'),
  ('00000000-0000-0000-0000-000000000002', 'admin@aadesh.local', 'admin1',
   '$2a$10$nhotSKfP8IyE5itnRD84pu7QpNFrg5V8pCZH1fp1UHhUtQalGmwSG',
   'Admin User', 'admin'),
  ('00000000-0000-0000-0000-000000000003', 'student@aadesh.local', 'student1',
   '$2a$10$nhotSKfP8IyE5itnRD84pu7QpNFrg5V8pCZH1fp1UHhUtQalGmwSG',
   'Rahul Verma', 'student')
ON CONFLICT (email) DO NOTHING;

-- =============================================================
-- COURSES (pick an exam category slug that exists in seed)
-- =============================================================
INSERT INTO courses (id, exam_category_id, title, slug, description, thumbnail_url,
                     price, discounted_price, duration_months, language, level,
                     is_free, is_published, created_by, approval_status)
SELECT
  id_gen, ec.id, t, slug_t, descr_t, thumb_t, price_t, disc_t, months_t,
  lang_t, level_t, free_t, TRUE, '00000000-0000-0000-0000-000000000001', 'approved'
FROM (VALUES
  (gen_random_uuid(), 'neet',    'NEET 2026 Dropper Batch',
   'neet-2026-dropper',
   'Comprehensive 12-month program for NEET 2026 with 1500+ hours of live classes, DPPs, test series and 24x7 doubt solving.',
   'https://images.unsplash.com/photo-1532094349884-543bc11b234d?w=800',
   4999, 2999, 12, 'en', 'advanced', FALSE),
  (gen_random_uuid(), 'jee',     'JEE Main + Advanced Crash Course',
   'jee-crash-course',
   '3-month intensive crash course with revision notes, Chapter-wise DPPs and 10 full-length mock tests.',
   'https://images.unsplash.com/photo-1635070041078-e363dbe005cb?w=800',
   1999, 1499, 3, 'en', 'advanced', FALSE),
  (gen_random_uuid(), 'school',  'Class 10 CBSE Foundation',
   'class-10-cbse',
   'Complete CBSE Class 10 Math + Science syllabus with chapter-wise notes, NCERT solutions and unit tests.',
   'https://images.unsplash.com/photo-1503676260728-1c00da094a0b?w=800',
   1299, 799, 10, 'en', 'foundation', FALSE),
  (gen_random_uuid(), 'upsc',    'UPSC Prelims Foundation',
   'upsc-prelims',
   'Static syllabus + current affairs with daily prelims-style MCQs and weekly tests.',
   'https://images.unsplash.com/photo-1589391886645-d51941baf7fb?w=800',
   6999, 4999, 12, 'en', 'advanced', FALSE),
  (gen_random_uuid(), 'school',  'Free Foundation Demo Class',
   'free-foundation-demo',
   'Try us out — this is a free sample course with 5 recorded lectures and one chapter test.',
   'https://images.unsplash.com/photo-1513258496099-48168024aec0?w=800',
   0, 0, 1, 'en', 'foundation', TRUE)
) AS x(id_gen, slug_ec, t, slug_t, descr_t, thumb_t, price_t, disc_t, months_t, lang_t, level_t, free_t)
JOIN exam_categories ec ON ec.slug = x.slug_ec
WHERE NOT EXISTS (SELECT 1 FROM courses WHERE slug = x.slug_t);

-- =============================================================
-- SUBJECTS per course
-- =============================================================
INSERT INTO subjects (course_id, name, description, display_order)
SELECT c.id, s.name, s.description, s.ord
FROM courses c
JOIN LATERAL (VALUES
  ('Physics',   'Mechanics, waves, optics, modern physics', 1),
  ('Chemistry', 'Physical, organic, inorganic chemistry',   2),
  ('Biology',   'Botany + Zoology fundamentals',             3),
  ('Mathematics', 'Algebra, calculus, coordinate geometry', 4)
) s(name, description, ord) ON TRUE
WHERE c.slug IN ('neet-2026-dropper','jee-crash-course','class-10-cbse','free-foundation-demo')
  AND NOT EXISTS (
    SELECT 1 FROM subjects s2 WHERE s2.course_id = c.id AND s2.name = s.name
  );

-- =============================================================
-- CHAPTERS per subject (generic 3 chapters each)
-- =============================================================
INSERT INTO chapters (subject_id, name, description, display_order, is_free)
SELECT s.id,
       'Chapter ' || x.num || ': ' || x.topic,
       'Core fundamentals of ' || x.topic || ' — video lessons, notes and practice questions.',
       x.num,
       (x.num = 1)  -- first chapter free
FROM subjects s
JOIN LATERAL (VALUES
  (1, 'Introduction & Basics'),
  (2, 'Core Concepts'),
  (3, 'Advanced Applications')
) x(num, topic) ON TRUE
WHERE NOT EXISTS (
  SELECT 1 FROM chapters c WHERE c.subject_id = s.id AND c.display_order = x.num
);

-- =============================================================
-- TOPICS per chapter (2 topics each)
-- =============================================================
INSERT INTO topics (chapter_id, name, description, display_order, is_free)
SELECT c.id, 'Topic ' || x.num || ' · ' || c.name, 'Sub-topic of ' || c.name, x.num,
       COALESCE(c.is_free, FALSE)
FROM chapters c
JOIN LATERAL (VALUES (1), (2)) x(num) ON TRUE
WHERE NOT EXISTS (
  SELECT 1 FROM topics t WHERE t.chapter_id = c.id AND t.display_order = x.num
);

-- =============================================================
-- LECTURES: 1 live + 2 recorded per chapter (first chapter only for brevity)
-- =============================================================
INSERT INTO lectures (chapter_id, subject_id, title, description, lecture_type,
                      instructor_id, scheduled_at, duration_seconds, is_free,
                      is_published, display_order)
SELECT c.id, c.subject_id,
       'Live: ' || c.name || ' — Walkthrough',
       'Interactive live class covering ' || c.name || ' with real-time Q&A.',
       'live',
       '00000000-0000-0000-0000-000000000001',
       CURRENT_TIMESTAMP + interval '2 hours',
       3600, TRUE, TRUE, 1
FROM chapters c
WHERE c.display_order = 1
  AND NOT EXISTS (
    SELECT 1 FROM lectures l WHERE l.chapter_id = c.id AND l.lecture_type = 'live'
  );

INSERT INTO lectures (chapter_id, subject_id, title, description, lecture_type,
                      instructor_id, duration_seconds, is_free, is_published, display_order)
SELECT c.id, c.subject_id,
       'Recorded: ' || c.name || ' — Part ' || x.num,
       'Pre-recorded lecture for ' || c.name || ' (Part ' || x.num || '). Watch anytime.',
       'recorded',
       '00000000-0000-0000-0000-000000000001',
       2400 + (x.num * 300),
       (c.is_free AND x.num = 1),
       TRUE, x.num + 1
FROM chapters c
JOIN LATERAL (VALUES (1), (2)) x(num) ON TRUE
WHERE NOT EXISTS (
  SELECT 1 FROM lectures l
  WHERE l.chapter_id = c.id
    AND l.lecture_type = 'recorded'
    AND l.display_order = x.num + 1
);

-- =============================================================
-- STUDY MATERIALS (one PDF per chapter)
-- =============================================================
INSERT INTO study_materials (chapter_id, subject_id, title, description,
                             material_type, file_path, file_size, language,
                             is_free, uploaded_by)
SELECT c.id, c.subject_id,
       'Notes · ' || c.name,
       'Revision notes PDF for ' || c.name,
       'pdf',
       'notes/' || c.id::text || '.pdf',
       250000,
       'en',
       (c.is_free IS TRUE),
       '00000000-0000-0000-0000-000000000001'
FROM chapters c
WHERE NOT EXISTS (
  SELECT 1 FROM study_materials m WHERE m.chapter_id = c.id AND m.material_type = 'pdf'
);

-- =============================================================
-- TESTS: one DPP and one chapter-test per chapter for the demo course
-- =============================================================
INSERT INTO tests (chapter_id, subject_id, course_id, title, description,
                   test_type, duration_minutes, total_marks, passing_marks,
                   negative_marking, is_free, is_published, created_by)
SELECT c.id, c.subject_id, s.course_id,
       'DPP · ' || c.name,
       'Daily Practice Problems for ' || c.name,
       'dpp', 20, 20, 8, FALSE, (c.is_free IS TRUE), TRUE,
       '00000000-0000-0000-0000-000000000001'
FROM chapters c
JOIN subjects s ON s.id = c.subject_id
WHERE NOT EXISTS (
  SELECT 1 FROM tests t WHERE t.chapter_id = c.id AND t.test_type = 'dpp'
);

INSERT INTO tests (chapter_id, subject_id, course_id, title, description,
                   test_type, duration_minutes, total_marks, passing_marks,
                   negative_marking, is_free, is_published, created_by)
SELECT c.id, c.subject_id, s.course_id,
       'Chapter Test · ' || c.name,
       'Chapter-level assessment for ' || c.name,
       'chapter', 45, 50, 20, TRUE, (c.is_free IS TRUE), TRUE,
       '00000000-0000-0000-0000-000000000001'
FROM chapters c
JOIN subjects s ON s.id = c.subject_id
WHERE NOT EXISTS (
  SELECT 1 FROM tests t WHERE t.chapter_id = c.id AND t.test_type = 'chapter'
);

-- Mock tests at course level
INSERT INTO tests (course_id, title, description, test_type, duration_minutes,
                   total_marks, passing_marks, negative_marking, is_free,
                   is_published, created_by)
SELECT c.id,
       'Mock Test · ' || c.title,
       'Full-length simulated exam for ' || c.title,
       'mock', 180, 200, 80, TRUE, FALSE, TRUE,
       '00000000-0000-0000-0000-000000000001'
FROM courses c
WHERE c.slug IN ('neet-2026-dropper','jee-crash-course')
  AND NOT EXISTS (
    SELECT 1 FROM tests t WHERE t.course_id = c.id AND t.test_type = 'mock'
  );

-- Previous Year Questions (tied to exam categories)
INSERT INTO tests (exam_category_id, title, description, test_type, exam_year,
                   duration_minutes, total_marks, passing_marks, negative_marking,
                   is_free, is_published, created_by)
SELECT ec.id,
       'NEET ' || y.year || ' — Full Paper',
       'Official NEET ' || y.year || ' paper re-hosted for practice.',
       'pyq', y.year, 180, 200, 80, TRUE, FALSE, TRUE,
       '00000000-0000-0000-0000-000000000001'
FROM exam_categories ec
JOIN (VALUES (2024), (2023), (2022)) y(year) ON TRUE
WHERE ec.slug = 'neet'
  AND NOT EXISTS (
    SELECT 1 FROM tests t WHERE t.exam_category_id = ec.id
                            AND t.test_type = 'pyq'
                            AND t.exam_year = y.year
  );

-- =============================================================
-- QUESTIONS + OPTIONS for each DPP/chapter test (3 sample MCQs each)
-- =============================================================
WITH new_questions AS (
  INSERT INTO questions (test_id, question_text, question_type, marks, difficulty,
                         explanation, display_order)
  SELECT t.id,
         x.q,
         'mcq', x.marks, x.difficulty, x.explanation, x.num
  FROM tests t
  JOIN LATERAL (VALUES
    (1, 'Which of the following is a scalar quantity?', 1, 'easy',
     'Scalars have magnitude only; vectors have direction too.'),
    (2, 'What is the SI unit of force?', 1, 'easy',
     'The SI unit of force is the Newton (kg·m/s²).'),
    (3, 'If acceleration is 2 m/s² and time is 3 s, find velocity (initial = 0).', 2, 'medium',
     'v = u + at = 0 + 2×3 = 6 m/s')
  ) x(num, q, marks, difficulty, explanation) ON TRUE
  WHERE t.test_type IN ('dpp','chapter')
    AND NOT EXISTS (
      SELECT 1 FROM questions q2 WHERE q2.test_id = t.id AND q2.display_order = x.num
    )
  RETURNING id, test_id, display_order
)
INSERT INTO question_options (question_id, option_text, is_correct, display_order)
SELECT q.id, opt.option_text, opt.is_correct, opt.ord
FROM new_questions q
JOIN LATERAL (
  SELECT * FROM (VALUES
    (1, 'Speed',       TRUE,  1),
    (1, 'Velocity',    FALSE, 2),
    (1, 'Acceleration',FALSE, 3),
    (1, 'Force',       FALSE, 4),
    (2, 'Joule',       FALSE, 1),
    (2, 'Newton',      TRUE,  2),
    (2, 'Pascal',      FALSE, 3),
    (2, 'Watt',        FALSE, 4),
    (3, '3 m/s',  FALSE, 1),
    (3, '6 m/s',  TRUE,  2),
    (3, '9 m/s',  FALSE, 3),
    (3, '12 m/s', FALSE, 4)
  ) t(qnum, option_text, is_correct, ord)
  WHERE t.qnum = q.display_order
) opt ON TRUE;

-- =============================================================
-- ANNOUNCEMENTS (global)
-- =============================================================
INSERT INTO announcements (title, body, priority, created_by)
SELECT x.title, x.body, x.priority, '00000000-0000-0000-0000-000000000002'
FROM (VALUES
  ('Welcome to Aadesh Academy! 🎉',
   'Your student journey starts here. Browse courses, start a free chapter, or take a DPP to warm up.',
   'normal'),
  ('NEET 2026 early-bird ends Sunday',
   '40% off on the NEET 2026 Dropper Batch. Valid till this weekend only.',
   'high'),
  ('Scheduled maintenance — 2am IST tonight',
   'The platform will be briefly unavailable from 2-3am IST for a planned upgrade.',
   'normal')
) x(title, body, priority)
WHERE NOT EXISTS (
  SELECT 1 FROM announcements a WHERE a.title = x.title
);

-- =============================================================
-- NOTIFICATIONS to the demo student
-- =============================================================
INSERT INTO notifications (user_id, type, title, body)
SELECT '00000000-0000-0000-0000-000000000003', x.type, x.title, x.body
FROM (VALUES
  ('announcement', 'Welcome!', 'Start with our free Foundation Demo Class.'),
  ('assignment',   'New assignment due',  'Kinematics DPP is due in 48 hours.'),
  ('test_result',  'Test result published','Your Chapter Test score is ready to view.'),
  ('doubt_answered','Your doubt was answered','Claude replied to your question on Newton''s Laws.'),
  ('fee_due',      'Fee reminder',        'Installment 1 of NEET 2026 is due in 5 days.')
) x(type, title, body)
WHERE NOT EXISTS (
  SELECT 1 FROM notifications n
  WHERE n.user_id = '00000000-0000-0000-0000-000000000003' AND n.title = x.title
);

-- =============================================================
-- FEE STRUCTURES for paid courses
-- =============================================================
INSERT INTO fee_structures (course_id, name, total_amount, currency,
                            installments_count, installment_gap_days, is_active)
SELECT c.id,
       c.title || ' — Fee',
       COALESCE(c.discounted_price, c.price),
       'INR',
       CASE WHEN COALESCE(c.discounted_price, c.price) > 3000 THEN 3 ELSE 1 END,
       30,
       TRUE
FROM courses c
WHERE COALESCE(c.discounted_price, c.price) > 0
  AND NOT EXISTS (
    SELECT 1 FROM fee_structures fs WHERE fs.course_id = c.id
  );

-- =============================================================
-- HOME BANNERS (extend the 2 seeded at migration time)
-- =============================================================
INSERT INTO banners (title, subtitle, image_url, background_color, link_type, display_order)
SELECT x.title, x.subtitle, x.image_url, x.bg, x.link_type, x.ord
FROM (VALUES
  ('JEE Main Crash Course · 25% off',
   'Master all chapters in 3 months with daily tests and live doubt sessions.',
   'https://images.unsplash.com/photo-1507842217343-583bb7270b66?w=1200',
   '#FFE8DA', 'none', 3),
  ('Ask Claude anytime',
   '24×7 AI doubt solving in English, Hindi and Hinglish.',
   'https://images.unsplash.com/photo-1555421689-491a97ff2040?w=1200',
   '#EEE8FB', 'none', 4)
) x(title, subtitle, image_url, bg, link_type, ord)
WHERE NOT EXISTS (
  SELECT 1 FROM banners b WHERE b.title = x.title
);

COMMIT;
