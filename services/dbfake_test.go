package services

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	cm "cogged/models"
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

func TestQueryWithOptionsSimilaritySearch(t *testing.T) {
	admin := &sec.UserAuthData{Role: sec.SYS_ROLE}

	// builds a similar_to root func with the given topK and inline vector
	fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	q := &req.QueryRequest{Similar: &req.QuerySimilarity{Vector: "[0.1, 0.2, 0.3]", TopK: 5}}
	if resp := newFakeDB(fake).QueryWithOptions(q, NODENODE, admin, nil); resp.Error != "" {
		t.Fatalf("unexpected error: %q", resp.Error)
	}
	if !strings.Contains(fake.lastQuery, `similar_to(vec, 5, "[0.1, 0.2, 0.3]")`) {
		t.Errorf("similarity query malformed:\n%s", fake.lastQuery)
	}

	// topK defaults to 10 when unset
	f2 := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	newFakeDB(f2).QueryWithOptions(&req.QueryRequest{Similar: &req.QuerySimilarity{Vector: "[1,2]"}}, NODENODE, admin, nil)
	if !strings.Contains(f2.lastQuery, "similar_to(vec, 10,") {
		t.Errorf("expected default topK 10, got:\n%s", f2.lastQuery)
	}

	// invalid / injection-y vectors are rejected before any query is built
	for _, bad := range []string{"not-a-vector", "[]", `[1]") @filter(eq(x,"y`, `[1;2]`} {
		f := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
		resp := newFakeDB(f).QueryWithOptions(&req.QueryRequest{Similar: &req.QuerySimilarity{Vector: bad}}, NODENODE, admin, nil)
		if resp.Error == "" {
			t.Errorf("vector %q should be rejected", bad)
		}
		if f.lastQuery != "" {
			t.Errorf("no query should be issued for invalid vector %q", bad)
		}
	}
}

// `p` is stripped from responses for non-owners, so it must not be usable as a filter
// either: eq(p, "guess") would otherwise confirm the value via the result set.
func TestQueryWithOptionsRejectsPrivateFieldFilter(t *testing.T) {
	reader := &sec.UserAuthData{Uid: "0x1a", Role: "user"}
	orderByPrivate := "p"

	cases := []struct {
		name string
		q    *req.QueryRequest
	}{
		{"top-level filter", &req.QueryRequest{
			RootIDs: []string{"0x5"},
			Filters: &req.QueryRequestClause{Field: "p", Op: "eq", Val: "secret"},
		}},
		{"nested in and", &req.QueryRequest{
			RootIDs: []string{"0x5"},
			Filters: &req.QueryRequestClause{And: []req.QueryRequestClause{
				{Field: "ty", Op: "eq", Val: "note"},
				{Or: []req.QueryRequestClause{{Field: "p", Op: "eq", Val: "secret"}}},
			}},
		}},
		{"case-insensitive", &req.QueryRequest{
			RootIDs: []string{"0x5"},
			Filters: &req.QueryRequestClause{Field: "P", Op: "eq", Val: "secret"},
		}},
		{"order_by", &req.QueryRequest{
			RootIDs: []string{"0x5"},
			OrderBy: &orderByPrivate,
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
			resp := newFakeDB(fake).QueryWithOptions(tc.q, NODENODE, reader, nil)
			if resp.Error == "" {
				t.Error("non-admin should be refused a query naming p")
			}
			if fake.lastQuery != "" {
				t.Errorf("no query should reach Dgraph, got:\n%s", fake.lastQuery)
			}
		})
	}
}

