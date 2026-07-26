package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	req "cogged/requests"
	sec "cogged/security"

	"github.com/dgraph-io/dgo/v250/protos/api"
)

// fakeClient is an in-memory DgraphClient that records what db.go sends it and
// returns canned responses, so DB-layer logic can be tested without a real Dgraph.
type fakeClient struct {
	queryJSON    []byte
	queryErr     error
	mutateResp   *api.Response
	mutateErr    error
	lastQuery    string
	lastVars     map[string]string
	lastMutation *api.Mutation
	alterOps     []*api.Operation
}

func (c *fakeClient) NewTxn() DgraphTxn { return &fakeTxn{c: c} }

func (c *fakeClient) Alter(ctx context.Context, op *api.Operation) error {
	c.alterOps = append(c.alterOps, op)
	return nil
}

type fakeTxn struct{ c *fakeClient }

func (t *fakeTxn) Query(ctx context.Context, q string) (*api.Response, error) {
	t.c.lastQuery = q
	return &api.Response{Json: t.c.queryJSON}, t.c.queryErr
}

func (t *fakeTxn) QueryWithVars(ctx context.Context, q string, vars map[string]string) (*api.Response, error) {
	t.c.lastQuery = q
	t.c.lastVars = vars
	return &api.Response{Json: t.c.queryJSON}, t.c.queryErr
}

func (t *fakeTxn) Mutate(ctx context.Context, mu *api.Mutation) (*api.Response, error) {
	t.c.lastMutation = mu
	if t.c.mutateResp != nil {
		return t.c.mutateResp, t.c.mutateErr
	}
	return &api.Response{}, t.c.mutateErr
}

func newFakeDB(fake *fakeClient) *DB {
	return NewDBWithClient(&Config{}, fake)
}

func TestMutateSetVsDelete(t *testing.T) {
	fake := &fakeClient{mutateResp: &api.Response{Uids: map[string]string{"a": "0x1"}}}
	db := newFakeDB(fake)

	payload := map[string]string{"uid": "0x1", "s1": "hi"}

	if _, err := db.Mutate(payload, ADD); err != nil {
		t.Fatalf("ADD mutate error: %v", err)
	}
	if fake.lastMutation.SetJson == nil || fake.lastMutation.DeleteJson != nil {
		t.Error("ADD should populate SetJson and leave DeleteJson nil")
	}
	if !fake.lastMutation.CommitNow {
		t.Error("mutations should CommitNow")
	}
	var decoded map[string]string
	if err := json.Unmarshal(fake.lastMutation.SetJson, &decoded); err != nil {
		t.Fatalf("SetJson not valid JSON: %v", err)
	}
	if decoded["s1"] != "hi" {
		t.Errorf("SetJson lost data: %v", decoded)
	}

	if _, err := db.Mutate(payload, DELETE); err != nil {
		t.Fatalf("DELETE mutate error: %v", err)
	}
	if fake.lastMutation.DeleteJson == nil || fake.lastMutation.SetJson != nil {
		t.Error("DELETE should populate DeleteJson and leave SetJson nil")
	}
}

func TestQueryUserBuildsVarsAndParses(t *testing.T) {
	fake := &fakeClient{queryJSON: []byte(`{"qr":[{"uid":"0x1","un":"alice","role":"user"}]}`)}
	db := newFakeDB(fake)

	resp, err := db.QueryUser("alice")
	if err != nil {
		t.Fatalf("QueryUser error: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error response: %q", resp.Error)
	}
	if resp.User == nil || resp.User.Uid != "0x1" {
		t.Fatalf("parsed user = %+v, want uid 0x1", resp.User)
	}
	if fake.lastVars["$username"] != "alice" {
		t.Errorf("expected $username bound to alice, got vars %v", fake.lastVars)
	}
	if !strings.Contains(fake.lastQuery, "eq(un, $username)") {
		t.Errorf("query did not filter on username: %q", fake.lastQuery)
	}
}

func TestQueryUserNoResult(t *testing.T) {
	fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	db := newFakeDB(fake)

	resp, _ := db.QueryUser("ghost")
	if resp.Error != "no result" {
		t.Errorf("expected 'no result' error, got %+v", resp)
	}
}

