package requests

import (
	"testing"

	cm "cogged/models"
	sec "cogged/security"
)

func TestLoginRequestValidate(t *testing.T) {
	cases := []struct {
		name string
		u    string
		want bool
	}{
		{"ok", "alice", true},
		{"empty", "", false},
		{"tilde-reserved", "~system", false},
	}
	for _, c := range cases {
		lr := &LoginRequest{Username: c.u, Password: "irrelevant"}
		if got := lr.Validate(); got != c.want {
			t.Errorf("%s: Validate() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCreateUserRequestValidate(t *testing.T) {
	cases := []struct {
		name string
		u, p string
		want bool
	}{
		{"ok", "bob", "longenough", true},
		{"empty-user", "", "longenough", false},
		{"tilde-user", "~bob", "longenough", false},
		{"short-pass-boundary", "bob", "1234", false}, // len 4, needs > 4
		{"min-pass", "bob", "12345", true},            // len 5
	}
	for _, c := range cases {
		r := &CreateUserRequest{Username: c.u, Password: c.p, Role: "user"}
		if got := r.Validate(); got != c.want {
			t.Errorf("%s: Validate() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCheckUidsArePlaceholders(t *testing.T) {
	ph := func(uid string, edges ...*cm.GraphNode) *cm.GraphNode {
		n := cm.NewGraphNodeJustUID(uid)
		if len(edges) > 0 {
			e := append([]*cm.GraphNode{}, edges...)
			n.OutEdges = &e
		}
		return n
	}

	valid := []*cm.GraphNode{ph("$1", ph("$2"), ph("$3"))}
	if !CheckUidsArePlaceholders(&valid) {
		t.Error("all-placeholder tree should validate")
	}

	badRoot := []*cm.GraphNode{ph("0x5")}
	if CheckUidsArePlaceholders(&badRoot) {
		t.Error("real uid at root should fail")
	}

	badChild := []*cm.GraphNode{ph("$1", ph("0x9"))}
	if CheckUidsArePlaceholders(&badChild) {
		t.Error("real uid in a child edge should fail")
	}

	// Regression: JSON null in "nodes" or "e" decodes to a nil *GraphNode. Dereferencing
	// it here used to panic the handler before validation could reject the request.
	nilRoot := []*cm.GraphNode{nil}
	if CheckUidsArePlaceholders(&nilRoot) {
		t.Error("a null node should fail validation")
	}

	nilChild := []*cm.GraphNode{ph("$1", nil)}
	if CheckUidsArePlaceholders(&nilChild) {
		t.Error("a null out-edge should fail validation")
	}

	mixed := []*cm.GraphNode{ph("$1"), nil}
	if CheckUidsArePlaceholders(&mixed) {
		t.Error("a null alongside valid nodes should fail validation")
	}
}

func TestCreateNodesRequestValidate(t *testing.T) {
	empty := &CreateNodesRequest{}
	if empty.Validate() {
		t.Error("nil/empty node list should not validate")
	}
	nodes := []*cm.GraphNode{cm.NewGraphNodeJustUID("$root")}
	ok := &CreateNodesRequest{Nodes: &nodes}
	if !ok.Validate() {
		t.Error("single placeholder node should validate")
	}

	// {"nodes":[null]} must be rejected, not crash the handler.
	nullNode := []*cm.GraphNode{nil}
	if (&CreateNodesRequest{Nodes: &nullNode}).Validate() {
		t.Error("a request containing a null node should not validate")
	}
}

func TestQueryRequestAuthzGating(t *testing.T) {
	sysUAD := sec.UserAuthData{Uid: "0x1", Role: sec.SYS_ROLE}
	userUAD := sec.UserAuthData{Uid: "0x2", Role: "user"}
	clause := &QueryRequestClause{Field: "ty", Op: "eq", Val: "x"}

	// non-sys supplying a root query is rejected
	rq := &QueryRequest{RootQuery: clause}
	if rq.AuthzDataUnpack(userUAD, "r") {
		t.Error("non-sys user must not be allowed a root query")
	}
	// sys supplying a root query with no root ids is allowed
	if !rq.AuthzDataUnpack(sysUAD, "r") {
		t.Error("sys user should be allowed a root query")
	}
	// empty root ids and no root query -> allowed (e.g. /user/nodes/shared)
	empty := &QueryRequest{}
	if !empty.AuthzDataUnpack(userUAD, "r") {
		t.Error("empty root ids should be allowed")
	}
}