func TestQueryWithOptionsAllowsPrivateFieldForAdminAndInSelect(t *testing.T) {
	// admins may filter on p
	fadmin := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	admin := &sec.UserAuthData{Uid: "0x1", Role: sec.SYS_ROLE}
	resp := newFakeDB(fadmin).QueryWithOptions(&req.QueryRequest{
		RootIDs: []string{"0x5"},
		Filters: &req.QueryRequestClause{Field: "p", Op: "eq", Val: "secret"},
	}, NODENODE, admin, nil)
	if resp.Error != "" {
		t.Errorf("admin filter on p should be allowed, got %q", resp.Error)
	}
	if !strings.Contains(fadmin.lastQuery, "eq(p,") {
		t.Errorf("admin query should filter on p:\n%s", fadmin.lastQuery)
	}

	// a non-admin may still *select* p — the response layer strips it if they are not
	// the owner (see responses.CoggedResponse.AuthzDataPack).
	fuser := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	reader := &sec.UserAuthData{Uid: "0x1a", Role: "user"}
	resp = newFakeDB(fuser).QueryWithOptions(&req.QueryRequest{
		RootIDs: []string{"0x5"},
		Select:  []string{"id", "p"},
	}, NODENODE, reader, nil)
	if resp.Error != "" {
		t.Errorf("selecting p should be allowed, got %q", resp.Error)
	}
	if !strings.Contains(fuser.lastQuery, "id p") {
		t.Errorf("select should still include p:\n%s", fuser.lastQuery)
	}
}

func TestQueryWithOptionsGeoRadiusSearch(t *testing.T) {
	admin := &sec.UserAuthData{Role: sec.SYS_ROLE}

	// builds a near() root func with the point and radius inlined as plain decimals
	fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	q := &req.QueryRequest{Geo: &req.QueryGeo{Point: []float64{151.2153, -33.8568}, Distance: 5000}}
	if resp := newFakeDB(fake).QueryWithOptions(q, NODENODE, admin, nil); resp.Error != "" {
		t.Fatalf("unexpected error: %q", resp.Error)
	}
	if !strings.Contains(fake.lastQuery, "near(g, [151.2153, -33.8568], 5000)") {
		t.Errorf("geo query malformed:\n%s", fake.lastQuery)
	}

	// tiny and large magnitudes must not render in exponent notation, which DQL rejects
	fexp := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	newFakeDB(fexp).QueryWithOptions(&req.QueryRequest{
		Geo: &req.QueryGeo{Point: []float64{0.0000001, -0.0000002}, Distance: 12000000},
	}, NODENODE, admin, nil)
	if strings.ContainsAny(fexp.lastQuery, "eE") && strings.Contains(fexp.lastQuery, "e-") {
		t.Errorf("coordinates must not use exponent notation:\n%s", fexp.lastQuery)
	}
	if !strings.Contains(fexp.lastQuery, "near(g, [0.0000001, -0.0000002], 12000000)") {
		t.Errorf("small coordinates rendered wrong:\n%s", fexp.lastQuery)
	}

	// the read-authz filter still applies to a geo search for a non-admin
	fauthz := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	reader := &sec.UserAuthData{Uid: "0x1a", Role: "user"}
	newFakeDB(fauthz).QueryWithOptions(q, NODENODE, reader, []string{"sgiA"})
	for _, want := range []string{"near(g,", "uid_in(own, 0x1a)", `eq(sgi, ["sgiA"])`} {
		if !strings.Contains(fauthz.lastQuery, want) {
			t.Errorf("geo query missing %q:\n%s", want, fauthz.lastQuery)
		}
	}

	// pagination composes with the geo root func
	first := 5
	fpage := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	newFakeDB(fpage).QueryWithOptions(&req.QueryRequest{
		Geo:   &req.QueryGeo{Point: []float64{0, 0}, Distance: 100},
		First: &first,
	}, NODENODE, admin, nil)
	if !strings.Contains(fpage.lastQuery, "near(g, [0, 0], 100), first: 5)") {
		t.Errorf("pagination args not placed inside the geo func:\n%s", fpage.lastQuery)
	}
}

