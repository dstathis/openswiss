//go:build integration

package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dstathis/openswiss/internal/auth"
	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

const testPassword = "correct-horse"

// accountUser creates a user whose stored hash matches testPassword, so the
// current-password gate on the account routes can be exercised for real.
func accountUser(t *testing.T, database *sql.DB, email, name string) *models.User {
	t.Helper()
	u := mustCreateUser(t, database, email, name)
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := db.UpdateUserPassword(context.Background(), database, u.ID, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}
	u.PasswordHash = hash
	return u
}

func renderedData(t *testing.T, tmpl *mockTemplate) map[string]interface{} {
	t.Helper()
	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	if tmpl.calls[0].Name != "account.html" {
		t.Fatalf("rendered %q, want account.html", tmpl.calls[0].Name)
	}
	data, ok := tmpl.calls[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("template data is %T, want map", tmpl.calls[0].Data)
	}
	return data
}

func wantError(t *testing.T, tmpl *mockTemplate, want string) {
	t.Helper()
	data := renderedData(t, tmpl)
	if data["Error"] != want {
		t.Errorf("Error = %v, want %q", data["Error"], want)
	}
}

func wantSuccess(t *testing.T, tmpl *mockTemplate) map[string]interface{} {
	t.Helper()
	data := renderedData(t, tmpl)
	if data["Error"] != nil {
		t.Fatalf("unexpected Error = %v", data["Error"])
	}
	if data["Success"] == nil {
		t.Fatalf("expected a Success message, got none")
	}
	return data
}

func TestAccountHandler_Page(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &AccountHandler{DB: database, Tmpl: tmpl}
	user := accountUser(t, database, "page@example.com", "PageUser")

	h.Page(httptest.NewRecorder(), requestWithUser("GET", "/account", "", user, nil))

	data := renderedData(t, tmpl)
	if data["User"].(*models.User).DisplayName != "PageUser" {
		t.Errorf("User = %+v", data["User"])
	}
}

func TestAccountHandler_UpdateDisplayName(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &AccountHandler{DB: database, Tmpl: tmpl}
	user := accountUser(t, database, "rename@example.com", "OldName")

	req := requestWithUser("POST", "/account/display-name", "display_name=NewName", user, nil)
	h.UpdateDisplayName(httptest.NewRecorder(), req)

	data := wantSuccess(t, tmpl)
	// The page must show the new name immediately — the context user is stale
	// the moment the rename commits.
	if got := data["User"].(*models.User).DisplayName; got != "NewName" {
		t.Errorf("rendered DisplayName = %q, want NewName", got)
	}
	stored, _ := db.GetUserByID(context.Background(), database, user.ID)
	if stored.DisplayName != "NewName" {
		t.Errorf("stored DisplayName = %q, want NewName", stored.DisplayName)
	}
}

func TestAccountHandler_UpdateDisplayName_Taken(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &AccountHandler{DB: database, Tmpl: tmpl}
	mustCreateUser(t, database, "holder@example.com", "Taken")
	user := accountUser(t, database, "renamer@example.com", "Renamer")

	req := requestWithUser("POST", "/account/display-name", "display_name=Taken", user, nil)
	h.UpdateDisplayName(httptest.NewRecorder(), req)

	wantError(t, tmpl, "That display name is already taken.")
}

func TestAccountHandler_UpdateDisplayName_Invalid(t *testing.T) {
	database := testDB(t)
	user := accountUser(t, database, "invalid-name@example.com", "InvalidName")

	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{"empty", "display_name=", "Display name is required."},
		{"whitespace only", "display_name=+++", "Display name is required."},
		{"too long", "display_name=" + strings.Repeat("a", 101), "Display name must be 100 characters or fewer."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := &mockTemplate{}
			h := &AccountHandler{DB: database, Tmpl: tmpl}
			h.UpdateDisplayName(httptest.NewRecorder(), requestWithUser("POST", "/account/display-name", tt.body, user, nil))
			wantError(t, tmpl, tt.want)
		})
	}
}

