// Package videoguard hardens video delivery: short-lived signed URLs for
// premium playback and a watermark spec the FFmpeg worker uses to overlay
// the student's phone on every recording variant.
//
// We deliberately keep the worker out of process — overlaying frames is
// slow and bursty, easier to scale by spawning more sidecars. This package
// just defines the contracts + URL signer the API serves.
package videoguard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// PlaybackTokenTTL is how long a signed URL is valid for. We pick 15 minutes:
// long enough to start the player and absorb a tab refresh, short enough
// that a leaked URL becomes useless before a screen recorder can catch up.
//
// Clients that need longer playback (a multi-hour recorded class) refresh
// the token via /downloads/token before the previous one expires.
const PlaybackTokenTTL = 15 * time.Minute

// Sign builds an opaque token of the form `<hex(hmac_sha256(payload))>.<exp_unix>`.
// Payload is `userID|recordingID|exp`, signed with the server-side secret.
//
// Verifiers check the signature first, then exp >= now. We don't encode the
// userID/recordingID into the token (it goes in the URL path) — keeping the
// token small avoids surprising URL-length limits on cheap CDNs.
func Sign(secret, userID, recordingID string, exp time.Time) string {
	payload := userID + "|" + recordingID + "|" + strconv.FormatInt(exp.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return sig + "." + strconv.FormatInt(exp.Unix(), 10)
}

// Verify returns nil on success. Constant-time comparison prevents timing
// oracles even though the upstream attacker pool is small.
func Verify(secret, userID, recordingID, token string) error {
	dot := strings.IndexByte(token, '.')
	if dot < 0 {
		return fmt.Errorf("malformed token")
	}
	gotSig := token[:dot]
	expStr := token[dot+1:]
	expUnix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return fmt.Errorf("malformed exp")
	}
	if time.Now().Unix() > expUnix {
		return fmt.Errorf("token expired")
	}

	payload := userID + "|" + recordingID + "|" + expStr
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(gotSig)) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}
