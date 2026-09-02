-- 0002_enums.sql
-- Every native enum type, in one place. Enums are used for STABLE domains
-- (a value is added rarely, never removed or reordered). Volatile
-- vocabularies (job_kind, notification_category, audit_action,
-- messaging_provider) stay text + CHECK in their table definitions.
--
-- RULES for changing an enum later:
--   * `ALTER TYPE x ADD VALUE 'y'` must be its own migration and cannot run
--     in a transaction that then uses 'y' (Postgres < 16) — keep it alone.
--   * values cannot be removed or reordered; plan additions to sort
--     sensibly or rely on an explicit ORDER BY.

CREATE OR REPLACE FUNCTION _create_enum(enum_name text, vals text[]) RETURNS void
    LANGUAGE plpgsql AS
$$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = enum_name) THEN
        EXECUTE format('CREATE TYPE %I AS ENUM (%s)', enum_name,
            (SELECT string_agg(quote_literal(v), ', ') FROM unnest(vals) AS v));
    END IF;
END $$;

-- identity / tenancy
SELECT _create_enum('tenant_role',        ARRAY['owner','admin','instructor','staff','student','parent']);
SELECT _create_enum('tenant_status',      ARRAY['trial','active','past_due','suspended','cancelled']);
SELECT _create_enum('tenant_plan',        ARRAY['starter','growth','pro','enterprise']);
SELECT _create_enum('membership_status',  ARRAY['invited','active','suspended']);
SELECT _create_enum('auth_provider',      ARRAY['phone','google','apple','password']);
SELECT _create_enum('otp_channel',        ARRAY['sms','whatsapp','email']);
SELECT _create_enum('otp_purpose',        ARRAY['login','link','verify']);
SELECT _create_enum('lead_status',        ARRAY['new','contacted','qualified','converted','lost']);
SELECT _create_enum('build_status',       ARRAY['queued','building','succeeded','failed','published']);
SELECT _create_enum('build_platform',     ARRAY['android','ios']);

-- catalog / content / assessment
SELECT _create_enum('publish_status',     ARRAY['draft','in_review','published','archived']);
SELECT _create_enum('content_kind',       ARRAY['video','document','live_session','quiz','assignment','link']);
SELECT _create_enum('session_status',     ARRAY['scheduled','live','ended','cancelled']);
SELECT _create_enum('recording_status',   ARRAY['queued','processing','ready','failed']);
SELECT _create_enum('question_kind',      ARRAY['mcq_single','mcq_multi','numeric','subjective','match']);
SELECT _create_enum('test_kind',          ARRAY['dpp','chapter_test','subject_test','mock','pyq','live_quiz']);
SELECT _create_enum('attempt_status',     ARRAY['in_progress','submitted','graded','expired','abandoned']);
SELECT _create_enum('attendance_status',  ARRAY['present','absent','late','excused']);
SELECT _create_enum('doubt_status',       ARRAY['open','answered','resolved','closed']);
SELECT _create_enum('certificate_status', ARRAY['issued','revoked']);

-- commerce / billing
SELECT _create_enum('product_kind',        ARRAY['course','bundle','plan','fee_plan']);
SELECT _create_enum('coupon_type',         ARRAY['percent','flat']);
SELECT _create_enum('coupon_scope',        ARRAY['all','products','categories']);
SELECT _create_enum('order_status',        ARRAY['pending','awaiting_payment','paid','partially_paid','cancelled','refunded','partially_refunded']);
SELECT _create_enum('payment_status',      ARRAY['created','authorized','captured','failed','refunded']);
SELECT _create_enum('payment_method',      ARRAY['card','upi','netbanking','wallet','emi','other']);
SELECT _create_enum('refund_status',       ARRAY['pending','processing','processed','failed']);
SELECT _create_enum('refund_reason',       ARRAY['requested_by_customer','duplicate','fraud','goodwill','chargeback','other']);
SELECT _create_enum('invoice_status',      ARRAY['issued','cancelled']);
SELECT _create_enum('gst_supply_type',     ARRAY['intra_state','inter_state','export','exempt']);
SELECT _create_enum('entitlement_source',  ARRAY['purchase','subscription','coupon','manual_grant','fee_plan','gift','bundle']);
SELECT _create_enum('enrollment_status',   ARRAY['active','completed','cancelled','expired']);
SELECT _create_enum('subscription_status', ARRAY['trialing','active','past_due','paused','cancelled','expired']);
SELECT _create_enum('subscription_interval', ARRAY['monthly','quarterly','half_yearly','yearly','custom']);
SELECT _create_enum('installment_status',  ARRAY['pending','paid','overdue','waived']);
SELECT _create_enum('fee_account_status',  ARRAY['pending','partial','paid','overdue','waived']);

-- wallet / payouts
SELECT _create_enum('wallet_txn_kind',     ARRAY['referral_reward','refund_credit','purchase_debit','adjustment','payout_debit']);
SELECT _create_enum('payout_kind',         ARRAY['affiliate','instructor_revshare','refund']);
SELECT _create_enum('payout_status',       ARRAY['pending','processing','paid','failed']);
SELECT _create_enum('referral_status',     ARRAY['pending','qualified','rewarded','void']);

-- communication
SELECT _create_enum('notification_channel', ARRAY['in_app','push','sms','whatsapp','email']);
SELECT _create_enum('delivery_status',      ARRAY['queued','sent','delivered','failed','read']);
SELECT _create_enum('message_direction',    ARRAY['inbound','outbound']);

DROP FUNCTION IF EXISTS _create_enum(text, text[]);