func TestAccountHandler_ChangePassword(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tmpl := &mockTemplate{}
	h := &AccountHandler{DB: database, Tmpl: tmpl}
	user := accountUser(t, database, "pwchange@example.com", "PwChange")

	// Two live sessions: the one making the request, and another device.
	expires := time.Now().Add(time.Hour)
	if err := db.CreateSession(ctx, database, "this-session", user.ID, expires); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := db.CreateSession(ctx, database, "other-device", user.ID, expires); err != nil {
		t.Fatalf("create other session: %v", err)
	}

	body := "current_password=" + testPassword + "&password=brand-new-pw&confirm_password=brand-new-pw"
	req := requestWithUser("POST", "/account/password", body, user, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "this-session"})
	h.ChangePassword(httptest.NewRecorder(), req)

	wantSuccess(t, tmpl)

	stored, _ := db.GetUserByID(ctx, database, user.ID)
	if !auth.CheckPassword(stored.PasswordHash, "brand-new-pw") {
		t.Error("new password does not verify against the stored hash")
	}
	if _, err := db.GetSession(ctx, database, "this-session"); err != nil {
		t.Errorf("current session was revoked: %v", err)
	}
	if _, err := db.GetSession(ctx, database, "other-device"); err == nil {
		t.Error("other device's session survived the password change")
	}
}

func TestAccountHandler_ChangePassword_Rejections(t *testing.T) {
	database := testDB(t)
	user := accountUser(t, database, "pwreject@example.com", "PwReject")

	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{"wrong current", "current_password=nope&password=long-enough&confirm_password=long-enough", "Current password is incorrect."},
		{"empty current", "password=long-enough&confirm_password=long-enough", "Current password is incorrect."},
		{"too short", "current_password=" + testPassword + "&password=short&confirm_password=short", "Password must be at least 8 characters."},
		{"mismatch", "current_password=" + testPassword + "&password=long-enough&confirm_password=long-enough-not", "Passwords do not match."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := &mockTemplate{}
			h := &AccountHandler{DB: database, Tmpl: tmpl}
			h.ChangePassword(httptest.NewRecorder(), requestWithUser("POST", "/account/password", tt.body, user, nil))
			wantError(t, tmpl, tt.want)

			// Nothing may have changed on a rejected attempt.
			stored, _ := db.GetUserByID(context.Background(), database, user.ID)
			if !auth.CheckPassword(stored.PasswordHash, testPassword) {
				t.Error("password was changed despite the rejection")
			}
		})
	}
}

// With SMTP unconfigured there is no confirmation link to send, so the change
// lands immediately.
func TestAccountHandler_ChangeEmail_NoSMTP(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &AccountHandler{DB: database, Tmpl: tmpl}
	user := accountUser(t, database, "old-addr@example.com", "EmailNoSMTP")

	body := "current_password=" + testPassword + "&email=new-addr@example.com"
	h.ChangeEmail(httptest.NewRecorder(), requestWithUser("POST", "/account/email", body, user, nil))

	wantSuccess(t, tmpl)
	stored, _ := db.GetUserByID(context.Background(), database, user.ID)
	if stored.Email != "new-addr@example.com" {
		t.Errorf("stored email = %q, want new-addr@example.com", stored.Email)
	}
}

func TestAccountHandler_ChangeEmail_Rejections(t *testing.T) {
	database := testDB(t)
	mustCreateUser(t, database, "occupied@example.com", "Occupier")
	user := accountUser(t, database, "emailreject@example.com", "EmailReject")

	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{"wrong current", "current_password=nope&email=fine@example.com", "Current password is incorrect."},
		{"invalid address", "current_password=" + testPassword + "&email=not-an-email", "Please enter a valid email address."},
		{"already in use", "current_password=" + testPassword + "&email=occupied@example.com", "That email address is already in use."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := &mockTemplate{}
			h := &AccountHandler{DB: database, Tmpl: tmpl}
			h.ChangeEmail(httptest.NewRecorder(), requestWithUser("POST", "/account/email", tt.body, user, nil))
			wantError(t, tmpl, tt.want)

			stored, _ := db.GetUserByID(context.Background(), database, user.ID)
			if stored.Email != "emailreject@example.com" {
				t.Errorf("email changed to %q despite the rejection", stored.Email)
			}
		})
	}
}

func TestAccountHandler_APIKeys(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tmpl := &mockTemplate{}
	h := &AccountHandler{DB: database, Tmpl: tmpl}
	user := accountUser(t, database, "keys@example.com", "KeyUser")

	h.CreateAPIKey(httptest.NewRecorder(), requestWithUser("POST", "/account/api-keys", "name=laptop", user, nil))
	data := renderedData(t, tmpl)
	fullKey, _ := data["NewAPIKey"].(string)
	if fullKey == "" {
		t.Fatal("expected the full key to be revealed once")
	}

	keys, err := db.ListAPIKeysByUser(ctx, database, user.ID)
	if err != nil || len(keys) != 1 {
		t.Fatalf("ListAPIKeysByUser = %v, %v; want 1 key", keys, err)
	}
	if keys[0].Name != "laptop" {
		t.Errorf("key name = %q, want laptop", keys[0].Name)
	}

	tmpl.calls = nil
	h.DeleteAPIKey(httptest.NewRecorder(), requestWithUser("POST", "/account/api-keys/x/delete", "", user,
		map[string]string{"id": strconv.FormatInt(keys[0].ID, 10)}))
	wantSuccess(t, tmpl)

	remaining, _ := db.ListAPIKeysByUser(ctx, database, user.ID)
	if len(remaining) != 0 {
		t.Errorf("key survived revocation: %+v", remaining)
	}
}

