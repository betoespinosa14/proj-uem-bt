package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Two-step sign-in with authenticator-app codes (TOTP, RFC 6238): SHA-1,
// 6 digits, 30-second steps, the defaults every authenticator app supports.
const (
	totpPeriod = 30
	totpDigits = 6
	totpSkew   = 1 // steps accepted either side of now, for clock drift

	RecoveryCodeCount = 10
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a random 160-bit secret, base32 encoded.
func NewTOTPSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b32.EncodeToString(b)
}

// TOTPURI is the otpauth:// link an authenticator app reads from the QR code.
func TOTPURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(totpPeriod))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// TOTPStep is the time step a moment falls in.
func TOTPStep(t time.Time) int64 { return t.Unix() / totpPeriod }

// TOTPCode is the code for a secret at a time step.
func TOTPCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimRight(secret, "=")))
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, n%1_000_000), nil
}

// CheckTOTP reports whether code is valid at now (±1 step) and newer than
// lastStep, the step of the last code accepted, so a code can't be reused.
// It returns the matching step, to be saved as the new lastStep.
func CheckTOTP(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, code)
	if len(code) != totpDigits {
		return 0, false
	}
	cur := TOTPStep(now)
	for s := cur - totpSkew; s <= cur+totpSkew; s++ {
		if s <= lastStep {
			continue
		}
		want, err := TOTPCode(secret, s)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// NewRecoveryCodes returns one-time codes like "k7m2q-x9c4d", using letters
// and digits that can't be confused with each other.
func NewRecoveryCodes() []string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	codes := make([]string, RecoveryCodeCount)
	for i := range codes {
		b := make([]byte, 0, 10)
		buf := make([]byte, 1)
		for len(b) < 10 {
			if _, err := rand.Read(buf); err != nil {
				panic(err)
			}
			// Skip the top of the byte range so every character is equally likely.
			if int(buf[0]) < 256/len(alphabet)*len(alphabet) {
				b = append(b, alphabet[int(buf[0])%len(alphabet)])
			}
		}
		codes[i] = string(b[:5]) + "-" + string(b[5:])
	}
	return codes
}

// NormalizeRecoveryCode makes typed codes match regardless of case, spaces
// and the dash.
func NormalizeRecoveryCode(code string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r == ' ' || r == '-':
			return -1
		}
		return r
	}, strings.TrimSpace(code))
}

// HashRecoveryCode hashes a (normalized) recovery code for storage. The codes
// carry about 49 bits of randomness and are rate-limited, so SHA-256 is enough.
func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte("beacon-recovery:" + NormalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}
