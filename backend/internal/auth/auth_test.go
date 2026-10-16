package auth

import (
	"strings"
	"testing"
	"time"
)

func TestHashVerify(t *testing.T) {
	h := Hasher{Iterations: 1000}
	hash, err := h.Hash("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "pbkdf2-sha256$1000$") {
		t.Fatalf("unexpected format %s", hash)
	}
	if !h.Verify("correct horse", hash) {
		t.Fatal("valid password rejected")
	}
	if h.Verify("wrong", hash) {
		t.Fatal("wrong password accepted")
	}
	for _, bad := range []string{"", "x$y", "pbkdf2-sha256$abc$a$b", "md5$1$a$b", "pbkdf2-sha256$10$!!$!!"} {
		if h.Verify("x", bad) {
			t.Fatalf("accepted malformed hash %q", bad)
		}
	}
	h2, _ := h.Hash("correct horse")
	if h2 == hash {
		t.Fatal("salt not random")
	}
}

func TestTokens(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tk := Tokens{Secret: []byte("test-secret-123456"), TTL: time.Hour, Now: func() time.Time { return now }}
	tok, exp, err := tk.Issue("user-1", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !exp.Equal(now.Add(time.Hour)) {
		t.Fatalf("exp %v", exp)
	}
	c, err := tk.Verify(tok)
	if err != nil || c.Subject != "user-1" || c.Role != "admin" {
		t.Fatalf("verify: %v %+v", err, c)
	}

	// tampered payload
	parts := strings.Split(tok, ".")
	forged := parts[0] + "." + b64.EncodeToString([]byte(`{"sub":"user-2","role":"admin","exp":9999999999}`)) + "." + parts[2]
	if _, err := tk.Verify(forged); err == nil {
		t.Fatal("forged token accepted")
	}
	// wrong secret
	other := Tokens{Secret: []byte("another-secret-xx"), TTL: time.Hour, Now: tk.Now}
	if _, err := other.Verify(tok); err == nil {
		t.Fatal("token verified with wrong secret")
	}
	// alg none
	none := b64.EncodeToString([]byte(`{"alg":"none"}`)) + "." + parts[1] + "."
	if _, err := tk.Verify(none); err == nil {
		t.Fatal("alg none accepted")
	}
	// expired
	later := Tokens{Secret: tk.Secret, TTL: time.Hour, Now: func() time.Time { return now.Add(2 * time.Hour) }}
	if _, err := later.Verify(tok); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, err := tk.Verify("garbage"); err == nil {
		t.Fatal("garbage accepted")
	}
	if _, _, err := (Tokens{}).Issue("u", "user"); err == nil {
		t.Fatal("empty secret must fail")
	}
}
