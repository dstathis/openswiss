//go:build integration

package engine

import (
"context"
"database/sql"
"fmt"
"log"
"math"
"os"
"testing"
"time"

"github.com/dstathis/openswiss/internal/auth"
"github.com/dstathis/openswiss/internal/db"
"github.com/dstathis/openswiss/internal/models"
"github.com/dstathis/swisstools"
"github.com/golang-migrate/migrate/v4"
"github.com/golang-migrate/migrate/v4/database/postgres"
_ "github.com/golang-migrate/migrate/v4/source/file"
_ "github.com/lib/pq"
)

const loadTestPlayers = 5000

func loadTestDB(t *testing.T) *sql.DB {
t.Helper()
dsn := os.Getenv("TEST_DATABASE_URL")
if dsn == "" {
t.Skip("TEST_DATABASE_URL not set")
}
database, err := sql.Open("postgres", dsn)
if err != nil {
t.Fatalf("open db: %v", err)
}
if err := database.Ping(); err != nil {
t.Fatalf("ping db: %v", err)
}

driver, err := postgres.WithInstance(database, &postgres.Config{})
if err != nil {
t.Fatalf("migration driver: %v", err)
}
m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "postgres", driver)
if err != nil {
t.Fatalf("migrator: %v", err)
}
if err := m.Up(); err != nil && err != migrate.ErrNoChange {
t.Fatalf("migrations: %v", err)
}

for _, table := range []string{"password_resets", "registrations", "api_keys", "sessions", "tournaments", "users"} {
if _, err := database.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
t.Fatalf("clean %s: %v", table, err)
}
}

t.Cleanup(func() { database.Close() })
return database
}

func TestLargeScaleTournament(t *testing.T) {
database := loadTestDB(t)
ctx := context.Background()

numRounds := int(math.Ceil(math.Log2(float64(loadTestPlayers))))
t.Logf("=== Large-scale tournament: %d players, %d rounds ===", loadTestPlayers, numRounds)

// --- Step 1: Create organizer ---
start := time.Now()
hash, _ := auth.HashPassword("testpassword123")
organizer, err := db.CreateUser(ctx, database, "organizer@loadtest.com", "Load Test Organizer", hash)
if err != nil {
t.Fatalf("create organizer: %v", err)
}
t.Logf("[setup] organizer created in %v", time.Since(start))

// --- Step 2: Create tournament ---
start = time.Now()
tournament := &models.Tournament{
Name:        "Load Test Tournament",
OrganizerID: organizer.ID,
PointsWin:   3,
PointsDraw:  1,
PointsLoss:  0,
NumRounds:   &numRounds,
Status:      models.TournamentStatusScheduled,
}
if err := db.CreateTournament(ctx, database, tournament); err != nil {
t.Fatalf("create tournament: %v", err)
}
db.UpdateTournamentStatus(ctx, database, tournament.ID, models.TournamentStatusRegistrationOpen)
t.Logf("[setup] tournament created in %v", time.Since(start))

// --- Step 3: Register 5000 players ---
start = time.Now()
for i := 1; i <= loadTestPlayers; i++ {
email := fmt.Sprintf("player%d@loadtest.com", i)
name := fmt.Sprintf("Player %d", i)
u, err := db.CreateUser(ctx, database, email, name, hash)
if err != nil {
t.Fatalf("create user %d: %v", i, err)
}
if _, err := db.CreateRegistration(ctx, database, tournament.ID, u.ID); err != nil {
t.Fatalf("register user %d: %v", i, err)
}
}
t.Logf("[register] %d players registered in %v", loadTestPlayers, time.Since(start))

// --- Step 4: Start tournament (init engine + pair round 1) ---
start = time.Now()
regs, err := db.ListRegistrations(ctx, database, tournament.ID)
if err != nil {
t.Fatalf("list registrations: %v", err)
}
t.Logf("[start] loaded %d registrations in %v", len(regs), time.Since(start))

start = time.Now()
err = WithTournamentEngine(ctx, database, tournament.ID,
func(tx *sql.Tx, tm *models.Tournament, eng *swisstools.Tournament) (string, error) {
state, err := InitTournamentEngine(ctx, tx, tm, regs)
if err != nil {
return "", err
}
newEng, err := swisstools.LoadTournament(state)
if err != nil {
return "", err
}
*eng = newEng
return models.TournamentStatusInProgress, nil
})
if err != nil {
t.Fatalf("start tournament: %v", err)
}
t.Logf("[start] tournament started + round 1 paired in %v", time.Since(start))

// --- Step 5: Play through all rounds ---
for round := 1; round <= numRounds; round++ {
roundStart := time.Now()

// Submit results
resultStart := time.Now()
err = WithTournamentEngine(ctx, database, tournament.ID,
func(tx *sql.Tx, tm *models.Tournament, eng *swisstools.Tournament) (string, error) {
pairings := eng.GetRound()
for _, p := range pairings {
if p.PlayerB() == swisstools.BYE_OPPONENT_ID {
continue
}
// Alternate who wins to create realistic standings
if p.PlayerA()%2 == round%2 {
if err := eng.AddResult(p.PlayerA(), 2, 0, 0); err != nil {
return "", fmt.Errorf("result for player %d: %w", p.PlayerA(), err)
}
} else {
if err := eng.AddResult(p.PlayerA(), 0, 2, 0); err != nil {
return "", fmt.Errorf("result for player %d: %w", p.PlayerA(), err)
}
}
}
return "", nil
})
if err != nil {
t.Fatalf("round %d results: %v", round, err)
}
resultDuration := time.Since(resultStart)

// Next round (or finish)
advanceStart := time.Now()
err = WithTournamentEngine(ctx, database, tournament.ID,
func(tx *sql.Tx, tm *models.Tournament, eng *swisstools.Tournament) (string, error) {
if err := eng.NextRound(); err != nil {
return "", err
}
if eng.GetStatus() == "finished" {
return models.TournamentStatusFinished, nil
}
if err := eng.Pair(false); err != nil {
return "", err
}
return "", nil
})
if err != nil {
t.Fatalf("round %d advance: %v", round, err)
}
advanceDuration := time.Since(advanceStart)

// Get standings
standingsStart := time.Now()
tm, _ := db.GetTournament(ctx, database, tournament.ID)
eng, err := swisstools.LoadTournament(tm.EngineState)
if err != nil {
t.Fatalf("load engine for standings: %v", err)
}
standings := eng.GetStandings()
standingsDuration := time.Since(standingsStart)

t.Logf("[round %2d] results: %v | advance+pair: %v | standings(%d): %v | total: %v",
round, resultDuration, advanceDuration, len(standings), standingsDuration, time.Since(roundStart))
}

// --- Step 6: Verify final state ---
tm, _ := db.GetTournament(ctx, database, tournament.ID)
if tm.Status != models.TournamentStatusFinished {
t.Errorf("expected finished, got %s", tm.Status)
}
eng, _ := swisstools.LoadTournament(tm.EngineState)
standings := eng.GetStandings()
if len(standings) != loadTestPlayers {
t.Errorf("expected %d standings, got %d", loadTestPlayers, len(standings))
}

t.Logf("=== Tournament complete: %d players, %d rounds ===", loadTestPlayers, numRounds)
}

// TestMain allows running just the load test from the command line.
func TestMain(m *testing.M) {
log.SetFlags(log.Lmicroseconds)
os.Exit(m.Run())
}
