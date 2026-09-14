//go:build integration

package db

import (
"context"
"database/sql"
"errors"
"testing"
"time"

"github.com/dstathis/openswiss/internal/models"
)

func TestCreateAndGetUser(t *testing.T) {
database := testDB(t)
ctx := context.Background()

u, err := CreateUser(ctx, database, "test@example.com", "TestUser", "hash123")
if err != nil {
t.Fatalf("CreateUser: %v", err)
}
if u.ID == 0 {
t.Fatal("expected non-zero user ID")
}
if u.Email != "test@example.com" {
t.Errorf("email = %q, want %q", u.Email, "test@example.com")
}
if u.DisplayName != "TestUser" {
t.Errorf("display_name = %q, want %q", u.DisplayName, "TestUser")
}
if len(u.Roles) != 1 || u.Roles[0] != "player" {
t.Errorf("roles = %v, want [player]", u.Roles)
}

// GetUserByEmail
got, err := GetUserByEmail(ctx, database, "test@example.com")
if err != nil {
t.Fatalf("GetUserByEmail: %v", err)
}
if got.ID != u.ID {
t.Errorf("GetUserByEmail ID = %d, want %d", got.ID, u.ID)
}

// GetUserByID
got2, err := GetUserByID(ctx, database, u.ID)
if err != nil {
t.Fatalf("GetUserByID: %v", err)
}
if got2.Email != "test@example.com" {
t.Errorf("GetUserByID email = %q, want %q", got2.Email, "test@example.com")
}
}

func TestGetUserByEmail_NotFound(t *testing.T) {
database := testDB(t)
ctx := context.Background()

_, err := GetUserByEmail(ctx, database, "nobody@example.com")
if err != sql.ErrNoRows {
t.Errorf("expected sql.ErrNoRows, got %v", err)
}
}

func TestUpdateUserRoles(t *testing.T) {
database := testDB(t)
ctx := context.Background()

u, _ := CreateUser(ctx, database, "roles@example.com", "RolesUser", "hash")
err := UpdateUserRoles(ctx, database, u.ID, []string{"player", "organizer", "admin"})
if err != nil {
t.Fatalf("UpdateUserRoles: %v", err)
}

got, _ := GetUserByID(ctx, database, u.ID)
if len(got.Roles) != 3 {
t.Errorf("roles count = %d, want 3", len(got.Roles))
}
}

func TestListUsers(t *testing.T) {
database := testDB(t)
ctx := context.Background()

CreateUser(ctx, database, "a@example.com", "UserA", "hash")
CreateUser(ctx, database, "b@example.com", "UserB", "hash")
CreateUser(ctx, database, "c@example.com", "UserC", "hash")

users, err := ListUsers(ctx, database, 1, 2)
if err != nil {
t.Fatalf("ListUsers: %v", err)
}
if len(users) != 2 {
t.Errorf("page 1 len = %d, want 2", len(users))
}

users2, err := ListUsers(ctx, database, 2, 2)
if err != nil {
t.Fatalf("ListUsers page 2: %v", err)
}
if len(users2) != 1 {
t.Errorf("page 2 len = %d, want 1", len(users2))
}
}

func TestUpdateUserPassword(t *testing.T) {
database := testDB(t)
ctx := context.Background()

u, _ := CreateUser(ctx, database, "pw@example.com", "PWUser", "oldhash")
err := UpdateUserPassword(ctx, database, u.ID, "newhash")
if err != nil {
t.Fatalf("UpdateUserPassword: %v", err)
}
got, _ := GetUserByID(ctx, database, u.ID)
if got.PasswordHash != "newhash" {
t.Errorf("password_hash = %q, want %q", got.PasswordHash, "newhash")
}
}

func TestSessions(t *testing.T) {
database := testDB(t)
ctx := context.Background()

u, _ := CreateUser(ctx, database, "sess@example.com", "SessUser", "hash")

expires := time.Now().Add(time.Hour)
err := CreateSession(ctx, database, "sess-token-123", u.ID, expires)
if err != nil {
t.Fatalf("CreateSession: %v", err)
}

s, err := GetSession(ctx, database, "sess-token-123")
if err != nil {
t.Fatalf("GetSession: %v", err)
}
if s.UserID != u.ID {
t.Errorf("session user_id = %d, want %d", s.UserID, u.ID)
}

// Delete session
err = DeleteSession(ctx, database, "sess-token-123")
if err != nil {
t.Fatalf("DeleteSession: %v", err)
}
_, err = GetSession(ctx, database, "sess-token-123")
if err != sql.ErrNoRows {
t.Errorf("expected sql.ErrNoRows after delete, got %v", err)
}
}

