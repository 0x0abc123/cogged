package models

// Access-control / AuthzData bypass tests. These lock down the core invariants that
// keep one user from reading or mutating another user's nodes. Helpers newKey,
// nodeWithPerms and the state manager (TestMain) come from node_test.go.

import (
	"testing"

	sec "cogged/security"
	state "cogged/state"
)

// A node token is signed with the *caller's* per-user key. Presenting a token that
// was minted for a different user must fail MAC verification (forgery / replay).
func TestAuthzDataUnpackRejectsAnotherUsersToken(t *testing.T) {
	alice := &sec.UserAuthData{Uid: "0xalice", Role: "user", SecretKey: newKey(t)}
	n := nodeWithPerms("0xnode", "0xalice", "sgi-forge", "rwds")
	n.AuthzDataPack(alice) // token as alice would receive it
	ads := n.AuthzData

	mallory := sec.UserAuthData{Uid: "0xmallory", Role: "user", SecretKey: newKey(t)}
	if AuthzDataUnpackADString(ads, mallory, "r") != nil {
		t.Error("a token signed with another user's key must not verify")
	}
}

// An unauthenticated caller (empty UAD => empty SecretKey) can never unpack a token.
func TestAuthzDataUnpackDeniesUnauthenticated(t *testing.T) {
	owner := &sec.UserAuthData{Uid: "0xowner", Role: "user", SecretKey: newKey(t)}
	n := nodeWithPerms("0xnode", "0xowner", "sgi-unauth", "r")
	n.AuthzDataPack(owner)
	if AuthzDataUnpackADString(n.AuthzData, sec.UserAuthData{}, "r") != nil {
		t.Error("unauthenticated (empty UAD) access must be denied")
	}
}

// A non-owner, non-sys user with a validly-signed token but no SGI grant is denied.
func TestAuthzDataUnpackNonOwnerWithoutShareDenied(t *testing.T) {
	other := sec.UserAuthData{Uid: "0xnoshare", Role: "user", SecretKey: newKey(t)}
	n := nodeWithPerms("0xnode", "0xowner", "sgi-noshare", "rwds")
	n.AuthzDataPack(&other) // signed with the caller's own key, so the MAC is valid
	if AuthzDataUnpackADString(n.AuthzData, other, "r") != nil {
		t.Error("non-owner without an SGI grant must be denied even with a valid token")
	}
}

// With an SGI grant, access is gated by the exact permission bits in the token, and
// every required bit must be present.
func TestAuthzDataUnpackSharePermMatrix(t *testing.T) {
	uid, sgi := "0xshareuser", "sgi-matrix"
	other := sec.UserAuthData{Uid: uid, Role: "user", SecretKey: newKey(t)}
	n := nodeWithPerms("0xnode", "0xowner", sgi, "rw") // grants r,w only
	n.AuthzDataPack(&other)
	ads := n.AuthzData
	state.UsmUserAllowlistSgi(uid, sgi)

	cases := map[string]bool{
		"": true, "r": true, "w": true, "rw": true,
		"s": false, "d": false, "rd": false, // one missing bit fails the whole set
	}
	for perm, want := range cases {
		if got := AuthzDataUnpackADString(ads, other, perm) != nil; got != want {
			t.Errorf("required perms %q: granted=%v, want %v", perm, got, want)
		}
	}
}

// Inbound node updates must match their signed token: a caller cannot flip a
// permission bit or reassign ownership and still have the node accepted.
func TestAuthzDataUnpackNodeSliceDetectsTampering(t *testing.T) {
	owner := &sec.UserAuthData{Uid: "0xowner", Role: "user", SecretKey: newKey(t)}

	honest := nodeWithPerms("0xn1", "0xowner", "sgi-t", "rw")
	honest.AuthzDataPack(owner)
	ok := []*GraphNode{honest}
	if !AuthzDataUnpackNodeSlice(&ok, *owner, "w") {
		t.Error("owner's node with matching authz fields should be accepted")
	}

	// valid token, but PermDelete flipped on after packing
	tampered := nodeWithPerms("0xn2", "0xowner", "sgi-t", "rw")
	tampered.AuthzDataPack(owner)
	yes := true
	tampered.PermDelete = &yes
	bad := []*GraphNode{tampered}
	if AuthzDataUnpackNodeSlice(&bad, *owner, "w") {
		t.Error("node whose live perms differ from its signed token must be rejected")
	}

	// valid token for a victim-owned node, owner field reassigned to the caller
	stolen := nodeWithPerms("0xn3", "0xvictim", "sgi-t", "rw")
	stolen.AuthzDataPack(owner)
	stolen.Owner = &GraphUser{GraphBase: GraphBase{Uid: "0xowner"}}
	stolenSlice := []*GraphNode{stolen}
	if AuthzDataUnpackNodeSlice(&stolenSlice, *owner, "w") {
		t.Error("node with a tampered owner field must be rejected")
	}
}

// Empty / nil ad slices must fail closed rather than silently authorizing.
func TestAuthzDataUnpackSliceFailClosedOnEmpty(t *testing.T) {
	uad := sec.UserAuthData{Uid: "0xu", Role: "user", SecretKey: newKey(t)}
	empty := []string{}
	if AuthzDataUnpackADStringSlice(&empty, uad, "r") {
		t.Error("empty ad slice must return false (fail-closed)")
	}
	if AuthzDataUnpackADStringSlice(nil, uad, "r") {
		t.Error("nil ad slice must return false (fail-closed)")
	}
}