func TestAccountHandler_CreateAPIKey_NameRequired(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &AccountHandler{DB: database, Tmpl: tmpl}
	user := accountUser(t, database, "noname@example.com", "NoName")

	h.CreateAPIKey(httptest.NewRecorder(), requestWithUser("POST", "/account/api-keys", "name=", user, nil))
	wantError(t, tmpl, "API key name is required.")
}

// Another user's key id must be a no-op, not a deletion.
func TestAccountHandler_DeleteAPIKey_OtherUsersKey(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tmpl := &mockTemplate{}
	h := &AccountHandler{DB: database, Tmpl: tmpl}
	owner := mustCreateUser(t, database, "keyowner@example.com", "KeyOwner")
	attacker := accountUser(t, database, "attacker@example.com", "Attacker")

	key, err := db.CreateAPIKey(ctx, database, owner.ID, "hash", "prefix01", "owner key", nil)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	h.DeleteAPIKey(httptest.NewRecorder(), requestWithUser("POST", "/", "", attacker,
		map[string]string{"id": strconv.FormatInt(key.ID, 10)}))
	wantError(t, tmpl, "API key not found.")

	keys, _ := db.ListAPIKeysByUser(ctx, database, owner.ID)
	if len(keys) != 1 {
		t.Error("another user's key was deleted")
	}
}

// The confirmation link is what actually moves the address: until it's
// clicked, the account keeps the old one.
func TestVerifyEmail_AppliesPendingChange(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tmpl := &mockTemplate{}
	h := &AuthHandler{DB: database, Tmpl: tmpl}
	user := accountUser(t, database, "pending-old@example.com", "PendingChange")

	rawToken, tokenHash, err := auth.GenerateResetToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	newEmail := "pending-new@example.com"
	if err := db.CreateEmailVerification(ctx, database, user.ID, tokenHash, time.Now().Add(time.Hour), &newEmail); err != nil {
		t.Fatalf("create verification: %v", err)
	}

	stillOld, _ := db.GetUserByID(ctx, database, user.ID)
	if stillOld.Email != "pending-old@example.com" {
		t.Fatalf("email changed before the link was clicked: %q", stillOld.Email)
	}

	h.VerifyEmail(httptest.NewRecorder(), httptest.NewRequest("GET", "/verify-email?token="+rawToken, nil))

	if len(tmpl.calls) != 1 || tmpl.calls[0].Name != "verify_email.html" {
		t.Fatalf("rendered %+v", tmpl.calls)
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] != nil {
		t.Fatalf("unexpected Error = %v", data["Error"])
	}

	got, _ := db.GetUserByID(ctx, database, user.ID)
	if got.Email != newEmail {
		t.Errorf("email = %q, want %q", got.Email, newEmail)
	}
	if !got.EmailVerified() {
		t.Error("the new address should be confirmed by the link that delivered it")
	}
}

// Someone else can claim the address between the request and the click.
func TestVerifyEmail_PendingChangeToTakenAddress(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tmpl := &mockTemplate{}
	h := &AuthHandler{DB: database, Tmpl: tmpl}
	user := accountUser(t, database, "racer@example.com", "Racer")

	rawToken, tokenHash, err := auth.GenerateResetToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	contested := "contested@example.com"
	if err := db.CreateEmailVerification(ctx, database, user.ID, tokenHash, time.Now().Add(time.Hour), &contested); err != nil {
		t.Fatalf("create verification: %v", err)
	}
	mustCreateUser(t, database, contested, "Claimer")

	h.VerifyEmail(httptest.NewRecorder(), httptest.NewRequest("GET", "/verify-email?token="+rawToken, nil))

	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] == nil {
		t.Fatal("expected an error when the address was claimed first")
	}
	got, _ := db.GetUserByID(ctx, database, user.ID)
	if got.Email != "racer@example.com" {
		t.Errorf("email = %q, want the original to be kept", got.Email)
	}
}
