package auth

import (
	"encoding/base32"
	"regexp"
	"strings"
	"testing"
	"time"
)

// RFC 6238 appendix B, SHA-1 vectors (8 digits there; we use the last 6).
func TestTOTPCodeRFCVectors(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, tc := range []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	} {
		got, err := TOTPCode(secret, tc.unix/30)
		if err != nil || got != tc.want {
			t.Errorf("t=%d: got %q (%v), want %q", tc.unix, got, err, tc.want)
		}
	}
}

func TestCheckTOTPDriftAndReplay(t *testing.T) {
	secret := NewTOTPSecret()
	now := time.Unix(1_800_000_000, 0)
	step := TOTPStep(now)
	code := func(s int64) string { c, _ := TOTPCode(secret, s); return c }

	for _, d := range []int64{-1, 0, 1} {
		if got, ok := CheckTOTP(secret, code(step+d), now, 0); !ok || got != step+d {
			t.Errorf("drift %d: ok=%v step=%d", d, ok, got)
		}
	}
	for _, d := range []int64{-2, 2} {
		if _, ok := CheckTOTP(secret, code(step+d), now, 0); ok {
			t.Errorf("drift %d accepted", d)
		}
	}
	// Spaces and dashes are ignored.
	c := code(step)
	if _, ok := CheckTOTP(secret, c[:3]+" "+c[3:], now, 0); !ok {
		t.Error("code with a space rejected")
	}
	// Replay: once a step is used, it and earlier steps are refused.
	if _, ok := CheckTOTP(secret, code(step), now, step); ok {
		t.Error("reused code accepted")
	}
	if _, ok := CheckTOTP(secret, code(step+1), now, step); !ok {
		t.Error("next code refused after a use")
	}
	if _, ok := CheckTOTP(secret, "12345", now, 0); ok {
		t.Error("5-digit code accepted")
	}
}

func TestTOTPURI(t *testing.T) {
	u := TOTPURI("BEACON", "ana@example.com", "ABC")
	if !strings.HasPrefix(u, "otpauth://totp/BEACON:ana@example.com?") || !strings.Contains(u, "secret=ABC") ||
		!strings.Contains(u, "issuer=BEACON") {
		t.Fatalf("unexpected URI %q", u)
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes := NewRecoveryCodes()
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("got %d codes", len(codes))
	}
	re := regexp.MustCompile(`^[a-z2-9]{5}-[a-z2-9]{5}$`)
	seen := map[string]bool{}
	for _, c := range codes {
		if !re.MatchString(c) {
			t.Errorf("bad code format %q", c)
		}
		if seen[c] {
			t.Errorf("duplicate code %q", c)
		}
		seen[c] = true
	}
	c := codes[0]
	if HashRecoveryCode(c) != HashRecoveryCode(" "+strings.ToUpper(strings.ReplaceAll(c, "-", ""))+" ") {
		t.Error("hash differs for the same code typed differently")
	}
	if HashRecoveryCode(codes[0]) == HashRecoveryCode(codes[1]) {
		t.Error("different codes hash the same")
	}
}
