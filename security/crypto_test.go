package security

import (
	"strings"
	"testing"
)

func TestB64RoundTrip(t *testing.T) {
	cases := [][]byte{
		{},
		{0x00},
		{0xff, 0x00, 0xaa, 0x55},
		[]byte("hello, cogged"),
	}
	for _, in := range cases {
		enc := B64Encode(in)
		if strings.ContainsAny(enc, "+/=") {
			t.Errorf("B64Encode(%v) = %q, expected raw-url encoding without +/=", in, enc)
		}
		got := B64Decode(enc)
		if string(got) != string(in) {
			t.Errorf("round trip mismatch: in=%v enc=%q out=%v", in, enc, got)
		}
	}
}

func TestGenerateGuidFormat(t *testing.T) {
	g, err := GenerateGuid()
	if err != nil {
		t.Fatalf("GenerateGuid error: %v", err)
	}
	if len(g) != 36 {
		t.Fatalf("guid length = %d, want 36 (%q)", len(g), g)
	}
	parts := strings.Split(g, "-")
	wantLens := []int{8, 4, 4, 4, 12}
	if len(parts) != len(wantLens) {
		t.Fatalf("guid %q has %d groups, want 5", g, len(parts))
	}
	for i, p := range parts {
		if len(p) != wantLens[i] {
			t.Errorf("guid group %d = %q len %d, want %d", i, p, len(p), wantLens[i])
		}
	}
	// version 4 nibble
	if parts[2][0] != '4' {
		t.Errorf("guid version nibble = %q, want '4' (%q)", parts[2][0], g)
	}
	// uniqueness
	g2, _ := GenerateGuid()
	if g == g2 {
		t.Errorf("two guids collided: %q", g)
	}
}

func TestMD5SumHexKnownVector(t *testing.T) {
	// md5("") = d41d8cd98f00b204e9800998ecf8427e
	if got := MD5SumHex([]byte("")); got != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("MD5SumHex(\"\") = %q", got)
	}
	if got := MD5SumHex([]byte("abc")); got != "900150983cd24fb0d6963f7d28e17f72" {
		t.Errorf("MD5SumHex(\"abc\") = %q", got)
	}
	if len(MD5SumHex([]byte("anything"))) != 32 {
		t.Error("MD5SumHex should always be 32 hex chars")
	}
}

func TestPasswordHashVerify(t *testing.T) {
	stored := GeneratePasswordHash("correct horse")
	if !strings.Contains(stored, "$") {
		t.Fatalf("stored hash %q not in <hash>$<salt> form", stored)
	}
	if !VerifyPasswordHash(stored, "correct horse") {
		t.Error("VerifyPasswordHash rejected the correct password")
	}
	if VerifyPasswordHash(stored, "wrong password") {
		t.Error("VerifyPasswordHash accepted a wrong password")
	}
	// two hashes of the same password differ (random salt)
	h1 := GeneratePasswordHash("same")
	h2 := GeneratePasswordHash("same")
	if h1 == h2 {
		t.Error("password hashes should differ due to random salt")
	}
	// malformed stored hash
	if VerifyPasswordHash("no-dollar-sign", "x") {
		t.Error("VerifyPasswordHash should reject a malformed stored hash")
	}
}

func TestAESGCMRoundTrip(t *testing.T) {
	key := B64Encode(mustBytes(t, 32)) // 32-byte AES-256 key, b64 encoded
	plain := "secret payload \x00 with bytes"

	ct, err := AESGCMEncrypt(key, plain)
	if err != nil {
		t.Fatalf("encrypt error: %v", err)
	}
	if !strings.Contains(ct, ".") {
		t.Fatalf("ciphertext %q not in <ct>.<nonce> form", ct)
	}
	got, err := AESGCMDecrypt(key, ct)
	if err != nil {
		t.Fatalf("decrypt error: %v", err)
	}
	if got != plain {
		t.Errorf("decrypt = %q, want %q", got, plain)
	}

	// wrong key fails to authenticate
	otherKey := B64Encode(mustBytes(t, 32))
	if _, err := AESGCMDecrypt(otherKey, ct); err == nil {
		t.Error("decrypt with wrong key should fail")
	}

	// malformed ciphertext (no nonce separator)
	if _, err := AESGCMDecrypt(key, "notvalid"); err == nil {
		t.Error("decrypt of malformed ciphertext should fail")
	}
}

func mustBytes(t *testing.T, n int) []byte {
	t.Helper()
	b, err := GenerateRandomBytes(n)
	if err != nil {
		t.Fatalf("GenerateRandomBytes: %v", err)
	}
	if len(b) != n {
		t.Fatalf("GenerateRandomBytes returned %d bytes, want %d", len(b), n)
	}
	return b
}
