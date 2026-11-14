// Command devseed populates a local database with a realistic tournament
// history, so the player statistics pages have something to show when trying
// the UI by hand.
//
// Normally invoked through `make demo` (seed and serve) or `make demo-seed`
// (rebuild the data only). It is destructive: it truncates every table first,
// so point it at a throwaway database, never at anything you care about.
//
//	DATABASE_URL=postgres://... go run ./cmd/devseed
//
// Results are randomised from a fixed seed, so repeated runs produce the same
// tournament history.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/dstathis/openswiss/internal/auth"
	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/engine"
	"github.com/dstathis/openswiss/internal/models"
	st "github.com/dstathis/swisstools"
	_ "github.com/lib/pq"
)

const password = "password123"

var rng = rand.New(rand.NewSource(20260917)) // fixed seed: reproducible runs

type seeder struct {
	ctx      context.Context
	database *sql.DB
	org      *models.User
	users    map[string]*models.User
}

func main() {
	database, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	if err := database.Ping(); err != nil {
		log.Fatal("ping: ", err)
	}

	s := &seeder{ctx: context.Background(), database: database, users: map[string]*models.User{}}

	// Wipe anything already there so re-running is clean.
	if _, err := database.Exec(`TRUNCATE round_timers, matches, tournament_results, registrations,
		tournament_staff, tournaments, sessions, api_keys, password_resets,
		email_verifications, users RESTART IDENTITY CASCADE`); err != nil {
		log.Fatal("truncate: ", err)
	}

	s.org = s.user("organizer@openswiss.test", "TournamentOrg", models.RolePlayer, models.RoleOrganizer, models.RoleAdmin)

	// Dylan is in most events, so logging in and clicking Stats lands on a
	// page with real history rather than an empty state.
	roster := []string{
		"Dylan", "Alice", "Bob", "Carol", "Dave", "Erin",
		"Frank", "Grace", "Heidi", "Ivan", "Judy", "Mallory",
	}
	for i, n := range roster {
		s.user(fmt.Sprintf("%s@openswiss.test", lower(n)), n, models.RolePlayer)
		_ = i
	}

	// A spread of finished events: different sizes, round counts, byes,
	// guests, drops and top cuts.
	s.runTournament(tourneySpec{
		name: "Spring Open", daysAgo: 120, rounds: 3, topCut: 4,
		players: []string{"Dylan", "Alice", "Bob", "Carol", "Dave", "Erin", "Frank", "Grace"},
	})
	s.runTournament(tourneySpec{
		// Odd entrant count: every round has a bye.
		name: "Summer Classic", daysAgo: 84, rounds: 3, topCut: 0,
		players: []string{"Dylan", "Alice", "Carol", "Erin", "Grace", "Heidi"},
		guests:  []string{"Walk In"},
		// Dylan asks to drop but the organizer never actions it: he keeps
		// playing and keeps a normal finishing rank.
		requestDrop: "Dylan",
	})
	s.runTournament(tourneySpec{
		name: "Autumn Invitational", daysAgo: 45, rounds: 4, topCut: 4,
		players: []string{"Dylan", "Alice", "Bob", "Carol", "Dave", "Erin", "Frank",
			"Grace", "Heidi", "Ivan", "Judy", "Mallory"},
		// A real drop, by the organizer, part way through.
		organizerDrop:      "Judy",
		organizerDropRound: 3,
	})
	s.runTournament(tourneySpec{
		name: "Winter Showdown", daysAgo: 12, rounds: 3, topCut: 4,
		players: []string{"Dylan", "Bob", "Dave", "Frank", "Heidi", "Ivan", "Judy"},
		guests:  []string{"Guest Player"},
	})

	// One event still running: it must NOT appear in anyone's statistics.
	s.runTournament(tourneySpec{
		name: "Midweek League", daysAgo: -3, rounds: 3, topCut: 0,
		players:        []string{"Dylan", "Alice", "Bob", "Carol", "Dave", "Erin"},
		stopAfterRound: 1,
		// A live round with a running clock, so the detail page shows the
		// timer and pairings at the top without anyone pressing Start.
		roundMinutes: 50,
		timerMinutes: 50,
	})

	// One open for registration, so the register button is clickable.
	s.openForRegistration("Next Month's Showcase", -30)

	log.Printf("seeded: 4 finished tournaments, 1 in progress (with a running round timer), 1 open for registration")
	log.Printf("login with any of: dylan@openswiss.test / organizer@openswiss.test  (password: %s)", password)
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

func (s *seeder) user(email, name string, roles ...string) *models.User {
	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Fatal(err)
	}
	u, err := db.CreateUser(s.ctx, s.database, email, name, hash)
	if err != nil {
		log.Fatalf("create user %s: %v", name, err)
	}
	if err := db.UpdateUserRoles(s.ctx, s.database, u.ID, roles); err != nil {
		log.Fatal(err)
	}
	// Verified, so these accounts can log in even if SMTP is later enabled.
	if err := db.MarkUserEmailVerified(s.ctx, s.database, u.ID); err != nil {
		log.Fatal(err)
	}
	s.users[name] = u
	return u
}

