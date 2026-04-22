-- Profile fields used by the app's post-signup onboarding flow.
-- class_level: '1'..'12' or 'other' (keeps rows valid for non-K12 learners).
-- board:       'cbse' | 'icse' | 'bihar' | 'state' | 'other'.
-- exam_goal:   'school' | 'jee_main' | 'neet' | 'ssc' | 'other'.
-- onboarding_completed gates app access on first login.
ALTER TABLE users ADD COLUMN IF NOT EXISTS class_level VARCHAR(20);
ALTER TABLE users ADD COLUMN IF NOT EXISTS board VARCHAR(30);
ALTER TABLE users ADD COLUMN IF NOT EXISTS exam_goal VARCHAR(30);
ALTER TABLE users ADD COLUMN IF NOT EXISTS onboarding_completed BOOLEAN DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_users_class_level ON users(class_level);
CREATE INDEX IF NOT EXISTS idx_users_exam_goal ON users(exam_goal);

-- Mirror the same tags on courses so the discovery feed can be filtered to
-- what each student actually needs. NULL on a course means "universal" — show
-- it to anyone regardless of their class_level / exam_goal.
ALTER TABLE courses ADD COLUMN IF NOT EXISTS class_level VARCHAR(20);
ALTER TABLE courses ADD COLUMN IF NOT EXISTS exam_goal VARCHAR(30);

CREATE INDEX IF NOT EXISTS idx_courses_class_level ON courses(class_level);
CREATE INDEX IF NOT EXISTS idx_courses_exam_goal ON courses(exam_goal);
