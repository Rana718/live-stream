// Package i18n is a minimal in-process string catalogue for human-facing
// API messages (error strings, push notifications). The locale arrives via
// the LocaleMiddleware; this package just resolves a key against it.
//
// Why not a third-party lib (go-i18n etc.): the catalogue stays small —
// dozens of strings, not hundreds — and the rest of the message body is
// rendered client-side from structured data anyway. Pulling a heavy lib
// for this is overkill.
//
// Add a new key by appending to `catalogue` below. Keys are stable IDs;
// changing a key text is a contract change, changing a translation isn't.
package i18n

import (
	"context"
	"strings"
)

const localeKey = "locale"

// catalogue holds every translation. Keys are language-agnostic IDs the
// handler picks; values are per-locale strings. Locales not present in a
// row fall through to the "en" default.
//
// Hindi (hi) and Hinglish (hinglish) are first-class because the target
// market is India. Other Indian languages can be filled in incrementally —
// the resolver falls back to English with no behaviour change.
var catalogue = map[string]map[string]string{
	"auth.invalid_otp": {
		"en":       "Invalid or expired OTP",
		"hi":       "अमान्य या समाप्त हो चुका OTP",
		"hinglish": "OTP galat hai ya expire ho chuka hai",
	},
	"auth.otp_sent": {
		"en":       "OTP sent",
		"hi":       "OTP भेज दिया गया",
		"hinglish": "OTP bhej diya gaya",
	},
	"auth.tenant_required": {
		"en":       "Org code required",
		"hi":       "Org code आवश्यक है",
		"hinglish": "Org code zaroori hai",
	},
	"course.already_enrolled": {
		"en":       "You're already enrolled in this course",
		"hi":       "आप पहले से ही इस कोर्स में नामांकित हैं",
		"hinglish": "Aap pehle se hi is course mein enroll ho",
	},
	"coupon.expired": {
		"en":       "This coupon has expired",
		"hi":       "यह कूपन समाप्त हो चुका है",
		"hinglish": "Yeh coupon expire ho chuka hai",
	},
	"coupon.exhausted": {
		"en":       "This coupon has been used up",
		"hi":       "यह कूपन समाप्त हो चुका है",
		"hinglish": "Yeh coupon khatam ho chuka hai",
	},
	"payment.signature_mismatch": {
		"en":       "Payment signature mismatch — please try again",
		"hi":       "भुगतान हस्ताक्षर मेल नहीं खाते — कृपया पुनः प्रयास करें",
		"hinglish": "Payment signature match nahi hua — phir se try karein",
	},
	"device.limit_reached": {
		"en":       "Logged out from oldest device — only 2 devices allowed per account",
		"hi":       "सबसे पुराने डिवाइस से लॉगआउट कर दिया गया — खाते में केवल 2 डिवाइस की अनुमति है",
		"hinglish": "Sabse purane device se logout ho gaya — sirf 2 devices allowed hain",
	},
	"live.starting_soon": {
		"en":       "Live class starting soon",
		"hi":       "लाइव क्लास जल्द ही शुरू होगी",
		"hinglish": "Live class jald hi shuru hogi",
	},
}

// LocaleFromCtx pulls the locale set by middleware. Returns "en" as the
// safe default so a missing locale never crashes a handler.
func LocaleFromCtx(ctx context.Context) string {
	if v := ctx.Value(localeKey); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return "en"
}

// T translates a key for the given locale. Falls back to English; falls
// back to the key itself if even English is missing (so a missing
// translation surfaces as a developer-visible string in QA rather than
// silently returning "").
func T(locale, key string) string {
	row, ok := catalogue[key]
	if !ok {
		return key
	}
	if v, ok := row[strings.ToLower(locale)]; ok && v != "" {
		return v
	}
	if v, ok := row["en"]; ok && v != "" {
		return v
	}
	return key
}

// TCtx is the convenient handler call:  i18n.TCtx(c.Context(), "auth.otp_sent").
func TCtx(ctx context.Context, key string) string {
	return T(LocaleFromCtx(ctx), key)
}