func TestExpiredSession(t *testing.T) {
database := testDB(t)
ctx := context.Background()

u, _ := CreateUser(ctx, database, "exp@example.com", "ExpUser", "hash")
expires := time.Now().Add(-time.Hour) // already expired
CreateSession(ctx, database, "expired-sess", u.ID, expires)

_, err := GetSession(ctx, database, "expired-sess")
if err != sql.ErrNoRows {
t.Errorf("expected sql.ErrNoRows for expired session, got %v", err)
}

// DeleteExpiredSessions should clean it up
err = DeleteExpiredSessions(ctx, database)
if err != nil {
t.Fatalf("DeleteExpiredSessions: %v", err)
}
}

func TestAPIKeys(t *testing.T) {
database := testDB(t)
ctx := context.Background()

u, _ := CreateUser(ctx, database, "api@example.com", "APIUser", "hash")

k, err := CreateAPIKey(ctx, database, u.ID, "keyhash123", "os_abc", "My Key", nil)
if err != nil {
t.Fatalf("CreateAPIKey: %v", err)
}
if k.ID == 0 {
t.Fatal("expected non-zero key ID")
}
if k.Prefix != "os_abc" {
t.Errorf("prefix = %q, want %q", k.Prefix, "os_abc")
}

// ListAPIKeysByUser
keys, err := ListAPIKeysByUser(ctx, database, u.ID)
if err != nil {
t.Fatalf("ListAPIKeysByUser: %v", err)
}
if len(keys) != 1 {
t.Errorf("keys count = %d, want 1", len(keys))
}

// GetAPIKeysByPrefix
keys2, err := GetAPIKeysByPrefix(ctx, database, "os_abc")
if err != nil {
t.Fatalf("GetAPIKeysByPrefix: %v", err)
}
if len(keys2) != 1 {
t.Errorf("keys by prefix count = %d, want 1", len(keys2))
}

// UpdateAPIKeyLastUsed
err = UpdateAPIKeyLastUsed(ctx, database, k.ID)
if err != nil {
t.Fatalf("UpdateAPIKeyLastUsed: %v", err)
}

// DeleteAPIKey
err = DeleteAPIKey(ctx, database, k.ID, u.ID)
if err != nil {
t.Fatalf("DeleteAPIKey: %v", err)
}
keys3, _ := ListAPIKeysByUser(ctx, database, u.ID)
if len(keys3) != 0 {
t.Errorf("keys after delete = %d, want 0", len(keys3))
}
}

func TestDeleteAPIKey_WrongUser(t *testing.T) {
database := testDB(t)
ctx := context.Background()

u1, _ := CreateUser(ctx, database, "u1@example.com", "User1", "hash")
u2, _ := CreateUser(ctx, database, "u2@example.com", "User2", "hash")

k, _ := CreateAPIKey(ctx, database, u1.ID, "keyhash", "os_xyz", "Key", nil)

err := DeleteAPIKey(ctx, database, k.ID, u2.ID)
if err != sql.ErrNoRows {
t.Errorf("expected sql.ErrNoRows when wrong user deletes key, got %v", err)
}
}

func TestPasswordResets(t *testing.T) {
database := testDB(t)
ctx := context.Background()

u, _ := CreateUser(ctx, database, "reset@example.com", "ResetUser", "hash")

expires := time.Now().Add(time.Hour)
err := CreatePasswordReset(ctx, database, u.ID, "tokenhash123", expires)
if err != nil {
t.Fatalf("CreatePasswordReset: %v", err)
}

r, err := GetPasswordResetByTokenHash(ctx, database, "tokenhash123")
if err != nil {
t.Fatalf("GetPasswordResetByTokenHash: %v", err)
}
if r.UserID != u.ID {
t.Errorf("reset user_id = %d, want %d", r.UserID, u.ID)
}

// Delete
err = DeletePasswordReset(ctx, database, r.ID)
if err != nil {
t.Fatalf("DeletePasswordReset: %v", err)
}
_, err = GetPasswordResetByTokenHash(ctx, database, "tokenhash123")
if err != sql.ErrNoRows {
t.Errorf("expected sql.ErrNoRows after delete, got %v", err)
}
}

func TestPasswordReset_ReplacesExisting(t *testing.T) {
database := testDB(t)
ctx := context.Background()

u, _ := CreateUser(ctx, database, "replace@example.com", "ReplaceUser", "hash")
expires := time.Now().Add(time.Hour)

CreatePasswordReset(ctx, database, u.ID, "token1", expires)
CreatePasswordReset(ctx, database, u.ID, "token2", expires)

// First token should be gone
_, err := GetPasswordResetByTokenHash(ctx, database, "token1")
if err != sql.ErrNoRows {
t.Errorf("expected old token deleted, got %v", err)
}

// Second token should exist
r, err := GetPasswordResetByTokenHash(ctx, database, "token2")
if err != nil {
t.Fatalf("expected new token to exist: %v", err)
}
if r.UserID != u.ID {
t.Errorf("reset user_id = %d, want %d", r.UserID, u.ID)
}
}

