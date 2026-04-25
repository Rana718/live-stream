-- 036_user_subscription_provider_id.sql
-- Add Razorpay subscription ID to user_subscriptions so the webhook
-- handler can find the local row from a sub_XXX event payload without an
-- external mapping table. Stored at row creation time by the
-- subscriptions service.

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS razorpay_subscription_id VARCHAR(100);

CREATE INDEX IF NOT EXISTS idx_user_subs_provider_id
    ON user_subscriptions(razorpay_subscription_id)
    WHERE razorpay_subscription_id IS NOT NULL;
