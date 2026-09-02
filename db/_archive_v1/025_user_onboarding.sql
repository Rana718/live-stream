ALTER TABLE users ADD COLUMN IF NOT EXISTS class_level VARCHAR(20);
ALTER TABLE users ADD COLUMN IF NOT EXISTS board VARCHAR(30);
ALTER TABLE users ADD COLUMN IF NOT EXISTS exam_goal VARCHAR(30);
ALTER TABLE users ADD COLUMN IF NOT EXISTS onboarding_completed BOOLEAN DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_users_class_level ON users(class_level);
CREATE INDEX IF NOT EXISTS idx_users_exam_goal ON users(exam_goal);

ALTER TABLE courses ADD COLUMN IF NOT EXISTS class_level VARCHAR(20);
ALTER TABLE courses ADD COLUMN IF NOT EXISTS exam_goal VARCHAR(30);

CREATE INDEX IF NOT EXISTS idx_courses_class_level ON courses(class_level);
CREATE INDEX IF NOT EXISTS idx_courses_exam_goal ON courses(exam_goal);
