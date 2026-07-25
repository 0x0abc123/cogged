package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	req "cogged/requests"

	"github.com/dgraph-io/dgo/v210/protos/api"
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
	resp := db.QueryWithOptions(q, NODENODE)
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

func TestQueryWithOptionsEmptyRootIDs(t *testing.T) {
	db := newFakeDB(&fakeClient{queryJSON: []byte(`{"qr":[]}`)})
	q := &req.QueryRequest{RootIDs: []string{}, Depth: 3}
	resp := db.QueryWithOptions(q, NODENODE)
	if resp.Error == "" {
		t.Error("empty root ids (non-root query) should return an error response")
	}
}