func TestQueryWithOptionsGeoValidation(t *testing.T) {
	admin := &sec.UserAuthData{Role: sec.SYS_ROLE}

	cases := []struct {
		name string
		geo  *req.QueryGeo
	}{
		{"empty point", &req.QueryGeo{Point: []float64{}, Distance: 100}},
		{"one coordinate", &req.QueryGeo{Point: []float64{1}, Distance: 100}},
		{"three coordinates", &req.QueryGeo{Point: []float64{1, 2, 3}, Distance: 100}},
		{"longitude too high", &req.QueryGeo{Point: []float64{180.1, 0}, Distance: 100}},
		{"longitude too low", &req.QueryGeo{Point: []float64{-180.1, 0}, Distance: 100}},
		// 100 is a valid longitude but not a valid latitude: catches lat/lon swapped
		{"latitude out of range", &req.QueryGeo{Point: []float64{0, 100}, Distance: 100}},
		{"zero distance", &req.QueryGeo{Point: []float64{0, 0}, Distance: 0}},
		{"negative distance", &req.QueryGeo{Point: []float64{0, 0}, Distance: -1}},
		{"distance over cap", &req.QueryGeo{Point: []float64{0, 0}, Distance: MAX_GEO_DISTANCE_METRES + 1}},
		{"NaN coordinate", &req.QueryGeo{Point: []float64{math.NaN(), 0}, Distance: 100}},
		{"Inf distance", &req.QueryGeo{Point: []float64{0, 0}, Distance: math.Inf(1)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
			resp := newFakeDB(fake).QueryWithOptions(&req.QueryRequest{Geo: tc.geo}, NODENODE, admin, nil)
			if resp.Error == "" {
				t.Error("invalid geo request should be rejected")
			}
			if fake.lastQuery != "" {
				t.Errorf("no query should reach Dgraph, got:\n%s", fake.lastQuery)
			}
		})
	}
}

// `g` cannot be expressed by any filter op, so naming it in filters or order_by is
// refused with a pointer at the geo block rather than an opaque "DB query failed".
func TestQueryWithOptionsRejectsGeoFieldInFilters(t *testing.T) {
	admin := &sec.UserAuthData{Role: sec.SYS_ROLE}
	orderByGeo := "g"

	cases := []struct {
		name string
		q    *req.QueryRequest
	}{
		{"filter on g", &req.QueryRequest{
			RootIDs: []string{"0x5"},
			Filters: &req.QueryRequestClause{Field: "g", Op: "eq", Val: "somewhere"},
		}},
		{"nested filter on g", &req.QueryRequest{
			RootIDs: []string{"0x5"},
			Filters: &req.QueryRequestClause{And: []req.QueryRequestClause{
				{Field: "ty", Op: "eq", Val: "place"},
				{Field: "G", Op: "has", Val: "somewhere"},
			}},
		}},
		{"order_by g", &req.QueryRequest{RootIDs: []string{"0x5"}, OrderBy: &orderByGeo}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
			resp := newFakeDB(fake).QueryWithOptions(tc.q, NODENODE, admin, nil)
			if resp.Error == "" {
				t.Error("naming g in filters/order_by should be refused")
			}
			if fake.lastQuery != "" {
				t.Errorf("no query should reach Dgraph, got:\n%s", fake.lastQuery)
			}
		})
	}

	// but g is still selectable
	fsel := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	resp := newFakeDB(fsel).QueryWithOptions(&req.QueryRequest{
		RootIDs: []string{"0x5"},
		Select:  []string{"id", "g"},
	}, NODENODE, admin, nil)
	if resp.Error != "" {
		t.Errorf("selecting g should be allowed, got %q", resp.Error)
	}
	if !strings.Contains(fsel.lastQuery, "id g") {
		t.Errorf("select should include g:\n%s", fsel.lastQuery)
	}
}

