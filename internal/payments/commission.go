package payments

// Commission rates per platform-subscription plan. Numbers are basis points
// (10000 = 100%, 200 = 2%) so the math stays integer-safe end-to-end.
//
// Mapping mirrors the published pricing tiers:
//   starter    — 5% platform commission, tenant pays no platform fee.
//   pro        — 2% commission, ₹35K/yr platform fee.
//   premium    — 0% commission (all gross to tenant).
//   enterprise — 0% commission, custom contract.
//
// The commission stays on the platform Razorpay account (no transfer); the
// remainder is split to the tenant's Linked Account via a Transfer.
var commissionBPS = map[string]int64{
	"starter":    500,
	"pro":        200,
	"premium":    0,
	"enterprise": 0,
}

// CommissionFor looks up the basis-point cut for a plan. Defaults to the
// starter rate so a new tenant with no platform plan still funnels revenue
// to us.
func CommissionFor(plan string) int64 {
	if v, ok := commissionBPS[plan]; ok {
		return v
	}
	return commissionBPS["starter"]
}

// SplitForTenant computes the (platformPaise, tenantPaise) breakdown for an
// order amount given a tenant's plan. If the tenant doesn't have a Linked
// Account the caller should still pass the full amount to platform — Route
// transfers require a real linked-account ID.
func SplitForTenant(amountPaise int64, plan string) (platform, tenant int64) {
	bps := CommissionFor(plan)
	platform = (amountPaise * bps) / 10000
	tenant = amountPaise - platform
	return platform, tenant
}
