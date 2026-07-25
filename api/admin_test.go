package api

import (
	"os"
	"strings"
	"testing"

	cm "cogged/models"
	svc "cogged/services"
)

// TestMain initialises the services-package regexes used by svc.ValidateUid. NewDBWithClient
// triggers the same lazy global init NewDB would, without opening a connection.
func TestMain(m *testing.M) {
	svc.NewDBWithClient(&svc.Config{}, nil)
	os.Exit(m.Run())
}

func strptr(s string) *string { return &s }

func userWithHash(uid string, ph *string) *cm.GraphUser {
	return &cm.GraphUser{GraphBase: cm.GraphBase{Uid: uid}, PasswordHash: ph}
}

// Regression: a user update that omits the password (nil PasswordHash) must not
// dereference a nil pointer.
func TestPrepareUsersForUpdateNilPasswordNoPanic(t *testing.T) {
	u := userWithHash("0x1", nil)
	if aerr := prepareUsersForUpdate([]*cm.GraphUser{u}); aerr != nil {
		t.Fatalf("unexpected error for nil password: %+v", aerr)
	}
	if u.PasswordHash != nil {
		t.Error("a user with no password should be left unchanged")
	}
}

func TestPrepareUsersForUpdateHashesValidPassword(t *testing.T) {
	u := userWithHash("0x1", strptr("longenough"))
	if aerr := prepareUsersForUpdate([]*cm.GraphUser{u}); aerr != nil {
		t.Fatalf("unexpected error: %+v", aerr)
	}
	if u.PasswordHash == nil || *u.PasswordHash == "longenough" || !strings.Contains(*u.PasswordHash, "$") {
		t.Errorf("password should be hashed to <hash>$<salt>, got %v", u.PasswordHash)
	}
}

func TestPrepareUsersForUpdateShortPassword(t *testing.T) {
	u := userWithHash("0x1", strptr("1234")) // len 4, must be > MIN_USER_PASS_LENGTH (4)
	aerr := prepareUsersForUpdate([]*cm.GraphUser{u})
	if aerr == nil || aerr.StatusCode != 400 {
		t.Errorf("short password should return 400, got %+v", aerr)
	}
}

func TestPrepareUsersForUpdateBadUid(t *testing.T) {
	aerr := prepareUsersForUpdate([]*cm.GraphUser{userWithHash("not-a-uid", nil)})
	if aerr == nil || aerr.StatusCode != 400 {
		t.Errorf("bad uid should return 400, got %+v", aerr)
	}
}