// A geo clause composes with ordinary filters and with a root_ids traversal, which the
// request-level geo block (a root function) cannot do.
func TestQueryWithOptionsGeoFilterClause(t *testing.T) {
	admin := &sec.UserAuthData{Role: sec.SYS_ROLE}
	geo := &req.QueryGeo{Point: []float64{151.2153, -33.8568}, Distance: 5000}

	// standalone geo clause over a uid traversal
	fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	resp := newFakeDB(fake).QueryWithOptions(&req.QueryRequest{
		RootIDs: []string{"0x5"},
		Depth:   3,
		Filters: &req.QueryRequestClause{Geo: geo},
	}, NODENODE, admin, nil)
	if resp.Error != "" {
		t.Fatalf("unexpected error: %q", resp.Error)
	}
	if !strings.Contains(fake.lastQuery, "@recurse(depth: $rdepth)") {
		t.Errorf("geo clause should not displace the traversal:\n%s", fake.lastQuery)
	}
	if !strings.Contains(fake.lastQuery, "@filter(near(g, [151.2153, -33.8568], 5000))") {
		t.Errorf("geo clause not compiled into the filter:\n%s", fake.lastQuery)
	}

	// ANDed with an ordinary clause
	fand := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	newFakeDB(fand).QueryWithOptions(&req.QueryRequest{
		RootIDs: []string{"0x5"},
		Filters: &req.QueryRequestClause{And: []req.QueryRequestClause{
			{Field: "ty", Op: "eq", Val: "cafe"},
			{Geo: geo},
		}},
	}, NODENODE, admin, nil)
	if !strings.Contains(fand.lastQuery, "(eq(ty,$vv") || !strings.Contains(fand.lastQuery, "and near(g, [151.2153, -33.8568], 5000))") {
		t.Errorf("geo clause did not AND with an ordinary filter:\n%s", fand.lastQuery)
	}

	// two radii ORed together, nested a level down
	for2 := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	newFakeDB(for2).QueryWithOptions(&req.QueryRequest{
		RootIDs: []string{"0x5"},
		Filters: &req.QueryRequestClause{Or: []req.QueryRequestClause{
			{Geo: geo},
			{Geo: &req.QueryGeo{Point: []float64{115.8575, -31.9505}, Distance: 100}},
		}},
	}, NODENODE, admin, nil)
	if !strings.Contains(for2.lastQuery, "(near(g, [151.2153, -33.8568], 5000) or near(g, [115.8575, -31.9505], 100))") {
		t.Errorf("two geo clauses did not OR:\n%s", for2.lastQuery)
	}

	// the read-authz filter is still ANDed on for a non-admin
	fauthz := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	newFakeDB(fauthz).QueryWithOptions(&req.QueryRequest{
		RootIDs: []string{"0x5"},
		Filters: &req.QueryRequestClause{Geo: geo},
	}, NODENODE, &sec.UserAuthData{Uid: "0x1a", Role: "user"}, []string{"sgiA"})
	for _, want := range []string{"near(g,", "uid_in(own, 0x1a)", `eq(sgi, ["sgiA"])`} {
		if !strings.Contains(fauthz.lastQuery, want) {
			t.Errorf("geo clause query missing %q:\n%s", want, fauthz.lastQuery)
		}
	}

	// a geo clause can also narrow a request-level geo root search (intersecting radii)
	fboth := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	resp = newFakeDB(fboth).QueryWithOptions(&req.QueryRequest{
		Geo:     geo,
		Filters: &req.QueryRequestClause{Geo: &req.QueryGeo{Point: []float64{151.2067, -33.8715}, Distance: 3000}},
	}, NODENODE, admin, nil)
	if resp.Error != "" {
		t.Fatalf("root geo + geo clause should be allowed, got %q", resp.Error)
	}
	if !strings.Contains(fboth.lastQuery, "func: near(g, [151.2153, -33.8568], 5000)") ||
		!strings.Contains(fboth.lastQuery, "@filter(near(g, [151.2067, -33.8715], 3000))") {
		t.Errorf("intersecting radii query malformed:\n%s", fboth.lastQuery)
	}
}

