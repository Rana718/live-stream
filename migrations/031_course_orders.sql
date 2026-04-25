-- 031_course_orders.sql
-- Direct course purchase: lets a student buy a single course outside the
-- subscription/fees flows (Phase 3 of the Classplus retrofit). Razorpay
-- order ID is the idempotency key.
--
-- We extend the existing payments table with a course_id pointer rather
-- than create a parallel "orders" table — the metadata + amount + status
-- columns we need already exist there.

ALTER TABLE payments ADD COLUMN IF NOT EXISTS course_id UUID
    REFERENCES courses(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_payments_course ON payments(course_id);

-- Composite index for the common "did this user buy this course" lookup.
CREATE INDEX IF NOT EXISTS idx_payments_user_course_status
    ON payments(user_id, course_id, status);
