//go:build integration

package db

import (
"context"
"database/sql"
"encoding/json"
"fmt"
"reflect"
"testing"
"time"

"github.com/dstathis/openswiss/internal/models"
)

func createTestOrganizer(t *testing.T, database *sql.DB) *models.User {
t.Helper()
ctx := context.Background()
email := fmt.Sprintf("org-%s@example.com", t.Name())
u, err := CreateUser(ctx, database, email, "Org-"+t.Name(), "hash")
if err != nil {
t.Fatalf("create organizer: %v", err)
}
return u
}

func TestCreateAndGetTournament(t *testing.T) {
database := testDB(t)
ctx := context.Background()
org := createTestOrganizer(t, database)

desc := "A test tournament"
tourn := &models.Tournament{
Name:        "Test Tournament",
Description: &desc,
MaxPlayers:  32,
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
TopCut:      8,
Status:      models.TournamentStatusScheduled,
OrganizerID: org.ID,
}
err := CreateTournament(ctx, database, tourn)
if err != nil {
t.Fatalf("CreateTournament: %v", err)
}
if tourn.ID == 0 {
t.Fatal("expected non-zero tournament ID")
}

got, err := GetTournament(ctx, database, tourn.ID)
if err != nil {
t.Fatalf("GetTournament: %v", err)
}
if got.Name != "Test Tournament" {
t.Errorf("name = %q, want %q", got.Name, "Test Tournament")
}
if got.MaxPlayers != 32 {
t.Errorf("max_players = %d, want 32", got.MaxPlayers)
}
if got.TopCut != 8 {
t.Errorf("top_cut = %d, want 8", got.TopCut)
}
}