// Regression: a geo clause binds no query variable (it is inlined from parsed floats), so
// a query whose only filter is geo has an empty $vv list. The parameter list must not be
// left with a dangling comma — "query q($ids: string, $rdepth: int, )" is a DQL parse
// error, and it only shows up against a real Dgraph.
func TestQueryWithOptionsGeoClauseLeavesNoDanglingComma(t *testing.T) {
	admin := &sec.UserAuthData{Role: sec.SYS_ROLE}
	geo := &req.QueryGeo{Point: []float64{0, 0}, Distance: 100}

	shapes := map[string]*req.QueryRequest{
		"recurse":      {RootIDs: []string{"0x5"}, Depth: 3, Filters: &req.QueryRequestClause{Geo: geo}},
		"uid no-depth": {RootIDs: []string{"0x5"}, Filters: &req.QueryRequestClause{Geo: geo}},
		"geo root":     {Geo: geo, Filters: &req.QueryRequestClause{Geo: geo}},
	}

	for name, q := range shapes {
		t.Run(name, func(t *testing.T) {
			fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
			if resp := newFakeDB(fake).QueryWithOptions(q, NODENODE, admin, nil); resp.Error != "" {
				t.Fatalf("unexpected error: %q", resp.Error)
			}
			head := fake.lastQuery
			if i := strings.Index(head, ")"); i >= 0 {
				head = head[:i+1]
			}
			if strings.Contains(head, ", )") || strings.Contains(head, ",)") {
				t.Errorf("dangling comma in query params: %q", head)
			}
		})
	}

	// and the params are still correct when values *are* bound
	fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	newFakeDB(fake).QueryWithOptions(&req.QueryRequest{
		RootIDs: []string{"0x5"}, Depth: 3,
		Filters: &req.QueryRequestClause{And: []req.QueryRequestClause{
			{Field: "ty", Op: "eq", Val: "cafe"},
			{Geo: geo},
		}},
	}, NODENODE, admin, nil)
	if !strings.Contains(fake.lastQuery, "$ids: string, $rdepth: int, $vv") {
		t.Errorf("expected fixed params followed by the bound value:\n%s", fake.lastQuery)
	}
}

func TestQueryWithOptionsGeoFilterClauseValidation(t *testing.T) {
	admin := &sec.UserAuthData{Role: sec.SYS_ROLE}

	cases := []struct {
		name   string
		clause *req.QueryRequestClause
	}{
		{"invalid point in clause", &req.QueryRequestClause{
			Geo: &req.QueryGeo{Point: []float64{0, 91}, Distance: 100},
		}},
		{"invalid distance in clause", &req.QueryRequestClause{
			Geo: &req.QueryGeo{Point: []float64{0, 0}, Distance: 0},
		}},
		{"invalid geo nested in and", &req.QueryRequestClause{And: []req.QueryRequestClause{
			{Field: "ty", Op: "eq", Val: "cafe"},
			{Geo: &req.QueryGeo{Point: []float64{999, 0}, Distance: 100}},
		}}},
		{"invalid geo nested in or", &req.QueryRequestClause{Or: []req.QueryRequestClause{
			{Geo: &req.QueryGeo{Point: []float64{0, 0}, Distance: math.Inf(1)}},
		}}},
		// field/op/val on a geo clause would be silently dropped, so it is refused
		{"geo clause also sets field", &req.QueryRequestClause{
			Field: "ty", Op: "eq", Val: "cafe",
			Geo: &req.QueryGeo{Point: []float64{0, 0}, Distance: 100},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
			resp := newFakeDB(fake).QueryWithOptions(
				&req.QueryRequest{RootIDs: []string{"0x5"}, Filters: tc.clause}, NODENODE, admin, nil)
			if resp.Error == "" {
				t.Error("invalid geo clause should be rejected")
			}
			if fake.lastQuery != "" {
				t.Errorf("no query should reach Dgraph, got:\n%s", fake.lastQuery)
			}
		})
	}
}

