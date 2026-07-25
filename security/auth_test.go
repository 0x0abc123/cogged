package security

import "testing"

func testKey(t *testing.T) string {
	t.Helper()
	b, err := GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("rand: %v", err)
	}
	return B64Encode(b)
}

func TestMACValidation(t *testing.T) {
	key := B64Decode(testKey(t))
	msg := []byte("authenticate me")
	mac := MAC(msg, key)
	if !IsValidMAC(msg, mac, key) {
		t.Error("IsValidMAC rejected a genuine MAC")
	}
	if IsValidMAC([]byte("tampered"), mac, key) {
		t.Error("IsValidMAC accepted a MAC for a different message")
	}
	other := B64Decode(testKey(t))
	if IsValidMAC(msg, mac, other) {
		t.Error("IsValidMAC accepted a MAC under a different key")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	key := testKey(t)
	uid, role, tokenId, ts := "0x1a", "user", "tok-123", "1700000000"

	token := ConstructToken(uid, role, tokenId, ts, key)
	uad := UADFromToken(token, key)
	if uad == nil {
		t.Fatal("UADFromToken returned nil for a valid token")
	}
	if uad.Uid != uid || uad.Role != role || uad.TokenId != tokenId || uad.Timestamp != ts {
		t.Errorf("decoded UAD = %+v, want uid=%s role=%s tok=%s ts=%s", uad, uid, role, tokenId, ts)
	}
	// per-user key is derived deterministically from master + uid + role
	if uad.SecretKey != UserKeyFromMasterSecret(key, uid, role) {
		t.Error("UAD.SecretKey does not match UserKeyFromMasterSecret")
	}
}

func TestTokenRejectsTamperingAndWrongKey(t *testing.T) {
	key := testKey(t)
	token := ConstructToken("0x1", "sys", "t", "1", key)

	if UADFromToken(token, testKey(t)) != nil {
		t.Error("token verified under a different master key")
	}
	if UADFromToken("garbage-without-dot", key) != nil {
		t.Error("malformed token (no MAC separator) should return nil")
	}
	if UADFromToken(token+"x", key) != nil {
		t.Error("tampered token should fail MAC verification")
	}
}

func TestIsAdmin(t *testing.T) {
	if !(&UserAuthData{Role: SYS_ROLE}).IsAdmin() {
		t.Errorf("role %q should be admin", SYS_ROLE)
	}
	if (&UserAuthData{Role: "user"}).IsAdmin() {
		t.Error("role \"user\" should not be admin")
	}
}

func TestUserKeyFromMasterSecretDeterministicAndScoped(t *testing.T) {
	master := testKey(t)
	a := UserKeyFromMasterSecret(master, "0x1", "user")
	if a != UserKeyFromMasterSecret(master, "0x1", "user") {
		t.Error("UserKeyFromMasterSecret is not deterministic")
	}
	if a == UserKeyFromMasterSecret(master, "0x2", "user") {
		t.Error("keys for different uids should differ")
	}
	if a == UserKeyFromMasterSecret(master, "0x1", "sys") {
		t.Error("keys for different roles should differ")
	}
}