func TestUpdateTournament(t *testing.T) {
database := testDB(t)
ctx := context.Background()
org := createTestOrganizer(t, database)

tourn := &models.Tournament{
Name:        "Original",
MaxPlayers:  16,
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusScheduled,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

tourn.Name = "Updated"
tourn.MaxPlayers = 64
err := UpdateTournament(ctx, database, tourn)
if err != nil {
t.Fatalf("UpdateTournament: %v", err)
}

got, _ := GetTournament(ctx, database, tourn.ID)
if got.Name != "Updated" {
t.Errorf("name = %q, want %q", got.Name, "Updated")
}
if got.MaxPlayers != 64 {
t.Errorf("max_players = %d, want 64", got.MaxPlayers)
}
}

func TestUpdateTournamentStatus(t *testing.T) {
database := testDB(t)
ctx := context.Background()
org := createTestOrganizer(t, database)

tourn := &models.Tournament{
Name:        "Status Test",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusScheduled,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

err := UpdateTournamentStatus(ctx, database, tourn.ID, models.TournamentStatusRegistrationOpen)
if err != nil {
t.Fatalf("UpdateTournamentStatus: %v", err)
}

got, _ := GetTournament(ctx, database, tourn.ID)
if got.Status != models.TournamentStatusRegistrationOpen {
t.Errorf("status = %q, want %q", got.Status, models.TournamentStatusRegistrationOpen)
}
}

func TestDeleteTournament(t *testing.T) {
database := testDB(t)
ctx := context.Background()
org := createTestOrganizer(t, database)

tourn := &models.Tournament{
Name:        "Delete Me",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusScheduled,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

err := DeleteTournament(ctx, database, tourn.ID)
if err != nil {
t.Fatalf("DeleteTournament: %v", err)
}
_, err = GetTournament(ctx, database, tourn.ID)
if err != sql.ErrNoRows {
t.Errorf("expected sql.ErrNoRows, got %v", err)
}
}

func TestListTournaments(t *testing.T) {
database := testDB(t)
ctx := context.Background()
org := createTestOrganizer(t, database)

for i := 0; i < 3; i++ {
tourn := &models.Tournament{
Name:        "Tournament",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusScheduled,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)
}

all, err := ListTournaments(ctx, database, "", 1, 10)
if err != nil {
t.Fatalf("ListTournaments: %v", err)
}
if len(all) != 3 {
t.Errorf("all count = %d, want 3", len(all))
}

filtered, err := ListTournaments(ctx, database, models.TournamentStatusScheduled, 1, 10)
if err != nil {
t.Fatalf("ListTournaments filtered: %v", err)
}
if len(filtered) != 3 {
t.Errorf("filtered count = %d, want 3", len(filtered))
}

page1, _ := ListTournaments(ctx, database, "", 1, 2)
if len(page1) != 2 {
t.Errorf("page1 count = %d, want 2", len(page1))
}
}

func TestListUpcomingTournaments(t *testing.T) {
database := testDB(t)
ctx := context.Background()
org := createTestOrganizer(t, database)

future := time.Now().Add(24 * time.Hour)
tourn := &models.Tournament{
Name:        "Upcoming",
ScheduledAt: &future,
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

finished := &models.Tournament{
Name:        "Done",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusFinished,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, finished)

upcoming, err := ListUpcomingTournaments(ctx, database, 10)
if err != nil {
t.Fatalf("ListUpcomingTournaments: %v", err)
}
if len(upcoming) != 1 {
t.Errorf("upcoming count = %d, want 1", len(upcoming))
}
}

func TestRegistrations(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-reg@example.com", "OrgReg", "hash")
player, _ := CreateUser(ctx, database, "player-reg@example.com", "PlayerReg", "hash")

tourn := &models.Tournament{
Name:        "Reg Test",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

reg, err := CreateRegistration(ctx, database, tourn.ID, player.ID, player.DisplayName)
if err != nil {
t.Fatalf("CreateRegistration: %v", err)
}
if reg.Status != "confirmed" {
t.Errorf("status = %q, want confirmed", reg.Status)
}

got, err := GetRegistration(ctx, database, tourn.ID, player.ID)
if err != nil {
t.Fatalf("GetRegistration: %v", err)
}
if got.DisplayName != "PlayerReg" {
t.Errorf("display_name = %q, want %q", got.DisplayName, "PlayerReg")
}

regs, err := ListRegistrations(ctx, database, tourn.ID)
if err != nil {
t.Fatalf("ListRegistrations: %v", err)
}
if len(regs) != 1 {
t.Errorf("registrations count = %d, want 1", len(regs))
}

count, err := CountRegistrations(ctx, database, tourn.ID)
if err != nil {
t.Fatalf("CountRegistrations: %v", err)
}
if count != 1 {
t.Errorf("count = %d, want 1", count)
}

decklist := []byte("{\"cards\": [\"Lightning Bolt\"]}")
err = UpdateRegistrationDecklist(ctx, database, tourn.ID, player.ID, decklist)
if err != nil {
t.Fatalf("UpdateRegistrationDecklist: %v", err)
}

err = UpdateRegistrationStatus(ctx, database, tourn.ID, player.ID, models.RegistrationStatusDropped)
if err != nil {
t.Fatalf("UpdateRegistrationStatus: %v", err)
}
got2, _ := GetRegistration(ctx, database, tourn.ID, player.ID)
if got2.Status != models.RegistrationStatusDropped {
t.Errorf("status = %q, want %q", got2.Status, models.RegistrationStatusDropped)
}

count2, _ := CountRegistrations(ctx, database, tourn.ID)
if count2 != 0 {
t.Errorf("count after drop = %d, want 0", count2)
}
}

func TestPendingRegistration(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-pending@example.com", "OrgPending", "hash")
player, _ := CreateUser(ctx, database, "player-pending@example.com", "PlayerPending", "hash")

tourn := &models.Tournament{
Name:        "Pending Test",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

reg, err := CreatePendingRegistration(ctx, database, tourn.ID, player.ID, player.DisplayName)
if err != nil {
t.Fatalf("CreatePendingRegistration: %v", err)
}
if reg.Status != "pending" {
t.Errorf("status = %q, want pending", reg.Status)
}
}

func TestDeleteRegistration(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-delreg@example.com", "OrgDelReg", "hash")
player, _ := CreateUser(ctx, database, "player-delreg@example.com", "PlayerDelReg", "hash")

tourn := &models.Tournament{
Name:        "Del Reg",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)
CreateRegistration(ctx, database, tourn.ID, player.ID, player.DisplayName)

err := DeleteRegistration(ctx, database, tourn.ID, player.ID)
if err != nil {
t.Fatalf("DeleteRegistration: %v", err)
}

_, err = GetRegistration(ctx, database, tourn.ID, player.ID)
if err != sql.ErrNoRows {
t.Errorf("expected sql.ErrNoRows, got %v", err)
}
}

func TestListUserRegistrations(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-listreg@example.com", "OrgListReg", "hash")
player, _ := CreateUser(ctx, database, "player-listreg@example.com", "PlayerListReg", "hash")

for i := 0; i < 2; i++ {
tourn := &models.Tournament{
Name:        "Tournament",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)
CreateRegistration(ctx, database, tourn.ID, player.ID, player.DisplayName)
}

regs, err := ListUserRegistrations(ctx, database, player.ID)
if err != nil {
t.Fatalf("ListUserRegistrations: %v", err)
}
if len(regs) != 2 {
t.Errorf("registrations count = %d, want 2", len(regs))
}
}

func TestDuplicateRegistration(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-dupreg@example.com", "OrgDupReg", "hash")
player, _ := CreateUser(ctx, database, "player-dupreg@example.com", "PlayerDupReg", "hash")

tourn := &models.Tournament{
Name:        "Dup Reg",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)
CreateRegistration(ctx, database, tourn.ID, player.ID, player.DisplayName)

_, err := CreateRegistration(ctx, database, tourn.ID, player.ID, player.DisplayName)
if err == nil {
t.Error("expected error on duplicate registration")
}
}

func TestCreateGuestRegistration(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-guest@example.com", "OrgGuest", "hash")
tourn := &models.Tournament{
Name:        "Guest Test",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

reg, err := CreateGuestRegistration(ctx, database, tourn.ID, "  Alice  ")
if err != nil {
t.Fatalf("CreateGuestRegistration: %v", err)
}
if !reg.IsGuest() {
t.Error("expected IsGuest() true for guest registration")
}
if reg.UserID != nil {
t.Errorf("UserID = %v, want nil for guest", reg.UserID)
}
if reg.GuestName == nil || *reg.GuestName != "Alice" {
t.Errorf("guest_name = %v, want \"Alice\" (trimmed)", reg.GuestName)
}
if reg.DisplayName != "Alice" {
t.Errorf("display_name = %q, want %q", reg.DisplayName, "Alice")
}
if reg.Status != models.RegistrationStatusConfirmed {
t.Errorf("status = %q, want confirmed", reg.Status)
}
}

func TestGuestNameSuffixOnCollision(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-guestsuffix@example.com", "OrgGuestSuffix", "hash")
tourn := &models.Tournament{
Name:        "Suffix Test",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

// Input casing is preserved on each insert; collision detection is
// case-insensitive so "Alice" and "alice" still bump each other.
inputs := []string{"Alice", "ALICE", "alice"}
expected := []string{"Alice", "ALICE (2)", "alice (3)"}
for i, name := range inputs {
reg, err := CreateGuestRegistration(ctx, database, tourn.ID, name)
if err != nil {
t.Fatalf("CreateGuestRegistration #%d: %v", i, err)
}
if reg.DisplayName != expected[i] {
t.Errorf("registration #%d display_name = %q, want %q", i, reg.DisplayName, expected[i])
}
}
}

func TestUserRegistrationBumpsCollidingGuest(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-bump@example.com", "OrgBump", "hash")
realAlice, _ := CreateUser(ctx, database, "alice-real@example.com", "Alice", "hash")
tourn := &models.Tournament{
Name:        "Bump Test",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

guest, err := CreateGuestRegistration(ctx, database, tourn.ID, "Alice")
if err != nil {
t.Fatalf("CreateGuestRegistration: %v", err)
}
if guest.DisplayName != "Alice" {
t.Fatalf("guest display_name = %q, want %q", guest.DisplayName, "Alice")
}

userReg, err := CreateRegistration(ctx, database, tourn.ID, realAlice.ID, realAlice.DisplayName)
if err != nil {
t.Fatalf("CreateRegistration for real Alice: %v", err)
}
if userReg.DisplayName != "Alice" {
t.Errorf("real-user display_name = %q, want %q (real user keeps name)", userReg.DisplayName, "Alice")
}

bumped, err := GetRegistrationByID(ctx, database, guest.ID)
if err != nil {
t.Fatalf("GetRegistrationByID for bumped guest: %v", err)
}
if bumped.DisplayName != "Alice (2)" {
t.Errorf("bumped guest display_name = %q, want %q", bumped.DisplayName, "Alice (2)")
}
}

func TestUserRegistrationBumpsAroundExistingSuffixes(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-bumparound@example.com", "OrgBumpAround", "hash")
realAlice, _ := CreateUser(ctx, database, "alice-around@example.com", "Alice", "hash")
tourn := &models.Tournament{
Name:        "Bump Around",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

// Three guests already share the base name: Alice, Alice (2), Alice (3).
for i := 0; i < 3; i++ {
if _, err := CreateGuestRegistration(ctx, database, tourn.ID, "Alice"); err != nil {
t.Fatalf("CreateGuestRegistration #%d: %v", i, err)
}
}

// Real-user Alice registers; the lone "Alice" guest should be bumped to the
// next free suffix (Alice (4)), leaving (2) and (3) untouched.
userReg, err := CreateRegistration(ctx, database, tourn.ID, realAlice.ID, realAlice.DisplayName)
if err != nil {
t.Fatalf("CreateRegistration: %v", err)
}
if userReg.DisplayName != "Alice" {
t.Errorf("real-user display_name = %q, want %q", userReg.DisplayName, "Alice")
}

regs, err := ListRegistrations(ctx, database, tourn.ID)
if err != nil {
t.Fatalf("ListRegistrations: %v", err)
}
got := map[string]bool{}
for _, r := range regs {
got[r.DisplayName] = true
}
for _, want := range []string{"Alice", "Alice (2)", "Alice (3)", "Alice (4)"} {
if !got[want] {
t.Errorf("missing display_name %q, got %v", want, got)
}
}
if len(got) != 4 {
t.Errorf("got %d distinct display names, want 4: %v", len(got), got)
}
}

func TestGuestRegistrationListedAndCounted(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-glist@example.com", "OrgGList", "hash")
player, _ := CreateUser(ctx, database, "player-glist@example.com", "PlayerGList", "hash")
tourn := &models.Tournament{
Name:        "Guest List",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

CreateRegistration(ctx, database, tourn.ID, player.ID, player.DisplayName)
CreateGuestRegistration(ctx, database, tourn.ID, "Bob")
CreateGuestRegistration(ctx, database, tourn.ID, "Carol")

regs, err := ListRegistrations(ctx, database, tourn.ID)
if err != nil {
t.Fatalf("ListRegistrations: %v", err)
}
if len(regs) != 3 {
t.Errorf("count = %d, want 3", len(regs))
}
guestCount := 0
for _, r := range regs {
if r.IsGuest() {
guestCount++
}
}
if guestCount != 2 {
t.Errorf("guest count = %d, want 2", guestCount)
}

count, err := CountRegistrations(ctx, database, tourn.ID)
if err != nil {
t.Fatalf("CountRegistrations: %v", err)
}
if count != 3 {
t.Errorf("CountRegistrations = %d, want 3", count)
}
}

func TestDeleteRegistrationByID(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-delbyid@example.com", "OrgDelByID", "hash")
tourn := &models.Tournament{
Name:        "Del By ID",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)

guest, _ := CreateGuestRegistration(ctx, database, tourn.ID, "Eve")
if err := DeleteRegistrationByID(ctx, database, guest.ID); err != nil {
t.Fatalf("DeleteRegistrationByID: %v", err)
}
if _, err := GetRegistrationByID(ctx, database, guest.ID); err != sql.ErrNoRows {
t.Errorf("expected sql.ErrNoRows after delete, got %v", err)
}
}

func TestUpdateRegistrationDecklistByID(t *testing.T) {
database := testDB(t)
ctx := context.Background()

org, _ := CreateUser(ctx, database, "org-deck@example.com", "OrgDeck", "hash")
tourn := &models.Tournament{
Name:        "Deck Test",
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
Status:      models.TournamentStatusRegistrationOpen,
OrganizerID: org.ID,
}
CreateTournament(ctx, database, tourn)
guest, _ := CreateGuestRegistration(ctx, database, tourn.ID, "Frank")

deck := []byte(`{"main":{"Lightning Bolt":4}}`)
if err := UpdateRegistrationDecklistByID(ctx, database, guest.ID, deck); err != nil {
t.Fatalf("UpdateRegistrationDecklistByID: %v", err)
}
got, _ := GetRegistrationByID(ctx, database, guest.ID)
// jsonb canonicalizes whitespace, so compare parsed structures.
var wantParsed, gotParsed map[string]interface{}
if err := json.Unmarshal(deck, &wantParsed); err != nil {
t.Fatalf("unmarshal want: %v", err)
}
if err := json.Unmarshal(got.Decklist, &gotParsed); err != nil {
t.Fatalf("unmarshal got: %v", err)
}
if !reflect.DeepEqual(wantParsed, gotParsed) {
t.Errorf("decklist = %v, want %v", gotParsed, wantParsed)
}
}
