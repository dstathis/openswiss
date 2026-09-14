//go:build integration

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/dstathis/openswiss/internal/auth"
	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

func TestUsersAPI_GetMe(t *testing.T) {
	database := testDB(t)
	api := &UsersAPI{DB: database}
	user := mustCreateUser(t, database, "me@example.com", "Me")

	req := requestWithUser("GET", "/api/v1/users/me", "", user, nil)
	rec := httptest.NewRecorder()
	api.GetMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got models.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != user.ID || got.Email != user.Email {
		t.Errorf("got %+v, want id=%d email=%s", got, user.ID, user.Email)
	}
}

func TestUsersAPI_CreateAPIKey(t *testing.T) {
	database := testDB(t)
	api := &UsersAPI{DB: database}
	user := mustCreateUser(t, database, "key@example.com", "Key")

	req := requestWithUser("POST", "/api/v1/users/me/keys", `{"name":"My Key"}`, user, nil)
	rec := httptest.NewRecorder()
	api.CreateAPIKey(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["name"] != "My Key" {
		t.Errorf("name = %v, want My Key", resp["name"])
	}
	keyStr, ok := resp["key"].(string)
	if !ok || keyStr == "" {
		t.Fatal("expected non-empty key in response")
	}
	prefix, ok := resp["prefix"].(string)
	if !ok || len(prefix) != 11 {
		t.Errorf("prefix = %v, want 11 chars", resp["prefix"])
	}
	// And validate the key was actually inserted
	keys, err := db.ListAPIKeysByUser(req.Context(), database, user.ID)
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key in DB, got %d", len(keys))
	}
}

