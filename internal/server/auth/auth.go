// Package auth handles password hashing, opaque session tokens, device
// secrets, and the role → permission matrix.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters (OWASP-recommended minimums).
const (
	argonTime    = 2
	argonMemory  = 19 * 1024
	argonThreads = 1
	argonKeyLen  = 32
)

var ErrMismatch = errors.New("password mismatch")

func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func CheckPassword(encoded, pw string) error {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return ErrMismatch
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return ErrMismatch
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return ErrMismatch
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return ErrMismatch
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrMismatch
	}
	return nil
}

// DummyCheck burns comparable CPU when a login email doesn't exist, so
// response timing doesn't reveal which emails are registered.
func DummyCheck(pw string) {
	argon2.IDKey([]byte(pw), make([]byte, 16), argonTime, argonMemory, argonThreads, argonKeyLen)
}

// RandomToken returns n random bytes, URL-safe base64 encoded.
func RandomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failing is unrecoverable
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken is used for session IDs and device secrets: they are high-entropy,
// so a fast hash is sufficient and lets lookups be indexed.
func HashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Permissions.
const (
	PermDevicesRead     = "devices.read"
	PermDevicesWrite    = "devices.write"
	PermCommandsRun     = "commands.run"
	PermScriptsWrite    = "scripts.write"
	PermComplianceWrite = "compliance.write"
	PermTasksWrite      = "tasks.write"
	PermEnrollmentWrite = "enrollment.write"
	PermUsersWrite      = "users.write"
	PermWebhooksWrite   = "webhooks.write"
	PermAlertsWrite     = "alerts.write"
	PermAuditRead       = "audit.read"
	PermSettingsWrite   = "settings.write"
)

var AllPermissions = []string{
	PermDevicesRead, PermDevicesWrite, PermCommandsRun, PermScriptsWrite, PermComplianceWrite, PermTasksWrite,
	PermEnrollmentWrite, PermUsersWrite, PermWebhooksWrite, PermAlertsWrite, PermAuditRead, PermSettingsWrite,
}

var Roles = []string{"owner", "admin", "helpdesk", "auditor"}

var rolePerms = map[string][]string{
	"owner": AllPermissions,
	"admin": without(AllPermissions, PermSettingsWrite),
	"helpdesk": {PermDevicesRead, PermCommandsRun, PermAlertsWrite},
	"auditor":  {PermDevicesRead, PermAuditRead},
}

func without(all []string, drop ...string) []string {
	out := []string{}
	for _, p := range all {
		keep := true
		for _, d := range drop {
			if p == d {
				keep = false
			}
		}
		if keep {
			out = append(out, p)
		}
	}
	return out
}

func PermissionsFor(role string) []string {
	if p, ok := rolePerms[role]; ok {
		return p
	}
	return []string{}
}

func Can(role, perm string) bool {
	for _, p := range rolePerms[role] {
		if p == perm {
			return true
		}
	}
	return false
}

func ValidRole(role string) bool {
	_, ok := rolePerms[role]
	return ok
}
