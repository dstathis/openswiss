//go:build integration

package db

import (
"context"
"database/sql"
"fmt"
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

reg, err := CreateRegistration(ctx, database, tourn.ID, player.ID)
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

reg, err := CreatePendingRegistration(ctx, database, tourn.ID, player.ID)
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
CreateRegistration(ctx, database, tourn.ID, player.ID)

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
CreateRegistration(ctx, database, tourn.ID, player.ID)
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
CreateRegistration(ctx, database, tourn.ID, player.ID)

_, err := CreateRegistration(ctx, database, tourn.ID, player.ID)
if err == nil {
t.Error("expected error on duplicate registration")
}
}
