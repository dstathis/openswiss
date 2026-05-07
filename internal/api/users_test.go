//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

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