func TestQueryWithOptionsRecurseQuery(t *testing.T) {
	fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	db := newFakeDB(fake)

	q := &req.QueryRequest{
		RootIDs: []string{"0x11"},
		Depth:   5,
		Select:  []string{"id", "ty"},
		Filters: &req.QueryRequestClause{Field: "ty", Op: "eq", Val: "msg"},
	}
	resp := db.QueryWithOptions(q, NODENODE, nil, nil)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %q", resp.Error)
	}
	if !strings.Contains(fake.lastQuery, "@recurse(depth: $rdepth)") {
		t.Errorf("expected a recurse query, got: %q", fake.lastQuery)
	}
	if fake.lastVars["$rdepth"] != "5" {
		t.Errorf("expected depth var 5, got %v", fake.lastVars["$rdepth"])
	}
	if !strings.Contains(fake.lastVars["$ids"], "0x11") {
		t.Errorf("expected sanitized root id in $ids, got %q", fake.lastVars["$ids"])
	}
}

func TestQueryWithOptionsInjectsPagination(t *testing.T) {
	fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	db := newFakeDB(fake)

	first, offset := 10, 20
	order, after := "c", "0x2a"
	q := &req.QueryRequest{
		RootIDs: []string{"0x11"},
		Depth:   3,
		First:   &first,
		Offset:  &offset,
		OrderBy: &order,
		After:   &after,
	}
	resp := db.QueryWithOptions(q, NODENODE, nil, nil)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %q", resp.Error)
	}
	for _, want := range []string{"orderasc: c", "first: 10", "offset: 20", "after: 0x2a"} {
		if !strings.Contains(fake.lastQuery, want) {
			t.Errorf("query missing %q; got:\n%s", want, fake.lastQuery)
		}
	}
	// the pagination args must sit inside the qr func parens, before the @filter
	if !strings.Contains(fake.lastQuery, "first: 10, offset: 20, after: 0x2a)") {
		t.Errorf("pagination args not placed inside func(...): %s", fake.lastQuery)
	}
}

func TestQueryWithOptionsReadAuthzFilter(t *testing.T) {
	q := &req.QueryRequest{RootIDs: []string{"0x5"}, Depth: 2}

	// non-admin NODENODE with grants -> read-authz filter injected
	fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	reader := &sec.UserAuthData{Uid: "0x1a", Role: "user"}
	newFakeDB(fake).QueryWithOptions(q, NODENODE, reader, []string{"sgiA", "sgiB"})
	for _, want := range []string{"uid_in(own, 0x1a)", `eq(sgi, ["sgiA", "sgiB"])`, "eq(r, true)"} {
		if !strings.Contains(fake.lastQuery, want) {
			t.Errorf("read-authz query missing %q:\n%s", want, fake.lastQuery)
		}
	}

	// admin -> no read-authz filter
	fadmin := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	admin := &sec.UserAuthData{Uid: "0x1", Role: sec.SYS_ROLE}
	newFakeDB(fadmin).QueryWithOptions(q, NODENODE, admin, []string{"sgiA"})
	if strings.Contains(fadmin.lastQuery, "uid_in(own") {
		t.Errorf("admin query should have no read-authz filter:\n%s", fadmin.lastQuery)
	}

	// USERSHARE (owner-scoped edge) -> no read-authz filter even for a non-admin
	fshare := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	newFakeDB(fshare).QueryWithOptions(q, USERSHARE, reader, []string{"sgiA"})
	if strings.Contains(fshare.lastQuery, "uid_in(own") {
		t.Errorf("USERSHARE query should have no read-authz filter:\n%s", fshare.lastQuery)
	}
}

func TestQueryWithOptionsEmptyRootIDs(t *testing.T) {
	db := newFakeDB(&fakeClient{queryJSON: []byte(`{"qr":[]}`)})
	q := &req.QueryRequest{RootIDs: []string{}, Depth: 3}
	resp := db.QueryWithOptions(q, NODENODE, nil, nil)
	if resp.Error == "" {
		t.Error("empty root ids (non-root query) should return an error response")
	}
}