func TestUpdateUserDisplayName(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	u, _ := CreateUser(ctx, database, "rename@example.com", "OldName", "hash")
	if err := UpdateUserDisplayName(ctx, database, u.ID, "NewName"); err != nil {
		t.Fatalf("UpdateUserDisplayName: %v", err)
	}

	got, _ := GetUserByID(ctx, database, u.ID)
	if got.DisplayName != "NewName" {
		t.Errorf("display_name = %q, want %q", got.DisplayName, "NewName")
	}
}

func TestUpdateUserDisplayNameTaken(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	if _, err := CreateUser(ctx, database, "holder@example.com", "Holder", "hash"); err != nil {
		t.Fatalf("create holder: %v", err)
	}
	u, _ := CreateUser(ctx, database, "renamer@example.com", "Renamer", "hash")

	err := UpdateUserDisplayName(ctx, database, u.ID, "Holder")
	if !errors.Is(err, ErrDisplayNameTaken) {
		t.Fatalf("UpdateUserDisplayName = %v, want ErrDisplayNameTaken", err)
	}
	got, _ := GetUserByID(ctx, database, u.ID)
	if got.DisplayName != "Renamer" {
		t.Errorf("display_name = %q, want the rename to have rolled back", got.DisplayName)
	}
}

// A rename follows the player into tournaments that haven't started, and stays
// out of ones that have — those names are baked into the engine state and are
// the historical record for their pairings.
func TestUpdateUserDisplayNamePropagates(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	u, _ := CreateUser(ctx, database, "propagate@example.com", "BeforeName", "hash")

	open := &models.Tournament{
		Name: "Open", MaxPlayers: 8, PointsWin: 3, PointsDraw: 1,
		Status: models.TournamentStatusRegistrationOpen, OrganizerID: u.ID,
	}
	if err := CreateTournament(ctx, database, open); err != nil {
		t.Fatalf("create open tournament: %v", err)
	}
	started := &models.Tournament{
		Name: "Started", MaxPlayers: 8, PointsWin: 3, PointsDraw: 1,
		Status: models.TournamentStatusRegistrationOpen, OrganizerID: u.ID,
	}
	if err := CreateTournament(ctx, database, started); err != nil {
		t.Fatalf("create started tournament: %v", err)
	}

	openReg, err := CreateRegistration(ctx, database, open.ID, u.ID, u.DisplayName)
	if err != nil {
		t.Fatalf("register in open tournament: %v", err)
	}
	startedReg, err := CreateRegistration(ctx, database, started.ID, u.ID, u.DisplayName)
	if err != nil {
		t.Fatalf("register in started tournament: %v", err)
	}
	if err := UpdateTournamentStatus(ctx, database, started.ID, models.TournamentStatusInProgress); err != nil {
		t.Fatalf("start tournament: %v", err)
	}

	if err := UpdateUserDisplayName(ctx, database, u.ID, "AfterName"); err != nil {
		t.Fatalf("UpdateUserDisplayName: %v", err)
	}

	gotOpen, _ := GetRegistrationByID(ctx, database, openReg.ID)
	if gotOpen.DisplayName != "AfterName" {
		t.Errorf("not-yet-started registration = %q, want AfterName", gotOpen.DisplayName)
	}
	gotStarted, _ := GetRegistrationByID(ctx, database, startedReg.ID)
	if gotStarted.DisplayName != "BeforeName" {
		t.Errorf("in-progress registration = %q, want the historical BeforeName", gotStarted.DisplayName)
	}
}

// The per-tournament unique index means a guest sitting on the new name has to
// move aside, exactly as it does when a real user registers.
func TestUpdateUserDisplayNameBumpsGuest(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	u, _ := CreateUser(ctx, database, "bumper@example.com", "Bumper", "hash")
	tourn := &models.Tournament{
		Name: "Bump", MaxPlayers: 8, PointsWin: 3, PointsDraw: 1,
		Status: models.TournamentStatusRegistrationOpen, OrganizerID: u.ID,
	}
	if err := CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}
	reg, err := CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName)
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	guest, err := CreateGuestRegistration(ctx, database, tourn.ID, "Wanted")
	if err != nil {
		t.Fatalf("create guest: %v", err)
	}

	if err := UpdateUserDisplayName(ctx, database, u.ID, "Wanted"); err != nil {
		t.Fatalf("UpdateUserDisplayName: %v", err)
	}

	gotUser, _ := GetRegistrationByID(ctx, database, reg.ID)
	if gotUser.DisplayName != "Wanted" {
		t.Errorf("user registration = %q, want Wanted", gotUser.DisplayName)
	}
	gotGuest, _ := GetRegistrationByID(ctx, database, guest.ID)
	if gotGuest.DisplayName != "Wanted (2)" {
		t.Errorf("guest registration = %q, want %q", gotGuest.DisplayName, "Wanted (2)")
	}
}

