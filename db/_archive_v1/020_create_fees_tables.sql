-- +migrate Up
CREATE TABLE IF NOT EXISTS fee_structures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,
    batch_id UUID REFERENCES batches(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    total_amount NUMERIC(10,2) NOT NULL,
    currency VARCHAR(10) DEFAULT 'INR',
    installments_count INTEGER DEFAULT 1,
    installment_gap_days INTEGER DEFAULT 30,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_fee_struct_course ON fee_structures(course_id);
CREATE INDEX IF NOT EXISTS idx_fee_struct_batch ON fee_structures(batch_id);

CREATE TABLE IF NOT EXISTS student_fees (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    fee_structure_id UUID REFERENCES fee_structures(id) ON DELETE SET NULL,
    course_id UUID REFERENCES courses(id) ON DELETE SET NULL,
    batch_id UUID REFERENCES batches(id) ON DELETE SET NULL,
    total_amount NUMERIC(10,2) NOT NULL,
    paid_amount NUMERIC(10,2) DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'INR',
    status VARCHAR(20) DEFAULT 'pending',
    due_date DATE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_student_fees_user ON student_fees(user_id);
CREATE INDEX IF NOT EXISTS idx_student_fees_course ON student_fees(course_id);
CREATE INDEX IF NOT EXISTS idx_student_fees_status ON student_fees(status);
CREATE INDEX IF NOT EXISTS idx_student_fees_due ON student_fees(due_date);

CREATE TABLE IF NOT EXISTS fee_installments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_fee_id UUID NOT NULL REFERENCES student_fees(id) ON DELETE CASCADE,
    installment_number INTEGER NOT NULL,
    amount NUMERIC(10,2) NOT NULL,
    due_date DATE,
    paid_at TIMESTAMP,
    payment_id UUID REFERENCES payments(id) ON DELETE SET NULL,
    status VARCHAR(20) DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_installments_fee ON fee_installments(student_fee_id);
CREATE INDEX IF NOT EXISTS idx_installments_due ON fee_installments(due_date);

ALTER TABLE payments ADD COLUMN IF NOT EXISTS student_fee_id UUID REFERENCES student_fees(id) ON DELETE SET NULL;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS fee_installment_id UUID REFERENCES fee_installments(id) ON DELETE SET NULL;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS purpose VARCHAR(30) DEFAULT 'subscription';
CREATE INDEX IF NOT EXISTS idx_payments_fee ON payments(student_fee_id);
CREATE INDEX IF NOT EXISTS idx_payments_installment ON payments(fee_installment_id);