type tourneySpec struct {
	name               string
	daysAgo            int // negative means in the future
	rounds             int
	topCut             int
	players            []string
	guests             []string
	requestDrop        string // sets registration status only; engine untouched
	organizerDrop      string // real engine removal
	organizerDropRound int
	stopAfterRound     int // leave the tournament in progress
	roundMinutes       int // the tournament's default round length setting
	timerMinutes       int // start a running round clock (only with stopAfterRound)
}

func (s *seeder) runTournament(spec tourneySpec) {
	when := time.Now().AddDate(0, 0, -spec.daysAgo)
	location := "The Game Room"
	desc := fmt.Sprintf("A %d-round Swiss event. Seeded demo data.", spec.rounds)
	t := &models.Tournament{
		Name: spec.name, Description: &desc, ScheduledAt: &when, Location: &location,
		MaxPlayers: 32, NumRounds: &spec.rounds,
		RoundMinutes: roundMinutesOrNil(spec.roundMinutes),
		PointsWin:    3, PointsDraw: 1, PointsLoss: 0, TopCut: spec.topCut,
		Status: models.TournamentStatusRegistrationOpen, OrganizerID: s.org.ID,
	}
	if err := db.CreateTournament(s.ctx, s.database, t); err != nil {
		log.Fatalf("%s: create: %v", spec.name, err)
	}
	for _, n := range spec.players {
		u := s.users[n]
		if _, err := db.CreateRegistration(s.ctx, s.database, t.ID, u.ID, u.DisplayName); err != nil {
			log.Fatalf("%s: register %s: %v", spec.name, n, err)
		}
	}
	for _, g := range spec.guests {
		if _, err := db.CreateGuestRegistration(s.ctx, s.database, t.ID, g); err != nil {
			log.Fatalf("%s: guest %s: %v", spec.name, g, err)
		}
	}

	regs, _ := db.ListRegistrations(s.ctx, s.database, t.ID)
	if err := engine.WithTournamentEngine(s.ctx, s.database, t.ID,
		func(tx *sql.Tx, tm *models.Tournament, eng *st.Tournament) (string, error) {
			state, err := engine.InitTournamentEngine(s.ctx, tx, tm, regs)
			if err != nil {
				return "", err
			}
			ne, err := st.LoadTournament(state)
			if err != nil {
				return "", err
			}
			*eng = ne
			return models.TournamentStatusInProgress, nil
		}); err != nil {
		log.Fatalf("%s: start: %v", spec.name, err)
	}

	if spec.requestDrop != "" {
		// The player-facing "Request Drop": registration status only.
		if err := db.UpdateRegistrationStatus(s.ctx, s.database, t.ID,
			s.users[spec.requestDrop].ID, models.RegistrationStatusDropped); err != nil {
			log.Fatal(err)
		}
	}

	for round := 1; round <= spec.rounds; round++ {
		if spec.stopAfterRound > 0 && round > spec.stopAfterRound {
			s.startRoundTimer(t.ID, spec)
			log.Printf("%-22s left in progress after round %d", spec.name, spec.stopAfterRound)
			return
		}
		if spec.organizerDrop != "" && round == spec.organizerDropRound {
			s.dropPlayer(t.ID, spec.organizerDrop)
		}
		s.enterResults(t.ID)
		if spec.stopAfterRound > 0 && round == spec.stopAfterRound {
			s.startRoundTimer(t.ID, spec)
			log.Printf("%-22s left in progress after round %d", spec.name, spec.stopAfterRound)
			return
		}
		s.advanceRound(t.ID)
	}

	if spec.topCut > 0 {
		s.runPlayoff(t.ID, spec.topCut, spec.name)
	}
	log.Printf("%-22s finished", spec.name)
}

func roundMinutesOrNil(m int) *int {
	if m <= 0 {
		return nil
	}
	return &m
}

// startRoundTimer puts a running clock on the round the demo stops at, so the
// seeded instance shows the timer at the top of a live tournament page without
// anyone having to press Start first.
func (s *seeder) startRoundTimer(tournamentID int64, spec tourneySpec) {
	if spec.timerMinutes <= 0 {
		return
	}
	if _, err := db.StartRoundTimer(s.ctx, s.database, tournamentID,
		spec.stopAfterRound, false, time.Duration(spec.timerMinutes)*time.Minute, &s.org.ID); err != nil {
		log.Fatalf("%s: start round timer: %v", spec.name, err)
	}
}

// enterResults mirrors the SubmitResults handler: results only. NextRound and
// FinishTournament accumulate standings themselves.
func (s *seeder) enterResults(tournamentID int64) {
	if err := engine.WithTournamentEngine(s.ctx, s.database, tournamentID,
		func(tx *sql.Tx, tm *models.Tournament, eng *st.Tournament) (string, error) {
			for _, p := range eng.GetRound() {
				if p.PlayerB() == st.BYE_OPPONENT_ID {
					continue
				}
				w, l, d := randomGameScore()
				if err := eng.AddResult(p.PlayerA(), w, l, d); err != nil {
					return "", err
				}
			}
			return "", nil
		}); err != nil {
		log.Fatal("results: ", err)
	}
}

