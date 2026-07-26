package spec

// Drift guard: fails if openapi3.yaml falls out of sync with the code. It derives the
// truth from the source — the handler route cases in api/*.go and the json tags on the
// GraphNode/GraphUser models — and compares against the spec's paths and schemas. It
// reads files only (no DB/network), so it runs in the normal `go test ./...` CI job.
//
// If this fails, update openapi3.yaml (see the /doc-sync command) — do not weaken the
// test to make it pass.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// repoRoot resolves the repository root from this test file's location.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	return filepath.Dir(filepath.Dir(thisFile)) // <root>/spec/drift_test.go -> <root>
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// --- routes ---

// codeRoutes returns the set of "METHOD /group/endpoint" derived from the handler
// switch cases in api/<group>.go. The group is the file name (auth, graph, ...).
func codeRoutes(t *testing.T, root string) map[string]bool {
	caseRe := regexp.MustCompile(`case "(GET|POST|PUT|PATCH|DELETE) (\w+)":`)
	groups := []string{"auth", "admin", "graph", "user"} // health is a catch-all, handled separately
	routes := map[string]bool{}
	for _, g := range groups {
		src := read(t, root, filepath.Join("api", g+".go"))
		for _, m := range caseRe.FindAllStringSubmatch(src, -1) {
			routes[m[1]+" /"+g+"/"+m[2]] = true
		}
	}
	if len(routes) == 0 {
		t.Fatal("no handler routes parsed from api/*.go — parser likely broken")
	}
	return routes
}

// specRoutes returns the set of "METHOD /group/endpoint" from openapi3.yaml, with any
// /{param} path segments stripped so it lines up with the handler endpoints.
func specRoutes(t *testing.T, spec string) map[string]bool {
	pathRe := regexp.MustCompile(`^  (/[^:\s]+):\s*$`)
	methodRe := regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`)
	paramRe := regexp.MustCompile(`/\{[^}]+\}`)

	routes := map[string]bool{}
	inPaths := false
	curPath := ""
	for _, line := range strings.Split(spec, "\n") {
		if line == "paths:" {
			inPaths = true
			continue
		}
		if !inPaths {
			continue
		}
		if len(line) > 0 && line[0] != ' ' { // next top-level key (e.g. components:)
			break
		}
		if m := pathRe.FindStringSubmatch(line); m != nil {
			curPath = paramRe.ReplaceAllString(m[1], "")
			continue
		}
		if m := methodRe.FindStringSubmatch(line); m != nil && curPath != "" {
			routes[strings.ToUpper(m[1])+" "+curPath] = true
		}
	}
	if len(routes) == 0 {
		t.Fatal("no paths parsed from openapi3.yaml — parser likely broken")
	}
	return routes
}

func TestOpenAPIRoutesMatchHandlers(t *testing.T) {
	root := repoRoot(t)
	spec := read(t, root, "openapi3.yaml")

	code := codeRoutes(t, root)
	specR := specRoutes(t, spec)

	for r := range code {
		if !specR[r] {
			t.Errorf("handler route %q has no matching path in openapi3.yaml", r)
		}
	}
	for r := range specR {
		if strings.HasPrefix(r, "GET /health/") || strings.HasPrefix(r, "POST /health/") {
			continue // health is a catch-all handler with no explicit case
		}
		if !code[r] {
			t.Errorf("openapi3.yaml documents %q but no handler implements it", r)
		}
	}
}

// --- model fields vs schema properties ---

var jsonTagRe = regexp.MustCompile("json:\"([a-zA-Z0-9._]+)")

// modelFields returns the client-facing json tags across the given model files,
// excluding the internal dgraph.type predicate.
func modelFields(t *testing.T, root string, files ...string) map[string]bool {
	out := map[string]bool{}
	for _, f := range files {
		for _, m := range jsonTagRe.FindAllStringSubmatch(read(t, root, f), -1) {
			if m[1] == "dgraph.type" {
				continue
			}
			out[m[1]] = true
		}
	}
	return out
}

// schemaProps returns the top-level property names of a named schema in the spec.
func schemaProps(t *testing.T, spec, schema string) map[string]bool {
	propRe := regexp.MustCompile(`^        ([a-zA-Z0-9_]+):`)
	schemaStart := regexp.MustCompile(`^    ` + regexp.QuoteMeta(schema) + `:\s*$`)
	nextSchema := regexp.MustCompile(`^    [A-Za-z0-9_]+:\s*$`)

	out := map[string]bool{}
	in := false
	for _, line := range strings.Split(spec, "\n") {
		if schemaStart.MatchString(line) {
			in = true
			continue
		}
		if in && nextSchema.MatchString(line) {
			break
		}
		if in {
			if m := propRe.FindStringSubmatch(line); m != nil {
				out[m[1]] = true
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no properties parsed for schema %q", schema)
	}
	return out
}

func TestOpenAPINodeAndUserSchemasMatchModels(t *testing.T) {
	root := repoRoot(t)
	spec := read(t, root, "openapi3.yaml")

	cases := []struct {
		schema string
		files  []string
	}{
		{"GraphNode", []string{"models/node.go", "models/base.go"}},
		{"GraphUser", []string{"models/user.go", "models/base.go"}},
	}
	for _, c := range cases {
		fields := modelFields(t, root, c.files...)
		props := schemaProps(t, spec, c.schema)
		for f := range fields {
			if !props[f] {
				t.Errorf("%s model field %q is not documented in the %s openapi schema", c.schema, f, c.schema)
			}
		}
		for p := range props {
			if !fields[p] {
				t.Errorf("%s openapi schema documents %q which is not a %s model field", c.schema, p, c.schema)
			}
		}
	}
}
