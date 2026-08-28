// Package auth implements the doc server's authentication: password
// verification (sha256 + constant-time compare) and HMAC-signed tokens.
// No external dependencies.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Config is the server-side auth configuration.
type Config struct {
	Enable   bool
	Password string
	Secret   string
	Expire   time.Duration
	Secure   bool // set Secure on the session cookie
}

// CheckPassword reports whether got matches the configured password, in
// constant time: both sides are hashed with the secret before comparison.
func CheckPassword(secret, want, got string) bool {
	a := sha256.Sum256([]byte(got + secret))
	b := sha256.Sum256([]byte(want + secret))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// Issue returns a token for data: base64url(expiry.data) . base64url(hmac).
func Issue(secret string, ttl time.Duration, data string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("auth: empty secret")
	}
	payload := strconv.FormatInt(time.Now().Add(ttl).Unix(), 10) + "." + data
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// Verify validates a token's signature and expiry, returning its data.
func Verify(secret, token string) (string, bool) {
	if secret == "" {
		return "", false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	// Reject non-canonical encodings: base64 ignores trailing padding bits,
	// so a tampered last char could decode to the same bytes.
	if base64.RawURLEncoding.EncodeToString(payload) != parts[0] ||
		base64.RawURLEncoding.EncodeToString(sig) != parts[1] {
		return "", false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", false
	}
	body := string(payload)
	i := strings.IndexByte(body, '.')
	if i < 0 {
		return "", false
	}
	exp, err := strconv.ParseInt(body[:i], 10, 64)
	if err != nil || time.Now().Unix() >= exp {
		return "", false
	}
	return body[i+1:], true
}