// randomGameScore returns a plausible best-of-three result from player A's
// point of view, including the occasional draw.
func randomGameScore() (wins, losses, draws int) {
	switch rng.Intn(10) {
	case 0:
		return 1, 1, 1 // drawn match
	case 1, 2, 3:
		return 2, 1, 0
	case 4, 5:
		return 1, 2, 0
	case 6, 7:
		return 2, 0, 0
	default:
		return 0, 2, 0
	}
}

// advanceRound mirrors the NextRound handler.
func (s *seeder) advanceRound(tournamentID int64) {
	if err := engine.WithTournamentEngine(s.ctx, s.database, tournamentID,
		func(tx *sql.Tx, tm *models.Tournament, eng *st.Tournament) (string, error) {
			if err := eng.NextRound(); err != nil {
				return "", err
			}
			if eng.GetStatus() == "finished" {
				return models.TournamentStatusFinished, nil
			}
			return "", eng.Pair(false)
		}); err != nil {
		log.Fatal("advance: ", err)
	}
}

// dropPlayer mirrors the organizer's DropPlayer action: a real engine removal
// plus the registration status change.
func (s *seeder) dropPlayer(tournamentID int64, name string) {
	regs, _ := db.ListRegistrations(s.ctx, s.database, tournamentID)
	var engineID int
	var regID int64
	for _, r := range regs {
		if r.DisplayName == name && r.EnginePlayerID != nil {
			engineID, regID = *r.EnginePlayerID, r.ID
		}
	}
	if engineID == 0 {
		log.Fatalf("drop: %s not found in tournament %d", name, tournamentID)
	}
	if err := engine.WithTournamentEngine(s.ctx, s.database, tournamentID,
		func(tx *sql.Tx, tm *models.Tournament, eng *st.Tournament) (string, error) {
			return "", eng.RemovePlayerById(engineID)
		}); err != nil {
		log.Fatal("drop: ", err)
	}
	if err := db.UpdateRegistrationStatusByID(s.ctx, s.database, regID,
		models.RegistrationStatusDropped); err != nil {
		log.Fatal(err)
	}
}

func (s *seeder) runPlayoff(tournamentID int64, topCut int, name string) {
	if err := engine.WithTournamentEngine(s.ctx, s.database, tournamentID,
		func(tx *sql.Tx, tm *models.Tournament, eng *st.Tournament) (string, error) {
			if err := eng.StartPlayoff(topCut); err != nil {
				return "", err
			}
			return models.TournamentStatusPlayoff, nil
		}); err != nil {
		log.Printf("%s: skipping playoff (%v)", name, err)
		return
	}
	rounds := 0
	for n := topCut; n > 1; n /= 2 {
		rounds++
	}
	for i := 0; i < rounds; i++ {
		last := i == rounds-1
		if err := engine.WithTournamentEngine(s.ctx, s.database, tournamentID,
			func(tx *sql.Tx, tm *models.Tournament, eng *st.Tournament) (string, error) {
				for _, p := range eng.GetPlayoffRound() {
					if p.PlayerB() == st.BYE_OPPONENT_ID {
						continue
					}
					// No draws in a bracket: someone has to advance.
					w, l := 2, rng.Intn(2)
					if rng.Intn(2) == 0 {
						w, l = l, 2
					}
					if err := eng.AddPlayoffResult(p.PlayerA(), w, l, 0); err != nil {
						return "", err
					}
				}
				if err := eng.NextPlayoffRound(); err != nil {
					return "", err
				}
				if last {
					return models.TournamentStatusFinished, nil
				}
				return "", nil
			}); err != nil {
			log.Fatalf("%s: playoff: %v", name, err)
		}
	}
}

func (s *seeder) openForRegistration(name string, daysAgo int) {
	when := time.Now().AddDate(0, 0, -daysAgo)
	location := "The Game Room"
	desc := "Open for registration. Sign in and hit **Register** to join."
	rounds := 4
	t := &models.Tournament{
		Name: name, Description: &desc, ScheduledAt: &when, Location: &location,
		MaxPlayers: 32, NumRounds: &rounds,
		PointsWin: 3, PointsDraw: 1, PointsLoss: 0, TopCut: 8,
		Status: models.TournamentStatusRegistrationOpen, OrganizerID: s.org.ID,
	}
	if err := db.CreateTournament(s.ctx, s.database, t); err != nil {
		log.Fatal(err)
	}
	for _, n := range []string{"Alice", "Bob", "Carol"} {
		u := s.users[n]
		db.CreateRegistration(s.ctx, s.database, t.ID, u.ID, u.DisplayName)
	}
	log.Printf("%-22s open for registration", name)
}
