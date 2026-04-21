-- name: CreateFeeStructure :one
INSERT INTO fee_structures (course_id, batch_id, name, total_amount, currency,
                            installments_count, installment_gap_days)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetFeeStructureByID :one
SELECT * FROM fee_structures WHERE id = $1 LIMIT 1;

-- name: ListFeeStructuresByCourse :many
SELECT * FROM fee_structures WHERE course_id = $1 AND is_active = TRUE ORDER BY created_at DESC;

-- name: ListFeeStructuresByBatch :many
SELECT * FROM fee_structures WHERE batch_id = $1 AND is_active = TRUE ORDER BY created_at DESC;

-- name: DeactivateFeeStructure :exec
UPDATE fee_structures SET is_active = FALSE, updated_at = CURRENT_TIMESTAMP WHERE id = $1;

-- name: CreateStudentFee :one
INSERT INTO student_fees (user_id, fee_structure_id, course_id, batch_id, total_amount, currency, due_date)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetStudentFeeByID :one
SELECT * FROM student_fees WHERE id = $1 LIMIT 1;

-- name: ListMyStudentFees :many
SELECT sf.*, c.title AS course_title
FROM student_fees sf
LEFT JOIN courses c ON c.id = sf.course_id
WHERE sf.user_id = $1
ORDER BY sf.due_date ASC NULLS LAST, sf.created_at DESC;

-- name: ListPendingStudentFees :many
SELECT sf.*, u.email, u.full_name
FROM student_fees sf
JOIN users u ON u.id = sf.user_id
WHERE sf.status IN ('pending','partial','overdue')
ORDER BY sf.due_date ASC NULLS LAST
LIMIT $1 OFFSET $2;

-- name: UpdateFeePaidAmount :one
UPDATE student_fees
SET paid_amount = paid_amount + $2,
    status = CASE
               WHEN paid_amount + $2 >= total_amount THEN 'paid'
               WHEN paid_amount + $2 > 0 THEN 'partial'
               ELSE status
             END,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: MarkOverdueFees :exec
UPDATE student_fees
SET status = 'overdue', updated_at = CURRENT_TIMESTAMP
WHERE status IN ('pending','partial')
  AND due_date IS NOT NULL
  AND due_date < CURRENT_DATE;

-- name: CreateFeeInstallment :one
INSERT INTO fee_installments (student_fee_id, installment_number, amount, due_date)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetInstallmentByID :one
SELECT * FROM fee_installments WHERE id = $1 LIMIT 1;

-- name: ListInstallmentsForFee :many
SELECT * FROM fee_installments WHERE student_fee_id = $1 ORDER BY installment_number ASC;

-- name: MarkInstallmentPaid :one
UPDATE fee_installments
SET status = 'paid', paid_at = CURRENT_TIMESTAMP, payment_id = $2
WHERE id = $1
RETURNING *;

-- name: ListOverdueInstallments :many
SELECT fi.*, sf.user_id, u.email
FROM fee_installments fi
JOIN student_fees sf ON sf.id = fi.student_fee_id
JOIN users u ON u.id = sf.user_id
WHERE fi.status = 'pending'
  AND fi.due_date IS NOT NULL
  AND fi.due_date < CURRENT_DATE
ORDER BY fi.due_date ASC
LIMIT $1 OFFSET $2;

-- name: RevenueSummary :one
SELECT
    COALESCE(SUM(amount) FILTER (WHERE status = 'captured'), 0)::numeric AS captured_total,
    COALESCE(SUM(amount) FILTER (WHERE status = 'created'), 0)::numeric  AS pending_total,
    COUNT(*) FILTER (WHERE status = 'captured')::bigint                  AS captured_count
FROM payments
WHERE created_at >= $1 AND created_at < $2;
