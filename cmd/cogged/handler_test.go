package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newGatingHandler builds a DefaultHandler with only the allow/admin route sets populated.
// The auth/admin gating under test returns before any API handler is dispatched, so the
// zero-value handler fields are never touched and no database is required.
func newGatingHandler(allow, admin Set) *DefaultHandler {
	return &DefaultHandler{
		allowList: &allow,
		adminList: &admin,
	}
}

func gatingRequest(t *testing.T, h *DefaultHandler, method, path, auth string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Content-Type", "application/json")
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

func TestServeHTTPRejectsNonJSON(t *testing.T) {
	h := newGatingHandler(Set{}, Set{})
	r := httptest.NewRequest("POST", "/health/status", nil) // no Content-Type header
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("missing JSON content-type: got %d, want 415", rr.Code)
	}
}

func TestServeHTTPUnauthenticatedRouteDenied(t *testing.T) {
	// No token and the path is not on the allowlist -> 401.
	h := newGatingHandler(Set{}, Set{"admin": true})
	rr := gatingRequest(t, h, "GET", "/graph/nodes", "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("no token on non-allowlisted route: got %d, want 401", rr.Code)
	}
}

// Regression test for the nil-deref in ServeHTTP: an unauthenticated request to an
// allowlisted path whose route group is admin-only must be denied with 401, not panic.
// The allowlist check lets a nil *UserAuthData through; the admin check must guard for it.
func TestServeHTTPAdminRouteWithNilAuthIsDeniedNotPanic(t *testing.T) {
	h := newGatingHandler(Set{"/admin/open": true}, Set{"admin": true})
	rr := gatingRequest(t, h, "GET", "/admin/open", "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("nil auth on admin route: got %d, want 401", rr.Code)
	}
}
