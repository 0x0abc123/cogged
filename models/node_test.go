package models

import (
	"os"
	"testing"

	sec "cogged/security"
	state "cogged/state"
)

// TestMain boots the in-memory session manager once, because the authz deny/shared
// paths call state.UsmUserCanAccessSgi, which sends on a channel that UsmRun creates.
func TestMain(m *testing.M) {
	state.UsmInit()
	state.UsmRun()
	os.Exit(m.Run())
}

func newKey(t *testing.T) string {
	t.Helper()
	b, err := sec.GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("rand: %v", err)
	}
	return sec.B64Encode(b)
}

func b(v bool) *bool     { return &v }
func s(v string) *string { return &v }

// nodeWithPerms builds a node with an owner, sgi and the given permission letters set true.
func nodeWithPerms(uid, owner, sgi, perms string) *GraphNode {
	n := NewGraphNodeJustUID(uid)
	n.Owner = &GraphUser{GraphBase: GraphBase{Uid: owner}}
	n.Sgi = s(sgi)
	set := func(c byte) *bool { return b(indexByte(perms, c)) }
	n.PermRead = set('r')
	n.PermWrite = set('w')
	n.PermOutEdge = set('o')
	n.PermInEdge = set('i')
	n.PermDelete = set('d')
	n.PermShare = set('s')
	return n
}

func indexByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

func TestAuthzDataPackUnpackRoundTrip(t *testing.T) {
	key := newKey(t)
	uad := &sec.UserAuthData{Uid: "0xowner", Role: "user", SecretKey: key}

	orig := nodeWithPerms("0xabc", "0xowner", "sgi-1", "rws")
	orig.AuthzDataPack(uad)
	if orig.AuthzData == "" {
		t.Fatal("AuthzDataPack produced empty ad")
	}

	got := GraphNodeFromAD(orig.AuthzData, key)
	if got == nil {
		t.Fatal("GraphNodeFromAD returned nil for a genuine token")
	}
	if got.Uid != "0xabc" {
		t.Errorf("uid = %q, want 0xabc", got.Uid)
	}
	if got.Owner == nil || got.Owner.Uid != "0xowner" {
		t.Errorf("owner = %+v, want 0xowner", got.Owner)
	}
	if got.Sgi == nil || *got.Sgi != "sgi-1" {
		t.Errorf("sgi = %v, want sgi-1", got.Sgi)
	}
	// only r, w, s were set
	if !got.HasRequiredPermissions("rws") {
		t.Error("expected r,w,s permissions to be present")
	}
	if got.HasRequiredPermissions("d") {
		t.Error("delete permission should not be present")
	}
}

func TestAuthzDataRejectsTamperAndWrongKey(t *testing.T) {
	key := newKey(t)
	uad := &sec.UserAuthData{Uid: "0xowner", Role: "user", SecretKey: key}
	n := nodeWithPerms("0xabc", "0xowner", "sgi-1", "r")
	n.AuthzDataPack(uad)

	if GraphNodeFromAD(n.AuthzData, newKey(t)) != nil {
		t.Error("token verified under a different key")
	}
	if GraphNodeFromAD(n.AuthzData+"x", key) != nil {
		t.Error("tampered token should not verify")
	}
	if DecodeAndVerifyAD("no-dot-here", key) != "" {
		t.Error("malformed ad should decode to empty string")
	}
}

func TestAuthzDataPackPropagatesToEdges(t *testing.T) {
	key := newKey(t)
	uad := &sec.UserAuthData{Uid: "0xowner", Role: "user", SecretKey: key}
	child := nodeWithPerms("0xchild", "0xowner", "sgi-1", "r")
	parent := nodeWithPerms("0xparent", "0xowner", "sgi-1", "r")
	parent.OutEdges = &[]*GraphNode{child}

	parent.AuthzDataPack(uad)
	if child.AuthzData == "" {
		t.Error("AuthzDataPack should recurse into out-edges")
	}
}

func TestAuthzDataUnpackOwnerAndSysPaths(t *testing.T) {
	key := newKey(t)
	owner := &sec.UserAuthData{Uid: "0xowner", Role: "user", SecretKey: key}
	n := nodeWithPerms("0xabc", "0xowner", "sgi-1", "") // no perms at all
	n.AuthzDataPack(owner)
	ads := n.AuthzData

	// owner may access even with no permission bits set
	if AuthzDataUnpackADString(ads, *owner, "rwd") == nil {
		t.Error("owner should be granted access regardless of perms")
	}

	// sys role may access anyone's node; its per-user key must match how the ad was signed,
	// so re-pack under a sys user and unpack as that same sys user.
	sysUser := &sec.UserAuthData{Uid: "0xroot", Role: sec.SYS_ROLE, SecretKey: key}
	nsys := nodeWithPerms("0xdef", "0xsomeoneelse", "sgi-2", "")
	nsys.AuthzDataPack(sysUser)
	if AuthzDataUnpackADString(nsys.AuthzData, *sysUser, "rwd") == nil {
		t.Error("sys role should be granted access to any node")
	}
}

func TestAuthzDataUnpackSharedPath(t *testing.T) {
	key := newKey(t)
	// A non-owner, non-sys user: access depends on SGI allowlist + required perms.
	other := &sec.UserAuthData{Uid: "0xother", Role: "user", SecretKey: key}
	n := nodeWithPerms("0xabc", "0xowner", "shared-sgi", "r")
	n.AuthzDataPack(other) // signed with other's key so the MAC verifies for them
	ads := n.AuthzData

	// not yet allowlisted -> denied
	if AuthzDataUnpackADString(ads, *other, "r") != nil {
		t.Error("user without SGI access should be denied")
	}

	state.UsmUserAllowlistSgi("0xother", "shared-sgi")

	// now allowlisted, node has 'r', requiring 'r' -> allowed
	if AuthzDataUnpackADString(ads, *other, "r") == nil {
		t.Error("user with SGI access and sufficient perms should be granted")
	}
	// requiring 'w' which the node lacks -> denied
	if AuthzDataUnpackADString(ads, *other, "w") != nil {
		t.Error("user lacking required 'w' perm should be denied")
	}
}

func TestHasRequiredPermissions(t *testing.T) {
	n := nodeWithPerms("0x1", "0xo", "g", "rw")
	cases := []struct {
		req  string
		want bool
	}{
		{"", true}, // no requirement
		{"r", true},
		{"rw", true},
		{"rwd", false},
		{"d", false},
	}
	for _, c := range cases {
		if got := n.HasRequiredPermissions(c.req); got != c.want {
			t.Errorf("HasRequiredPermissions(%q) = %v, want %v", c.req, got, c.want)
		}
	}
}

func TestAuthzFieldsAreEqual(t *testing.T) {
	a := nodeWithPerms("0x1", "0xo", "g", "rw")
	same := nodeWithPerms("0x1", "0xo", "g", "rw")
	diffPerm := nodeWithPerms("0x1", "0xo", "g", "r")
	diffOwner := nodeWithPerms("0x1", "0xother", "g", "rw")

	if !AuthzFieldsAreEqual(a, same) {
		t.Error("identical authz fields should be equal")
	}
	if AuthzFieldsAreEqual(a, diffPerm) {
		t.Error("differing permissions should not be equal")
	}
	if AuthzFieldsAreEqual(a, diffOwner) {
		t.Error("differing owner should not be equal")
	}
	if AuthzFieldsAreEqual(a, nil) {
		t.Error("nil comparand should not be equal")
	}
}
