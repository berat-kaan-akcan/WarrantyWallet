// Package auth implements password hashing (PBKDF2-SHA256) and HS256 JWTs using only the
// Go standard library, plus HTTP middleware.
package auth

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DefaultIterations follows OWASP's 2023 recommendation for PBKDF2-HMAC-SHA256.
const DefaultIterations = 600_000

// MinPasswordLen is the minimum accepted password length.
const MinPasswordLen = 8

// ErrInvalidToken is returned for any malformed, forged or expired token.
var ErrInvalidToken = errors.New("invalid token")

// Hasher hashes and verifies passwords. Iterations is configurable so tests stay fast.
type Hasher struct{ Iterations int }

// Hash returns "pbkdf2-sha256$<iter>$<salt>$<hash>".
func (h Hasher) Hash(password string) (string, error) {
	iter := h.Iterations
	if iter <= 0 {
		iter = DefaultIterations
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iter, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", iter, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Verify checks a password against a stored hash in constant time.
func (h Hasher) Verify(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// Claims carried in the JWT.
type Claims struct {
	Subject string `json:"sub"`
	Role    string `json:"role"`
	Issued  int64  `json:"iat"`
	Expires int64  `json:"exp"`
}

// Tokens issues and verifies HS256 JWTs.
type Tokens struct {
	Secret []byte
	TTL    time.Duration
	Now    func() time.Time
}

func (t Tokens) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

var b64 = base64.RawURLEncoding

const jwtHeader = `{"alg":"HS256","typ":"JWT"}`

// Issue creates a signed token for the user.
func (t Tokens) Issue(userID, role string) (string, time.Time, error) {
	if len(t.Secret) == 0 {
		return "", time.Time{}, errors.New("empty JWT secret")
	}
	now := t.now()
	exp := now.Add(t.TTL)
	payload, err := json.Marshal(Claims{Subject: userID, Role: role, Issued: now.Unix(), Expires: exp.Unix()})
	if err != nil {
		return "", time.Time{}, err
	}
	signing := b64.EncodeToString([]byte(jwtHeader)) + "." + b64.EncodeToString(payload)
	return signing + "." + b64.EncodeToString(t.sign(signing)), exp, nil
}

// Verify validates signature, algorithm and expiry.
func (t Tokens) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}
	hdr, err := b64.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var h struct {
		Alg string `json:"alg"`
	}
	if json.Unmarshal(hdr, &h) != nil || h.Alg != "HS256" {
		return Claims{}, ErrInvalidToken
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, t.sign(parts[0]+"."+parts[1])) {
		return Claims{}, ErrInvalidToken
	}
	body, err := b64.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var c Claims
	if json.Unmarshal(body, &c) != nil || c.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	if t.now().Unix() >= c.Expires {
		return Claims{}, ErrInvalidToken
	}
	return c, nil
}

func (t Tokens) sign(s string) []byte {
	m := hmac.New(sha256.New, t.Secret)
	m.Write([]byte(s))
	return m.Sum(nil)
}