func TestQueryWithOptionsGeoAndSimilarAreExclusive(t *testing.T) {
	fake := &fakeClient{queryJSON: []byte(`{"qr":[]}`)}
	resp := newFakeDB(fake).QueryWithOptions(&req.QueryRequest{
		Geo:     &req.QueryGeo{Point: []float64{0, 0}, Distance: 100},
		Similar: &req.QuerySimilarity{Vector: "[1,2]"},
	}, NODENODE, &sec.UserAuthData{Role: sec.SYS_ROLE}, nil)
	if resp.Error == "" {
		t.Error("combining geo and similar should be refused")
	}
	if fake.lastQuery != "" {
		t.Errorf("no query should reach Dgraph, got:\n%s", fake.lastQuery)
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

// UpsertNodes used to nil-deref — panicking the handler and dropping the connection — on
// two shapes an ordinary API request can produce. Each must now come back as an error
// response, with nothing written.
func TestUpsertNodesRejectsUnprocessableLists(t *testing.T) {
	ghost := func() *cm.GraphNode {
		n := cm.NewGraphNodeJustUID("$a")
		n.OutEdges = &[]*cm.GraphNode{cm.NewGraphNodeJustUID("$ghost")}
		return n
	}
	nilEdge := func() *cm.GraphNode {
		n := cm.NewGraphNodeJustUID("$a")
		n.OutEdges = &[]*cm.GraphNode{nil}
		return n
	}

	cases := []struct {
		name string
		list []*cm.GraphNode
	}{
		// an out-edge to a temp uid no node defines: Dgraph mints an ownerless orphan and
		// returns a uid that has no originating node to describe it
		{"edge to undefined temp uid", []*cm.GraphNode{ghost()}},
		{"null node", []*cm.GraphNode{nil}},
		{"null node among valid ones", []*cm.GraphNode{cm.NewGraphNodeJustUID("$a"), nil}},
		{"null out-edge", []*cm.GraphNode{nilEdge()}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeClient{mutateResp: &api.Response{Uids: map[string]string{}}}
			list := tc.list
			resp, err := newFakeDB(fake).UpsertNodes(&list)
			if err == nil {
				t.Error("expected an error")
			}
			if resp == nil || resp.Error == "" {
				t.Errorf("expected an error response, got %+v", resp)
			}
			if fake.lastMutation != nil {
				t.Error("nothing should have been written")
			}
		})
	}
}

func TestUpsertNodesAcceptsValidEdgeShapes(t *testing.T) {
	// a temp uid defined by another node in the same request, and an edge to an existing
	// node by real uid, are both fine
	child := cm.NewGraphNodeJustUID("$child")
	parent := cm.NewGraphNodeJustUID("$parent")
	parent.OutEdges = &[]*cm.GraphNode{cm.NewGraphNodeJustUID("$child"), cm.NewGraphNodeJustUID("0x2a")}
	list := []*cm.GraphNode{parent, child}

	fake := &fakeClient{mutateResp: &api.Response{Uids: map[string]string{
		sec.MD5SumHex([]byte("$parent")): "0x1",
		sec.MD5SumHex([]byte("$child")):  "0x2",
	}}}
	resp, err := newFakeDB(fake).UpsertNodes(&list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.CreatedNodes["$parent"] == nil || resp.CreatedNodes["$child"] == nil {
		t.Errorf("both new nodes should be in created_nodes, got %+v", resp.CreatedNodes)
	}
}

// Defence in depth: even if a uid with no originating node reaches the response loop, it
// must be skipped rather than dereferenced.
func TestUpsertNodesSkipsUnmappedMutationUid(t *testing.T) {
	list := []*cm.GraphNode{cm.NewGraphNodeJustUID("$a")}
	fake := &fakeClient{mutateResp: &api.Response{Uids: map[string]string{
		sec.MD5SumHex([]byte("$a")): "0x1",
		"a-key-nothing-maps-to":     "0x99",
	}}}

	resp, err := newFakeDB(fake).UpsertNodes(&list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.CreatedNodes["$a"] == nil || resp.CreatedNodes["$a"].Uid != "0x1" {
		t.Errorf("the known node should still be returned, got %+v", resp.CreatedNodes)
	}
	if len(resp.CreatedNodes) != 1 {
		t.Errorf("the unmapped uid should be skipped, got %+v", resp.CreatedNodes)
	}
}
