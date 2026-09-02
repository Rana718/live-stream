-- 044_seed_curriculum_content.sql
-- Populates real curriculum content under the Vidya Warrior tenant
-- (RANJAN24) so the student/instructor UI has something to render:
-- an instructor user, one batch + enrollment per course, a 3-chapter /
-- 2-topic-per-chapter tree with a lecture + material per topic, one
-- test with two MCQ questions per course, and one assignment per course.
--
-- Idempotent per course: if a course already has subjects, its whole
-- subtree is skipped, so re-running this migration (or re-running
-- migrate.sh against a partially-seeded DB) is a no-op.

DO $$
DECLARE
    v_tenant_id     uuid := 'aaaaaaaa-bbbb-cccc-dddd-000000000001';
    v_instructor_id uuid := 'aaaaaaaa-bbbb-cccc-dddd-000000000a03';
    v_student_id    uuid := 'aaaaaaaa-bbbb-cccc-dddd-000000000a02';
    v_course        record;
    v_batch_id      uuid;
    v_subject_id    uuid;
    v_chapter_id    uuid;
    v_topic_id      uuid;
    v_test_id       uuid;
    v_question_id   uuid;
    ch_idx          int;
    topic_idx       int;
BEGIN
    -- Instructor for the tenant. Same shape/style as the admin + student
    -- rows in 038_seed_vidya_warrior.sql.
    INSERT INTO users (id, tenant_id, full_name, phone_number, phone_verified, role, is_active, auth_method)
    VALUES (v_instructor_id, v_tenant_id, 'Priya Ma''am', '+919999900003', TRUE, 'instructor', TRUE, 'phone')
    ON CONFLICT (id) DO UPDATE
        SET full_name      = EXCLUDED.full_name,
            phone_number   = EXCLUDED.phone_number,
            phone_verified = EXCLUDED.phone_verified,
            role           = EXCLUDED.role,
            is_active      = EXCLUDED.is_active;

    FOR v_course IN SELECT id, title FROM courses WHERE tenant_id = v_tenant_id ORDER BY created_at LOOP
        IF EXISTS (SELECT 1 FROM subjects WHERE course_id = v_course.id) THEN
            CONTINUE;
        END IF;

        INSERT INTO batches (id, tenant_id, course_id, name, instructor_id, start_date, max_students, current_students, is_active)
        VALUES (gen_random_uuid(), v_tenant_id, v_course.id, v_course.title || ' - Batch A', v_instructor_id, CURRENT_DATE, 60, 1, TRUE)
        RETURNING id INTO v_batch_id;

        INSERT INTO enrollments (id, tenant_id, user_id, course_id, batch_id, status, progress_percent, enrolled_at)
        VALUES (gen_random_uuid(), v_tenant_id, v_student_id, v_course.id, v_batch_id, 'active', 15, CURRENT_TIMESTAMP)
        ON CONFLICT DO NOTHING;

        INSERT INTO subjects (id, tenant_id, course_id, name, description, display_order)
        VALUES (gen_random_uuid(), v_tenant_id, v_course.id, v_course.title, 'Core syllabus for ' || v_course.title, 1)
        RETURNING id INTO v_subject_id;

        FOR ch_idx IN 1..3 LOOP
            INSERT INTO chapters (id, tenant_id, subject_id, name, description, display_order, is_free)
            VALUES (gen_random_uuid(), v_tenant_id, v_subject_id, 'Chapter ' || ch_idx,
                    'Chapter ' || ch_idx || ' of ' || v_course.title, ch_idx, ch_idx = 1)
            RETURNING id INTO v_chapter_id;

            FOR topic_idx IN 1..2 LOOP
                INSERT INTO topics (id, tenant_id, chapter_id, name, description, display_order, is_free)
                VALUES (gen_random_uuid(), v_tenant_id, v_chapter_id, 'Topic ' || ch_idx || '.' || topic_idx,
                        'Topic ' || topic_idx || ' under Chapter ' || ch_idx, topic_idx, ch_idx = 1)
                RETURNING id INTO v_topic_id;

                INSERT INTO lectures (id, tenant_id, topic_id, chapter_id, subject_id, title, description,
                                       lecture_type, instructor_id, duration_seconds, language,
                                       is_free, is_published, display_order)
                VALUES (gen_random_uuid(), v_tenant_id, v_topic_id, v_chapter_id, v_subject_id,
                        'Lecture ' || ch_idx || '.' || topic_idx || ' — ' || v_course.title,
                        'Recorded walkthrough covering Topic ' || ch_idx || '.' || topic_idx || '.',
                        'recorded', v_instructor_id, 1800, 'en', ch_idx = 1, TRUE, 1);

                INSERT INTO study_materials (id, tenant_id, topic_id, chapter_id, subject_id, title, description,
                                              material_type, file_path, language, is_free, uploaded_by)
                VALUES (gen_random_uuid(), v_tenant_id, v_topic_id, v_chapter_id, v_subject_id,
                        'Notes — Chapter ' || ch_idx || '.' || topic_idx,
                        'Printable notes for Topic ' || ch_idx || '.' || topic_idx || '.',
                        'pdf', 'seed/materials/placeholder-notes.pdf', 'en', ch_idx = 1, v_instructor_id);
            END LOOP;
        END LOOP;

        INSERT INTO tests (id, tenant_id, course_id, subject_id, title, description, test_type,
                            duration_minutes, total_marks, passing_marks, negative_marking,
                            is_free, is_published, created_by)
        VALUES (gen_random_uuid(), v_tenant_id, v_course.id, v_subject_id,
                v_course.title || ' — Chapter 1 Test', 'Quick topic test covering Chapter 1.',
                'topic_test', 20, 10, 4, TRUE, TRUE, TRUE, v_instructor_id)
        RETURNING id INTO v_test_id;

        FOR topic_idx IN 1..2 LOOP
            INSERT INTO questions (id, tenant_id, test_id, question_text, question_type,
                                    marks, negative_marks, difficulty, explanation, display_order)
            VALUES (gen_random_uuid(), v_tenant_id, v_test_id,
                    'Sample question ' || topic_idx || ' for ' || v_course.title || '?',
                    'mcq', 5, 1, 'easy', 'Explanation for sample question ' || topic_idx || '.', topic_idx)
            RETURNING id INTO v_question_id;

            INSERT INTO question_options (id, tenant_id, question_id, option_text, is_correct, display_order)
            VALUES
                (gen_random_uuid(), v_tenant_id, v_question_id, 'Option A', topic_idx = 1, 1),
                (gen_random_uuid(), v_tenant_id, v_question_id, 'Option B', FALSE, 2),
                (gen_random_uuid(), v_tenant_id, v_question_id, 'Option C', topic_idx = 2, 3),
                (gen_random_uuid(), v_tenant_id, v_question_id, 'Option D', FALSE, 4);
        END LOOP;

        INSERT INTO assignments (id, tenant_id, batch_id, course_id, title, description,
                                  due_date, max_marks, is_published, created_by)
        VALUES (gen_random_uuid(), v_tenant_id, v_batch_id, v_course.id,
                v_course.title || ' — Practice Set 1',
                'Solve the attached practice set and submit your answers.',
                CURRENT_TIMESTAMP + INTERVAL '7 days', 50, TRUE, v_instructor_id);
    END LOOP;
END $$;
