package auth

import (
"crypto/sha256"
"encoding/hex"
"strings"
"testing"
)

func TestHashPassword(t *testing.T) {
hash, err := HashPassword("testpassword123")
if err != nil {
t.Fatalf("HashPassword returned error: %v", err)
}
if hash == "" {
t.Fatal("HashPassword returned empty hash")
}
if hash == "testpassword123" {
t.Fatal("HashPassword returned plaintext password")
}
}

func TestCheckPassword(t *testing.T) {
password := "mysecretpassword"
hash, err := HashPassword(password)
if err != nil {
t.Fatalf("HashPassword returned error: %v", err)
}
if CheckPassword(hash, password) == false {
t.Error("CheckPassword returned false for correct password")
}
if CheckPassword(hash, "wrongpassword") {
t.Error("CheckPassword returned true for wrong password")
}
if CheckPassword(hash, "") {
t.Error("CheckPassword returned true for empty password")
}
}

func TestGenerateSessionToken(t *testing.T) {
token, err := GenerateSessionToken()
if err != nil {
t.Fatalf("GenerateSessionToken returned error: %v", err)
}
if len(token) != 64 {
t.Errorf("expected token length 64, got %d", len(token))
}
if _, err := hex.DecodeString(token); err != nil {
t.Errorf("token is not valid hex: %v", err)
}
}

func TestGenerateSessionToken_Unique(t *testing.T) {
token1, _ := GenerateSessionToken()
token2, _ := GenerateSessionToken()
if token1 == token2 {
t.Error("two successive tokens should be different")
}
}

func TestGenerateAPIKey(t *testing.T) {
fullKey, prefix, err := GenerateAPIKey()
if err != nil {
t.Fatalf("GenerateAPIKey returned error: %v", err)
}
if strings.HasPrefix(fullKey, "os_") == false {
t.Errorf("API key should start with os_, got %q", fullKey[:3])
}
if len(fullKey) != 67 {
t.Errorf("expected full key length 67, got %d", len(fullKey))
}
if len(prefix) != 11 {
t.Errorf("expected prefix length 11, got %d", len(prefix))
}
if prefix != fullKey[:11] {
t.Errorf("prefix %q should be first 11 chars of key", prefix)
}
}

func TestGenerateAPIKey_Unique(t *testing.T) {
key1, _, _ := GenerateAPIKey()
key2, _, _ := GenerateAPIKey()
if key1 == key2 {
t.Error("two successive API keys should be different")
}
}

func TestHashAndCheckAPIKey(t *testing.T) {
key := "os_testkey123456789012345678901234567890123456789012345678901234"
hash, err := HashAPIKey(key)
if err != nil {
t.Fatalf("HashAPIKey returned error: %v", err)
}
if CheckAPIKey(hash, key) == false {
t.Error("CheckAPIKey returned false for correct key")
}
if CheckAPIKey(hash, "os_wrongkey") {
t.Error("CheckAPIKey returned true for wrong key")
}
}

func TestGenerateResetToken(t *testing.T) {
rawToken, tokenHash, err := GenerateResetToken()
if err != nil {
t.Fatalf("GenerateResetToken returned error: %v", err)
}
if len(rawToken) != 64 {
t.Errorf("expected raw token length 64, got %d", len(rawToken))
}
if len(tokenHash) != 64 {
t.Errorf("expected token hash length 64, got %d", len(tokenHash))
}
expected := sha256.Sum256([]byte(rawToken))
expectedHex := hex.EncodeToString(expected[:])
if tokenHash != expectedHex {
t.Errorf("token hash does not match SHA-256 of raw token")
}
}

func TestGenerateResetToken_Unique(t *testing.T) {
raw1, _, _ := GenerateResetToken()
raw2, _, _ := GenerateResetToken()
if raw1 == raw2 {
t.Error("two successive reset tokens should be different")
}
}

func TestHashResetToken(t *testing.T) {
rawToken := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
hash := HashResetToken(rawToken)
expected := sha256.Sum256([]byte(rawToken))
expectedHex := hex.EncodeToString(expected[:])
if hash != expectedHex {
t.Errorf("HashResetToken mismatch: got %s, want %s", hash, expectedHex)
}
}

func TestHashResetToken_Consistent(t *testing.T) {
token := "sometoken"
h1 := HashResetToken(token)
h2 := HashResetToken(token)
if h1 != h2 {
t.Error("HashResetToken should return same value for same input")
}
}

func TestGenerateResetToken_HashMatchesHashResetToken(t *testing.T) {
rawToken, tokenHash, _ := GenerateResetToken()
recomputed := HashResetToken(rawToken)
if tokenHash != recomputed {
t.Errorf("hash mismatch: %s != %s", tokenHash, recomputed)
}
}
