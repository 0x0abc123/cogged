package services

import (
	"os"
	"strings"
	"testing"

	cm "cogged/models"
	req "cogged/requests"
)

// TestMain initialises the package-level regexes that NewDB would normally set up,
// so pure helpers can be tested without a database.
func TestMain(m *testing.M) {
	initGlobal()
	os.Exit(m.Run())
}

func TestSanitiseUID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0x1a", "0x1a"},
		{"1a", "0x1a"},
		{"0x0", "0x0"},
		{"", "0x0"},
		{"0xFF", "0xff"},
		{"notavaliduid", "0x0"}, // ParseUint fails -> 0
	}
	for _, c := range cases {
		if got := SanitiseUID(c.in); got != c.want {
			t.Errorf("SanitiseUID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidateUid(t *testing.T) {
	valid := []string{"0x1", "0xabc123", "0X1A"} // ToLower applied internally
	for _, v := range valid {
		if !ValidateUid(v) {
			t.Errorf("ValidateUid(%q) = false, want true", v)
		}
	}
	invalid := []string{"1a", "0xg", "", "0x", "abc"}
	for _, v := range invalid {
		if ValidateUid(v) {
			t.Errorf("ValidateUid(%q) = true, want false", v)
		}
	}
}

func TestRenderOpAndField(t *testing.T) {
	if renderOp("EQ") != "eq" {
		t.Error("renderOp should lowercase known ops")
	}
	if renderOp("bogus") != OP_TEXTSEARCH {
		t.Errorf("renderOp of unknown op = %q, want fallback %q", renderOp("bogus"), OP_TEXTSEARCH)
	}
	if renderField("TY") != "ty" {
		t.Error("renderField should lowercase and accept allowed field")
	}
	if renderField("evil") != "" {
		t.Error("renderField of disallowed field should be empty")
	}
}

func TestRenderFields(t *testing.T) {
	got := renderFields([]string{"id", "ty", "e", "notallowed"})
	if !strings.Contains(got, "id") || !strings.Contains(got, "ty") {
		t.Errorf("renderFields dropped allowed fields: %q", got)
	}
	if strings.Contains(got, "notallowed") {
		t.Errorf("renderFields kept a disallowed field: %q", got)
	}
	// 'e' expands to include the edge subselection
	if !strings.Contains(got, "e {uid own {uid} sgi r w o i d s}") {
		t.Errorf("renderFields did not expand edge field: %q", got)
	}
}

func TestConstructQueryStringEqClause(t *testing.T) {
	vars := map[string]string{}
	clause := req.QueryRequestClause{Field: "ty", Op: "eq", Val: "chat"}
	got := constructQueryStringAndAddVars(clause, &vars)
	if !strings.HasPrefix(got, "eq(ty,$vv") {
		t.Errorf("clause string = %q, want eq(ty,$vv...)", got)
	}
	// the value must be bound as a query variable
	found := false
	for k, v := range vars {
		if strings.HasPrefix(k, "$vv") && v == "chat" {
			found = true
		}
	}
	if !found {
		t.Errorf("value 'chat' not bound in vars: %v", vars)
	}
}

func TestConstructQueryStringTextSearchBecomesRegexp(t *testing.T) {
	vars := map[string]string{}
	clause := req.QueryRequestClause{Field: "s1", Op: "has", Val: "hello"}
	got := constructQueryStringAndAddVars(clause, &vars)
	if !strings.HasPrefix(got, "regexp(s1,$vv") {
		t.Errorf("text search with len>2 should become regexp, got %q", got)
	}
	found := false
	for _, v := range vars {
		if v == "/hello/i" {
			found = true
		}
	}
	if !found {
		t.Errorf("regex value not bound: %v", vars)
	}
}

func TestConstructQueryStringNestedAnd(t *testing.T) {
	vars := map[string]string{}
	clause := req.QueryRequestClause{
		And: []req.QueryRequestClause{
			{Field: "ty", Op: "eq", Val: "msg"},
			{Field: "s1", Op: "eq", Val: "hi"},
		},
	}
	got := constructQueryStringAndAddVars(clause, &vars)
	if !strings.HasPrefix(got, "(") || !strings.Contains(got, " and ") {
		t.Errorf("nested AND string = %q, want (... and ...)", got)
	}
}

func TestRenderQueryVarsString(t *testing.T) {
	single := map[string]string{"$vvAAA": "x", "$ids": "ignored"}
	if got := renderQueryVarsString(&single); got != "$vvAAA: string" {
		t.Errorf("single var = %q, want '$vvAAA: string'", got)
	}
	empty := map[string]string{"$ids": "only-non-vv"}
	if got := renderQueryVarsString(&empty); got != "" {
		t.Errorf("no $vv vars should render empty, got %q", got)
	}
	two := map[string]string{"$vvAAA": "x", "$vvBBB": "y"}
	got := renderQueryVarsString(&two)
	if !strings.Contains(got, "$vvAAA: string") || !strings.Contains(got, "$vvBBB: string") {
		t.Errorf("two vars = %q, want both declared as string", got)
	}
}

func TestSliceFromResultJSON(t *testing.T) {
	j := `{"qr":[{"uid":"0x1"},{"uid":"0x2"}]}`
	got := SliceFromResultJSON[cm.GraphNode](&j)
	if got == nil {
		t.Fatal("SliceFromResultJSON returned nil for valid input")
	}
	if len(*got) != 2 {
		t.Fatalf("got %d nodes, want 2", len(*got))
	}
	if (*got)[0].Uid != "0x1" || (*got)[1].Uid != "0x2" {
		t.Errorf("unexpected uids: %q %q", (*got)[0].Uid, (*got)[1].Uid)
	}
}

func TestGetUidOrSafeTempUid(t *testing.T) {
	if got := getUidOrSafeTempUid("0x1a"); got != "0x1a" {
		t.Errorf("valid uid should pass through, got %q", got)
	}
	got := getUidOrSafeTempUid("my-temp-key")
	if !strings.HasPrefix(got, "_:") {
		t.Errorf("non-uid should become a _: temp key, got %q", got)
	}
}

func TestMakeTempKeyFromStringStable(t *testing.T) {
	m := map[string]string{}
	a := MakeTempKeyFromString("alpha", &m)
	b := MakeTempKeyFromString("alpha", &m)
	if a != b {
		t.Errorf("same input should map to same temp key: %q vs %q", a, b)
	}
	c := MakeTempKeyFromString("beta", &m)
	if c == a {
		t.Errorf("different inputs should map to different temp keys")
	}
	if !strings.HasPrefix(a, "_") {
		t.Errorf("temp key should be prefixed with _, got %q", a)
	}
}

func TestRenderPagination(t *testing.T) {
	i := func(n int) *int { return &n }
	s := func(v string) *string { return &v }

	if got := renderPagination(&req.QueryRequest{}); got != "" {
		t.Errorf("no pagination should render empty, got %q", got)
	}
	// offset-based, ascending order
	got := renderPagination(&req.QueryRequest{First: i(10), Offset: i(20), OrderBy: s("c")})
	if got != ", orderasc: c, first: 10, offset: 20" {
		t.Errorf("offset pagination = %q", got)
	}
	// cursor-based, descending order; After is sanitised to a 0x uid
	got = renderPagination(&req.QueryRequest{First: i(5), After: s("2a"), OrderBy: s("m"), OrderDesc: true})
	if got != ", orderdesc: m, first: 5, after: 0x2a" {
		t.Errorf("cursor pagination = %q", got)
	}
	// a disallowed order predicate is dropped (not injected)
	got = renderPagination(&req.QueryRequest{OrderBy: s("evil; drop"), First: i(1)})
	if got != ", first: 1" {
		t.Errorf("disallowed order field should be dropped, got %q", got)
	}
}

func TestEscapeAndRegex(t *testing.T) {
	// alphanumerics and spaces pass through untouched
	if got := escapeAllNonAlphanumOrSpaceChars("abc 123"); got != "abc 123" {
		t.Errorf("alnum/space should be unescaped, got %q", got)
	}
	// special chars get backslash-escaped
	got := escapeAllNonAlphanumOrSpaceChars("a.b")
	if got != "a\\.b" {
		t.Errorf("special char should be escaped, got %q", got)
	}
	if r := createRegex("hi"); r != "/hi/i" {
		t.Errorf("createRegex = %q, want /hi/i", r)
	}
}
