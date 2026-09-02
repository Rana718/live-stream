-- 0002_enums.sql
-- Every native enum type. Plain CREATE TYPE (not wrapped) so sqlc's static
-- parser picks them up and emits a Go type per enum. This file runs exactly
-- once (tracked in schema_migrations_applied); it is not re-run.
--
-- RULES for changing an enum later:
--   * `ALTER TYPE x ADD VALUE 'y'` must be its own migration and cannot run
--     in a transaction that then uses 'y' (Postgres < 16) — keep it alone.
--   * values cannot be removed or reordered.

-- identity / tenancy
CREATE TYPE tenant_role       AS ENUM ('owner','admin','instructor','staff','student','parent');
CREATE TYPE tenant_status     AS ENUM ('trial','active','past_due','suspended','cancelled');
CREATE TYPE tenant_plan       AS ENUM ('starter','growth','pro','enterprise');
CREATE TYPE membership_status AS ENUM ('invited','active','suspended');
CREATE TYPE auth_provider     AS ENUM ('phone','google','apple','password');
CREATE TYPE otp_channel       AS ENUM ('sms','whatsapp','email');
CREATE TYPE otp_purpose       AS ENUM ('login','link','verify');
CREATE TYPE lead_status       AS ENUM ('new','contacted','qualified','converted','lost');
CREATE TYPE build_status      AS ENUM ('queued','building','succeeded','failed','published');
CREATE TYPE build_platform    AS ENUM ('android','ios');

-- catalog / content / assessment
CREATE TYPE publish_status     AS ENUM ('draft','in_review','published','archived');
CREATE TYPE content_kind       AS ENUM ('video','document','live_session','quiz','assignment','link');
CREATE TYPE session_status     AS ENUM ('scheduled','live','ended','cancelled');
CREATE TYPE recording_status   AS ENUM ('queued','processing','ready','failed');
CREATE TYPE question_kind      AS ENUM ('mcq_single','mcq_multi','numeric','subjective','match');
CREATE TYPE test_kind          AS ENUM ('dpp','chapter_test','subject_test','mock','pyq','live_quiz');
CREATE TYPE attempt_status     AS ENUM ('in_progress','submitted','graded','expired','abandoned');
CREATE TYPE attendance_status  AS ENUM ('present','absent','late','excused');
CREATE TYPE doubt_status       AS ENUM ('open','answered','resolved','closed');
CREATE TYPE certificate_status AS ENUM ('issued','revoked');

-- commerce / billing
CREATE TYPE product_kind          AS ENUM ('course','bundle','plan','fee_plan');
CREATE TYPE coupon_type           AS ENUM ('percent','flat');
CREATE TYPE coupon_scope          AS ENUM ('all','products','categories');
CREATE TYPE order_status          AS ENUM ('pending','awaiting_payment','paid','partially_paid','cancelled','refunded','partially_refunded');
CREATE TYPE payment_status        AS ENUM ('created','authorized','captured','failed','refunded');
CREATE TYPE payment_method        AS ENUM ('card','upi','netbanking','wallet','emi','other');
CREATE TYPE refund_status         AS ENUM ('pending','processing','processed','failed');
CREATE TYPE refund_reason         AS ENUM ('requested_by_customer','duplicate','fraud','goodwill','chargeback','other');
CREATE TYPE invoice_status        AS ENUM ('issued','cancelled');
CREATE TYPE gst_supply_type       AS ENUM ('intra_state','inter_state','export','exempt');
CREATE TYPE entitlement_source    AS ENUM ('purchase','subscription','coupon','manual_grant','fee_plan','gift','bundle');
CREATE TYPE enrollment_status     AS ENUM ('active','completed','cancelled','expired');
CREATE TYPE subscription_status   AS ENUM ('trialing','active','past_due','paused','cancelled','expired');
CREATE TYPE subscription_interval AS ENUM ('monthly','quarterly','half_yearly','yearly','custom');
CREATE TYPE installment_status    AS ENUM ('pending','paid','overdue','waived');
CREATE TYPE fee_account_status    AS ENUM ('pending','partial','paid','overdue','waived');

-- wallet / payouts
CREATE TYPE wallet_txn_kind AS ENUM ('referral_reward','refund_credit','purchase_debit','adjustment','payout_debit');
CREATE TYPE payout_kind     AS ENUM ('affiliate','instructor_revshare','refund');
CREATE TYPE payout_status   AS ENUM ('pending','processing','paid','failed');
CREATE TYPE referral_status AS ENUM ('pending','qualified','rewarded','void');

-- communication
CREATE TYPE notification_channel AS ENUM ('in_app','push','sms','whatsapp','email');
CREATE TYPE delivery_status      AS ENUM ('queued','sent','delivered','failed','read');
CREATE TYPE message_direction    AS ENUM ('inbound','outbound');