func TestUsersAPI_CreateAPIKey_MissingName(t *testing.T) {
	database := testDB(t)
	api := &UsersAPI{DB: database}
	user := mustCreateUser(t, database, "noname@example.com", "NoName")

	req := requestWithUser("POST", "/api/v1/users/me/keys", `{}`, user, nil)
	rec := httptest.NewRecorder()
	api.CreateAPIKey(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestUsersAPI_CreateAPIKey_InvalidJSON(t *testing.T) {
	database := testDB(t)
	api := &UsersAPI{DB: database}
	user := mustCreateUser(t, database, "bad@example.com", "Bad")

	req := requestWithUser("POST", "/api/v1/users/me/keys", `not-json`, user, nil)
	rec := httptest.NewRecorder()
	api.CreateAPIKey(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestUsersAPI_ListAPIKeys(t *testing.T) {
	database := testDB(t)
	api := &UsersAPI{DB: database}
	user := mustCreateUser(t, database, "list@example.com", "List")

	for _, name := range []string{"k1", "k2"} {
		full, prefix, _ := auth.GenerateAPIKey()
		hash, _ := auth.HashAPIKey(full)
		if _, err := db.CreateAPIKey(req(t).Context(), database, user.ID, hash, prefix, name, nil); err != nil {
			t.Fatalf("seed key: %v", err)
		}
	}

	r := requestWithUser("GET", "/api/v1/users/me/keys", "", user, nil)
	rec := httptest.NewRecorder()
	api.ListAPIKeys(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []models.APIKey
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %d", len(got))
	}
}

func TestUsersAPI_DeleteAPIKey(t *testing.T) {
	database := testDB(t)
	api := &UsersAPI{DB: database}
	user := mustCreateUser(t, database, "del@example.com", "Del")

	full, prefix, _ := auth.GenerateAPIKey()
	hash, _ := auth.HashAPIKey(full)
	key, err := db.CreateAPIKey(req(t).Context(), database, user.ID, hash, prefix, "k", nil)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	r := requestWithUser("DELETE", "/api/v1/users/me/keys/"+strconv.FormatInt(key.ID, 10), "", user, map[string]string{"id": strconv.FormatInt(key.ID, 10)})
	rec := httptest.NewRecorder()
	api.DeleteAPIKey(rec, r)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	keys, _ := db.ListAPIKeysByUser(r.Context(), database, user.ID)
	if len(keys) != 0 {
		t.Errorf("expected 0 keys after delete, got %d", len(keys))
	}
}

func TestUsersAPI_DeleteAPIKey_WrongUser(t *testing.T) {
	database := testDB(t)
	api := &UsersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")

	full, prefix, _ := auth.GenerateAPIKey()
	hash, _ := auth.HashAPIKey(full)
	key, err := db.CreateAPIKey(req(t).Context(), database, owner.ID, hash, prefix, "k", nil)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	r := requestWithUser("DELETE", "/api/v1/users/me/keys/"+strconv.FormatInt(key.ID, 10), "", other, map[string]string{"id": strconv.FormatInt(key.ID, 10)})
	rec := httptest.NewRecorder()
	api.DeleteAPIKey(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (key belongs to another user)", rec.Code)
	}
}

// req returns a tiny request object only for getting a context to feed
// db.* calls in tests that don't otherwise have one.
func req(t *testing.T) *http.Request {
	t.Helper()
	return httptest.NewRequest("GET", "/", nil)
}

// apiAccountUser creates a user whose stored hash matches apiTestPassword, so
// the current_password gate on PATCH /users/me can be exercised for real.
const apiTestPassword = "correct-horse"

func apiAccountUser(t *testing.T, database *sql.DB, email, name string) *models.User {
	t.Helper()
	u := mustCreateUser(t, database, email, name)
	hash, err := auth.HashPassword(apiTestPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := db.UpdateUserPassword(context.Background(), database, u.ID, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}
	u.PasswordHash = hash
	return u
}

func TestUsersAPI_UpdateMe_DisplayName(t *testing.T) {
	database := testDB(t)
	api := &UsersAPI{DB: database}
	user := apiAccountUser(t, database, "patch-name@example.com", "PatchBefore")

	rec := httptest.NewRecorder()
	api.UpdateMe(rec, requestWithUser("PATCH", "/api/v1/users/me", `{"display_name":"PatchAfter"}`, user, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got models.User
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DisplayName != "PatchAfter" {
		t.Errorf("response DisplayName = %q, want PatchAfter", got.DisplayName)
	}
	stored, _ := db.GetUserByID(context.Background(), database, user.ID)
	if stored.DisplayName != "PatchAfter" {
		t.Errorf("stored DisplayName = %q, want PatchAfter", stored.DisplayName)
	}
}

func TestUsersAPI_UpdateMe_Password(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	api := &UsersAPI{DB: database}
	user := apiAccountUser(t, database, "patch-pw@example.com", "PatchPw")

	if err := db.CreateSession(ctx, database, "api-other-device", user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	body := `{"current_password":"` + apiTestPassword + `","password":"a-longer-password"}`
	rec := httptest.NewRecorder()
	api.UpdateMe(rec, requestWithUser("PATCH", "/api/v1/users/me", body, user, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	stored, _ := db.GetUserByID(ctx, database, user.ID)
	if !auth.CheckPassword(stored.PasswordHash, "a-longer-password") {
		t.Error("new password does not verify against the stored hash")
	}
	// No session cookie on an API-key style call, so every session goes.
	if _, err := db.GetSession(ctx, database, "api-other-device"); err == nil {
		t.Error("sessions survived a password change made over the API")
	}
}

func TestUsersAPI_UpdateMe_Rejections(t *testing.T) {
	database := testDB(t)
	api := &UsersAPI{DB: database}
	mustCreateUser(t, database, "patch-occupied@example.com", "PatchOccupier")
	user := apiAccountUser(t, database, "patch-reject@example.com", "PatchReject")

	for _, tt := range []struct {
		name string
		body string
		want int
	}{
		{"empty body", `{}`, http.StatusBadRequest},
		{"invalid json", `not json`, http.StatusBadRequest},
		{"password without current", `{"password":"a-longer-password"}`, http.StatusBadRequest},
		{"email without current", `{"email":"x@example.com"}`, http.StatusBadRequest},
		{"wrong current password", `{"current_password":"nope","password":"a-longer-password"}`, http.StatusForbidden},
		{"short password", `{"current_password":"` + apiTestPassword + `","password":"short"}`, http.StatusBadRequest},
		{"invalid email", `{"current_password":"` + apiTestPassword + `","email":"nope"}`, http.StatusBadRequest},
		{"display name taken", `{"display_name":"PatchOccupier"}`, http.StatusConflict},
		{"email taken", `{"current_password":"` + apiTestPassword + `","email":"patch-occupied@example.com"}`, http.StatusConflict},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			api.UpdateMe(rec, requestWithUser("PATCH", "/api/v1/users/me", tt.body, user, nil))
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}

	// Nothing may have moved after all those rejections.
	stored, _ := db.GetUserByID(context.Background(), database, user.ID)
	if stored.DisplayName != "PatchReject" || stored.Email != "patch-reject@example.com" {
		t.Errorf("account changed despite rejections: %+v", stored)
	}
	if !auth.CheckPassword(stored.PasswordHash, apiTestPassword) {
		t.Error("password changed despite rejections")
	}
}

// A validation failure on one field must not let an earlier field land.
func TestUsersAPI_UpdateMe_ValidatesBeforeWriting(t *testing.T) {
	database := testDB(t)
	api := &UsersAPI{DB: database}
	user := apiAccountUser(t, database, "patch-atomic@example.com", "PatchAtomic")

	body := `{"current_password":"` + apiTestPassword + `","display_name":"PatchAtomicNew","email":"not-an-email"}`
	rec := httptest.NewRecorder()
	api.UpdateMe(rec, requestWithUser("PATCH", "/api/v1/users/me", body, user, nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	stored, _ := db.GetUserByID(context.Background(), database, user.ID)
	if stored.DisplayName != "PatchAtomic" {
		t.Errorf("display name %q was committed before the email failed validation", stored.DisplayName)
	}
}
