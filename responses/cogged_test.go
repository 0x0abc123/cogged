package responses

import (
	"os"
	"testing"

	cm "cogged/models"
	sec "cogged/security"
	state "cogged/state"
)

// TestMain boots the in-memory session manager once: the shared-node path in
// AuthzDataPack calls state.UsmUserCanAccessSgi, which sends on a channel UsmRun creates.
func TestMain(m *testing.M) {
	state.UsmInit()
	state.UsmRun()
	os.Exit(m.Run())
}

func b(v bool) *bool     { return &v }
func s(v string) *string { return &v }

func newKey(t *testing.T) string {
	t.Helper()
	k, err := sec.GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("rand: %v", err)
	}
	return sec.B64Encode(k)
}

// readableNode builds a shared, readable node owned by owner, carrying private data.
func readableNode(uid, owner, sgi string) *cm.GraphNode {
	n := cm.NewGraphNodeJustUID(uid)
	n.Owner = &cm.GraphUser{GraphBase: cm.GraphBase{Uid: owner}}
	n.Sgi = s(sgi)
	n.PermRead = b(true)
	n.PrivateData = s("owner secret")
	n.String1 = s("visible")
	return n
}

func TestAuthzDataPackKeepsPrivateDataForOwner(t *testing.T) {
	uad := &sec.UserAuthData{Uid: "0xowner", Role: "user", SecretKey: newKey(t)}
	n := readableNode("0xabc", "0xowner", "sgi-1")

	resp := &CoggedResponse{ResultNodes: []*cm.GraphNode{n}}
	resp.AuthzDataPack(uad)

	if len(resp.ResultNodes) != 1 {
		t.Fatalf("owner should see their own node, got %d nodes", len(resp.ResultNodes))
	}
	if resp.ResultNodes[0].PrivateData == nil || *resp.ResultNodes[0].PrivateData != "owner secret" {
		t.Error("owner must still see p")
	}
}

func TestAuthzDataPackKeepsPrivateDataForAdmin(t *testing.T) {
	uad := &sec.UserAuthData{Uid: "0xadmin", Role: sec.SYS_ROLE, SecretKey: newKey(t)}
	n := readableNode("0xabc", "0xowner", "sgi-1")

	resp := &CoggedResponse{ResultNodes: []*cm.GraphNode{n}}
	resp.AuthzDataPack(uad)

	if len(resp.ResultNodes) != 1 {
		t.Fatalf("admin should see the node, got %d nodes", len(resp.ResultNodes))
	}
	if resp.ResultNodes[0].PrivateData == nil || *resp.ResultNodes[0].PrivateData != "owner secret" {
		t.Error("sys-role user must still see p")
	}
}

// The regression this file exists for: a node reached through a share edge is readable,
// but its p predicate belongs to the owner and must not be returned.
func TestAuthzDataPackStripsPrivateDataForSharedReader(t *testing.T) {
	state.UsmUserAllowlistSgi("0xbob", "sgi-1")
	uad := &sec.UserAuthData{Uid: "0xbob", Role: "user", SecretKey: newKey(t)}
	n := readableNode("0xabc", "0xowner", "sgi-1")

	resp := &CoggedResponse{ResultNodes: []*cm.GraphNode{n}}
	resp.AuthzDataPack(uad)

	if len(resp.ResultNodes) != 1 {
		t.Fatalf("shared reader should still get the node, got %d nodes", len(resp.ResultNodes))
	}
	got := resp.ResultNodes[0]
	if got.PrivateData != nil {
		t.Errorf("p must be stripped for a non-owner, got %q", *got.PrivateData)
	}
	if got.String1 == nil || *got.String1 != "visible" {
		t.Error("stripping p must not affect the other data fields")
	}
	if got.AuthzData == "" {
		t.Error("node should still be AuthzData-packed")
	}
}

func TestAuthzDataPackStripsPrivateDataOnOutEdges(t *testing.T) {
	state.UsmUserAllowlistSgi("0xbob", "sgi-1")
	uad := &sec.UserAuthData{Uid: "0xbob", Role: "user", SecretKey: newKey(t)}

	child := readableNode("0xchild", "0xowner", "sgi-1")
	parent := readableNode("0xparent", "0xowner", "sgi-1")
	parent.OutEdges = &[]*cm.GraphNode{child}

	resp := &CoggedResponse{ResultNodes: []*cm.GraphNode{parent}}
	resp.AuthzDataPack(uad)

	if len(resp.ResultNodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(resp.ResultNodes))
	}
	edges := resp.ResultNodes[0].OutEdges
	if edges == nil || len(*edges) != 1 {
		t.Fatal("out-edge missing from response")
	}
	if (*edges)[0].PrivateData != nil {
		t.Error("p must be stripped from out-edge nodes too")
	}
}

// A node the caller can neither own nor reach through a share group is dropped
// entirely, p included.
func TestAuthzDataPackDropsUnreadableNode(t *testing.T) {
	uad := &sec.UserAuthData{Uid: "0xmallory", Role: "user", SecretKey: newKey(t)}
	n := readableNode("0xabc", "0xowner", "sgi-unshared")

	resp := &CoggedResponse{ResultNodes: []*cm.GraphNode{n}}
	resp.AuthzDataPack(uad)

	if len(resp.ResultNodes) != 0 {
		t.Fatalf("unreadable node should be dropped, got %d nodes", len(resp.ResultNodes))
	}
}
