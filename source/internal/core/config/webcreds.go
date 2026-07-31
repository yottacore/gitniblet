// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	shcl "github.com/jim-collier/shcl/source/go"
)

// webCredsFile holds the proxied-mode web logins. It lives at the registry root,
// deliberately OUTSIDE any git-synced tree: password hashes are per-deployment
// secrets and must never ride along in the shared tx-log repo. Like settingsFile
// it is a plain file, so database discovery (which scans only dirs) ignores it.
const webCredsFile = "webusers.shcl"

// pbkdf2 parameters. Hashing is stdlib (crypto/pbkdf2, Go 1.24+), so proxied
// login adds no dependency - the same discipline as the rest of the web tier.
// The iteration count follows the OWASP guidance for PBKDF2-HMAC-SHA256.
const (
	pbkdf2Iters   = 600000
	pbkdf2SaltLen = 16
	pbkdf2KeyLen  = 32
)

// WebCreds is the set of username -> password-hash records for proxied mode.
type WebCreds struct {
	Users map[string]string
}

func webCredsPath() (string, error) {
	base, err := UserBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, webCredsFile), nil
}

// LoadWebCreds reads the web logins, returning an empty set when the file is
// missing (proxied mode with no users yet is a valid, if useless, state - the
// server simply cannot be logged into until a user is added).
// Each login is one "user" instance discriminated by the username, with the
// hash on a child leaf. Instances (not child field names) carry the username
// because SHCL folds field-name case but compares discriminator values exactly
// - so "Alice" and "alice" stay two accounts, as they were under the old map.
func LoadWebCreds() *WebCreds {
	c := &WebCreds{Users: map[string]string{}}
	path, err := webCredsPath()
	if err != nil {
		return c
	}
	doc := loadLenient(path)
	for i, user := range doc.Instances("user") {
		if user == "" {
			continue
		}
		c.Users[user] = doc.GetStringOr(fmt.Sprintf("user[#%d].hash", i), "")
	}
	return c
}

// Set adds or replaces a user's password, hashing it with a fresh random salt.
func (c *WebCreds) Set(user, password string) error {
	if c.Users == nil {
		c.Users = map[string]string{}
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	c.Users[user] = hash
	return nil
}

// Verify reports whether password matches the stored hash for user. It runs the
// KDF even for an unknown user so a missing account and a wrong password take the
// same time, giving nothing away by timing.
func (c *WebCreds) Verify(user, password string) bool {
	stored, ok := c.Users[user]
	if !ok {
		stored = "" // still hash below, then fail the compare
	}
	return verifyHash(stored, password)
}

// Save writes the web logins with 0600 perms - they are password hashes.
func (c *WebCreds) Save() error {
	path, err := webCredsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	doc := shcl.New()
	// Sorted so a save is reproducible; Go map order would reshuffle the file
	// on every write and make it undiffable.
	for _, user := range slices.Sorted(maps.Keys(c.Users)) {
		doc.SetString(fmt.Sprintf("user[%q].hash", user), c.Users[user])
	}
	return save(path, doc, 0o600)
}

// hashPassword returns a self-describing PBKDF2 hash string:
// pbkdf2-sha256$<iters>$<salt-b64>$<hash-b64>. Storing the params inline lets the
// cost be raised later without invalidating existing hashes.
func hashPassword(password string) (string, error) {
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iters, pbkdf2KeyLen)
	if err != nil {
		return "", err
	}
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iters, b64.EncodeToString(salt), b64.EncodeToString(dk)), nil
}

// verifyHash checks password against a stored hash string in constant time. A
// malformed or empty stored hash always fails, but only after a real KDF run so
// the timing does not distinguish it from a wrong password.
func verifyHash(stored, password string) bool {
	iters, salt, want := parseHash(stored)
	if iters <= 0 {
		iters, salt = pbkdf2Iters, make([]byte, pbkdf2SaltLen) // burn the time anyway
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iters, len(want))
	if err != nil || len(want) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// parseHash splits a stored pbkdf2-sha256$iters$salt$hash string. It returns a
// zero iteration count on any parse failure.
func parseHash(stored string) (iters int, salt, hash []byte) {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return 0, nil, nil
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n <= 0 {
		return 0, nil, nil
	}
	b64 := base64.RawStdEncoding
	salt, err = b64.DecodeString(parts[2])
	if err != nil {
		return 0, nil, nil
	}
	hash, err = b64.DecodeString(parts[3])
	if err != nil {
		return 0, nil, nil
	}
	return n, salt, hash
}
