package requests

import (
	"testing"

	cm "cogged/models"
	sec "cogged/security"
)

func reqKey(t *testing.T) string {
	t.Helper()
	b, err := sec.GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("rand: %v", err)
	}
	return sec.B64Encode(b)
}

// packOwnedNode returns an AuthzData token for a node owned by ownerUID, signed with
// uad's key (as the owner would have received it). Owner access bypasses the perm
// bits, so this is enough to exercise the request-layer wrappers without the session
// manager.
func packOwnedNode(uid, ownerUID string, uad *sec.UserAuthData) string {
	n := cm.NewGraphNodeJustUID(uid)
	n.Owner = &cm.GraphUser{GraphBase: cm.GraphBase{Uid: ownerUID}}
	sgi := "sgi-req"
	n.Sgi = &sgi
	n.AuthzDataPack(uad)
	return n.AuthzData
}

// EdgesRequest.AuthzDataUnpack authorizes each supplied id list, and (by current
// design) requires all three lists to be non-empty — a missing list fails closed.
func TestEdgesRequestAuthz(t *testing.T) {
	uad := sec.UserAuthData{Uid: "0xowner", Role: "user", SecretKey: reqKey(t)}
	tok := func(u string) string { return packOwnedNode(u, "0xowner", &uad) }

	full := &EdgesRequest{
		SubjectIds:  &[]string{tok("0xa")},
		IncomingIds: &[]string{tok("0xb")},
		OutgoingIds: &[]string{tok("0xc")},
	}
	if !full.AuthzDataUnpack(uad, "") {
		t.Error("edges request with all three owned lists should authorize")
	}

	// Single-direction request (only outgoing): the absent incoming list is optional
	// and must not fail closed.
	outOnly := &EdgesRequest{
		SubjectIds:  &[]string{tok("0xa")},
		OutgoingIds: &[]string{tok("0xc")},
	}
	if !outOnly.AuthzDataUnpack(uad, "") {
		t.Error("single-direction (outgoing-only) edge request should authorize")
	}
	inOnly := &EdgesRequest{
		SubjectIds:  &[]string{tok("0xa")},
		IncomingIds: &[]string{tok("0xb")},
	}
	if !inOnly.AuthzDataUnpack(uad, "") {
		t.Error("single-direction (incoming-only) edge request should authorize")
	}

	// A subject token signed by a different user must be rejected.
	other := sec.UserAuthData{Uid: "0xother", Role: "user", SecretKey: reqKey(t)}
	forged := &EdgesRequest{
		SubjectIds:  &[]string{packOwnedNode("0xa", "0xother", &other)},
		IncomingIds: &[]string{tok("0xb")},
		OutgoingIds: &[]string{tok("0xc")},
	}
	if forged.AuthzDataUnpack(uad, "") {
		t.Error("edges request with a foreign-signed subject token must be denied")
	}
}

// Validate requires subjects plus at least one edge direction (else the op is a no-op).
func TestEdgesRequestValidate(t *testing.T) {
	ids := func(v ...string) *[]string { s := append([]string{}, v...); return &s }

	cases := []struct {
		name string
		req  EdgesRequest
		want bool
	}{
		{"subject+outgoing", EdgesRequest{SubjectIds: ids("0xa"), OutgoingIds: ids("0xc")}, true},
		{"subject+incoming", EdgesRequest{SubjectIds: ids("0xa"), IncomingIds: ids("0xb")}, true},
		{"subject only (no direction)", EdgesRequest{SubjectIds: ids("0xa")}, false},
		{"no subject", EdgesRequest{OutgoingIds: ids("0xc")}, false},
		{"empty", EdgesRequest{}, false},
	}
	for _, c := range cases {
		if got := c.req.Validate(); got != c.want {
			t.Errorf("%s: Validate() = %v, want %v", c.name, got, c.want)
		}
	}
}

// The owner of a node may share it; a node token signed by another user is rejected.
func TestShareNodesRequestAuthz(t *testing.T) {
	uad := sec.UserAuthData{Uid: "0xowner", Role: "user", SecretKey: reqKey(t)}

	target := cm.NewGraphUser("0xtarget")
	role := "user"
	target.Role = &role
	target.AuthzDataPack(&uad) // user token minted for the caller

	ok := &ShareNodesRequest{
		Nodes: &[]string{packOwnedNode("0xnode", "0xowner", &uad)},
		Users: &[]string{target.AuthzData},
	}
	if !ok.AuthzDataUnpack(uad, "s") {
		t.Error("owner should be able to share their own node")
	}

	other := sec.UserAuthData{Uid: "0xother", Role: "user", SecretKey: reqKey(t)}
	forged := &ShareNodesRequest{
		Nodes: &[]string{packOwnedNode("0xnode", "0xother", &other)},
		Users: &[]string{target.AuthzData},
	}
	if forged.AuthzDataUnpack(uad, "s") {
		t.Error("share request with a node token signed by another user must be denied")
	}
}