// Changing only the case of a name must not make the user collide with their
// own registration row and bump themselves to "name (2)".
func TestUpdateUserDisplayNameCaseOnlyChange(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	u, _ := CreateUser(ctx, database, "case@example.com", "lowercase", "hash")
	tourn := &models.Tournament{
		Name: "Case", MaxPlayers: 8, PointsWin: 3, PointsDraw: 1,
		Status: models.TournamentStatusRegistrationOpen, OrganizerID: u.ID,
	}
	if err := CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}
	reg, err := CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName)
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	if err := UpdateUserDisplayName(ctx, database, u.ID, "LowerCase"); err != nil {
		t.Fatalf("UpdateUserDisplayName: %v", err)
	}

	got, _ := GetRegistrationByID(ctx, database, reg.ID)
	if got.DisplayName != "LowerCase" {
		t.Errorf("registration = %q, want LowerCase", got.DisplayName)
	}
}

func TestUpdateUserEmail(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	u, _ := CreateUser(ctx, database, "before@example.com", "EmailUser", "hash")

	if err := UpdateUserEmail(ctx, database, u.ID, "after@example.com", true); err != nil {
		t.Fatalf("UpdateUserEmail: %v", err)
	}
	got, _ := GetUserByID(ctx, database, u.ID)
	if got.Email != "after@example.com" {
		t.Errorf("email = %q, want after@example.com", got.Email)
	}
	if !got.EmailVerified() {
		t.Error("verified=true should mark the address confirmed")
	}

	if err := UpdateUserEmail(ctx, database, u.ID, "unconfirmed@example.com", false); err != nil {
		t.Fatalf("UpdateUserEmail unverified: %v", err)
	}
	got, _ = GetUserByID(ctx, database, u.ID)
	if got.EmailVerified() {
		t.Error("verified=false should clear the confirmed status")
	}
}

func TestUpdateUserEmailTaken(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	if _, err := CreateUser(ctx, database, "occupied@example.com", "Occupier", "hash"); err != nil {
		t.Fatalf("create occupier: %v", err)
	}
	u, _ := CreateUser(ctx, database, "mover@example.com", "Mover", "hash")

	err := UpdateUserEmail(ctx, database, u.ID, "occupied@example.com", true)
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("UpdateUserEmail = %v, want ErrEmailTaken", err)
	}
}

func TestDeleteUserSessionsExcept(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	u, _ := CreateUser(ctx, database, "sessions@example.com", "SessionUser", "hash")
	other, _ := CreateUser(ctx, database, "bystander@example.com", "Bystander", "hash")
	expires := time.Now().Add(time.Hour)

	for _, id := range []string{"keep-me", "drop-me-1", "drop-me-2"} {
		if err := CreateSession(ctx, database, id, u.ID, expires); err != nil {
			t.Fatalf("create session %s: %v", id, err)
		}
	}
	if err := CreateSession(ctx, database, "someone-else", other.ID, expires); err != nil {
		t.Fatalf("create bystander session: %v", err)
	}

	if err := DeleteUserSessionsExcept(ctx, database, u.ID, "keep-me"); err != nil {
		t.Fatalf("DeleteUserSessionsExcept: %v", err)
	}

	if _, err := GetSession(ctx, database, "keep-me"); err != nil {
		t.Errorf("kept session was deleted: %v", err)
	}
	for _, id := range []string{"drop-me-1", "drop-me-2"} {
		if _, err := GetSession(ctx, database, id); err == nil {
			t.Errorf("session %s survived", id)
		}
	}
	if _, err := GetSession(ctx, database, "someone-else"); err != nil {
		t.Errorf("another user's session was deleted: %v", err)
	}
}

// An empty exceptID is the API-key case: no session to keep, so all go.
func TestDeleteUserSessionsExceptAll(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	u, _ := CreateUser(ctx, database, "allsessions@example.com", "AllSessions", "hash")
	if err := CreateSession(ctx, database, "only-one", u.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := DeleteUserSessionsExcept(ctx, database, u.ID, ""); err != nil {
		t.Fatalf("DeleteUserSessionsExcept: %v", err)
	}
	if _, err := GetSession(ctx, database, "only-one"); err == nil {
		t.Error("session survived a full revocation")
	}
}
