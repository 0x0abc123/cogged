package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"cogged/api"
	sec "cogged/security"
	state "cogged/state"
)

// TestMain boots the in-memory session manager once; the authentication path calls
// state.UsmCheckTokenId, which sends on a channel that UsmRun creates.
func TestMain(m *testing.M) {
	state.UsmInit()
	state.UsmRun()
	os.Exit(m.Run())
}

// newGatingHandler builds a DefaultHandler with only the allow/admin route sets populated.
// The auth/admin gating under test returns before any API handler is dispatched, so the
// zero-value handler fields are never touched and no database is required.
func newGatingHandler(allow, admin Set) *DefaultHandler {
	return &DefaultHandler{
		allowList: &allow,
		adminList: &admin,
	}
}

// newAuthHandler additionally wires an AuthAPI (secret key + expiry) so the full
// authentication path runs. "/auth/check" dispatches to AuthAPI and needs no database.
func newAuthHandler(key string, expiry int64) *DefaultHandler {
	allow := Set{"/health/status": true}
	admin := Set{"admin": true}
	return &DefaultHandler{
		auth:      api.AuthAPI{SecretKey: key, TokenExpiry: expiry},
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

func testSecret(t *testing.T) string {
	t.Helper()
	b, err := sec.GenerateRandomBytes(32)
	if err != nil {
		t.Fatal(err)
	}
	return sec.B64Encode(b)
}

func bearer(uid, role, tokenId string, issuedAtUnix int64, key string) string {
	return "Bearer " + sec.ConstructToken(uid, role, tokenId, fmt.Sprintf("%d", issuedAtUnix), key)
}

// --- route/content gating (no auth wired) ---

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
	h := newGatingHandler(Set{}, Set{"admin": true})
	rr := gatingRequest(t, h, "GET", "/graph/nodes", "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("no token on non-allowlisted route: got %d, want 401", rr.Code)
	}
}

// Regression test for the nil-deref in ServeHTTP: an unauthenticated request to an
// allowlisted path whose route group is admin-only must be denied with 401, not panic.
func TestServeHTTPAdminRouteWithNilAuthIsDeniedNotPanic(t *testing.T) {
	h := newGatingHandler(Set{"/admin/open": true}, Set{"admin": true})
	rr := gatingRequest(t, h, "GET", "/admin/open", "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("nil auth on admin route: got %d, want 401", rr.Code)
	}
}

// --- authentication path (full token validation via /auth/check) ---

func TestServeHTTPAuthValidToken(t *testing.T) {
	key := testSecret(t)
	h := newAuthHandler(key, 600)
	uid, tid := "0xu1", "tok-valid"
	state.UsmAddTokenId(uid, tid)
	rr := gatingRequest(t, h, "GET", "/auth/check", bearer(uid, "user", tid, time.Now().Unix(), key))
	if rr.Code != http.StatusOK {
		t.Errorf("valid token on /auth/check: got %d, want 200", rr.Code)
	}
}

func TestServeHTTPAuthTamperedToken(t *testing.T) {
	key := testSecret(t)
	h := newAuthHandler(key, 600)
	uid, tid := "0xu2", "tok-tamper"
	state.UsmAddTokenId(uid, tid)
	tok := bearer(uid, "user", tid, time.Now().Unix(), key)
	rr := gatingRequest(t, h, "GET", "/auth/check", tok+"x") // corrupt the MAC
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("tampered token: got %d, want 401", rr.Code)
	}
}

func TestServeHTTPAuthExpiredToken(t *testing.T) {
	key := testSecret(t)
	h := newAuthHandler(key, 600)
	uid, tid := "0xu3", "tok-exp"
	state.UsmAddTokenId(uid, tid)
	old := time.Now().Unix() - 10000 // far beyond the 600s expiry
	rr := gatingRequest(t, h, "GET", "/auth/check", bearer(uid, "user", tid, old, key))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expired token: got %d, want 401", rr.Code)
	}
}

func TestServeHTTPAuthTokenNotInSession(t *testing.T) {
	key := testSecret(t)
	h := newAuthHandler(key, 600)
	// valid MAC + fresh timestamp, but the token id was never registered (e.g. logged out)
	rr := gatingRequest(t, h, "GET", "/auth/check", bearer("0xu4", "user", "tok-ghost", time.Now().Unix(), key))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("token id absent from session: got %d, want 401", rr.Code)
	}
}

func TestServeHTTPAuthWrongMasterKey(t *testing.T) {
	h := newAuthHandler(testSecret(t), 600) // server holds one master secret
	uid, tid := "0xu5", "tok-wrongkey"
	state.UsmAddTokenId(uid, tid)
	// token signed with a *different* master secret
	rr := gatingRequest(t, h, "GET", "/auth/check", bearer(uid, "user", tid, time.Now().Unix(), testSecret(t)))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("token under wrong master key: got %d, want 401", rr.Code)
	}
}

// An authenticated but non-admin user must still be denied on an admin route group.
func TestServeHTTPAuthenticatedNonAdminDeniedOnAdminRoute(t *testing.T) {
	key := testSecret(t)
	h := newAuthHandler(key, 600)
	uid, tid := "0xu6", "tok-nonadmin"
	state.UsmAddTokenId(uid, tid)
	rr := gatingRequest(t, h, "GET", "/admin/whatever", bearer(uid, "user", tid, time.Now().Unix(), key))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("authenticated non-admin on admin route: got %d, want 401", rr.Code)
	}
}
