package models

import (
	"testing"

	sec "cogged/security"
)

// User AuthzData tokens are also per-viewer: a token minted for one user must not
// verify for another. This guards the /user/share flow, where a caller submits user
// tokens they previously received.
func TestUserAuthzDataCrossUserForgeryDenied(t *testing.T) {
	keyB := newKey(t)
	alice := &sec.UserAuthData{Uid: "0xalice", Role: "user", SecretKey: newKey(t)}

	u := NewGraphUser("0xbob")
	role := "user"
	u.Role = &role
	u.AuthzDataPack(alice) // minted for alice
	ads := u.AuthzData

	aliceView := []string{ads}
	if !AuthzDataUnpackUserADStringSlice(&aliceView, *alice, "") {
		t.Error("user token should verify for the user it was minted for")
	}

	mallory := sec.UserAuthData{Uid: "0xmallory", Role: "user", SecretKey: keyB}
	malloryView := []string{ads}
	if AuthzDataUnpackUserADStringSlice(&malloryView, mallory, "") {
		t.Error("user token minted for another user must not verify")
	}
	if GraphUserFromAD(ads, keyB) != nil {
		t.Error("GraphUserFromAD with the wrong key must return nil")
	}
}
