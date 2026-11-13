package models

import (
	"time"
)

type User struct {
	ID                  int64      `json:"id"`
	Email               string     `json:"email"`
	DisplayName         string     `json:"display_name"`
	PasswordHash        string     `json:"-"`
	Roles               []string   `json:"roles"`
	EmailVerifiedAt     *time.Time `json:"email_verified_at,omitempty"`
	FailedLoginAttempts int        `json:"-"`
	LockedUntil         *time.Time `json:"-"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// IsLocked reports whether the account is currently within a brute-force lockout window.
func (u *User) IsLocked(now time.Time) bool {
	return u.LockedUntil != nil && u.LockedUntil.After(now)
}

// EmailVerified reports whether the user has confirmed their email address.
func (u *User) EmailVerified() bool {
	return u.EmailVerifiedAt != nil
}

func (u *User) HasRole(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type Session struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type APIKey struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	KeyHash   string     `json:"-"`
	Prefix    string     `json:"prefix"`
	Name      string     `json:"name"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type Tournament struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Description     *string    `json:"description,omitempty"`
	ScheduledAt     *time.Time `json:"scheduled_at,omitempty"`
	Location        *string    `json:"location,omitempty"`
	MaxPlayers      int        `json:"max_players"`
	NumRounds       *int       `json:"num_rounds,omitempty"`
	RequireDecklist bool       `json:"require_decklist"`
	DecklistPublic  bool       `json:"decklist_public"`
	PointsWin       int        `json:"points_win"`
	PointsDraw      int        `json:"points_draw"`
	PointsLoss      int        `json:"points_loss"`
	TopCut          int        `json:"top_cut"`
	Status          string     `json:"status"`
	OrganizerID     int64      `json:"organizer_id"`
	EngineState     []byte     `json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// TournamentTier is a per-tournament management role. Compare with AtLeast,
// not ==, so callers can express "judge or above" cleanly.
type TournamentTier string

const (
	TierAdmin       TournamentTier = "admin"
	TierCoOrganizer TournamentTier = "co_organizer"
	TierJudge       TournamentTier = "judge"
)

func (t TournamentTier) rank() int {
	switch t {
	case TierAdmin:
		return 3
	case TierCoOrganizer:
		return 2
	case TierJudge:
		return 1
	}
	return 0
}

// AtLeast reports whether t is the same tier as min or a more-privileged one.
func (t TournamentTier) AtLeast(min TournamentTier) bool {
	return t.rank() >= min.rank()
}

type TournamentStaff struct {
	TournamentID int64          `json:"tournament_id"`
	UserID       int64          `json:"user_id"`
	Tier         TournamentTier `json:"tier"`
	GrantedBy    *int64         `json:"granted_by,omitempty"`
	GrantedAt    time.Time      `json:"granted_at"`
}

type Registration struct {
	ID             int64     `json:"id"`
	TournamentID   int64     `json:"tournament_id"`
	UserID         *int64    `json:"user_id,omitempty"`
	GuestName      *string   `json:"guest_name,omitempty"`
	DisplayName    string    `json:"display_name"`
	Decklist       []byte    `json:"decklist,omitempty"`
	Status         string    `json:"status"`
	EnginePlayerID *int      `json:"engine_player_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// IsGuest reports whether this registration is a guest entry (no user account).
func (r Registration) IsGuest() bool { return r.UserID == nil }

type PasswordReset struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type EmailVerification struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	// NewEmail is set only for pending email *changes*: the address the user
	// asked to move to, applied to the account when the token is consumed.
	// Nil for ordinary signup verifications.
	NewEmail *string `json:"-"`
}

const (
	RolePlayer    = "player"
	RoleOrganizer = "organizer"
	RoleAdmin     = "admin"

	TournamentStatusScheduled        = "scheduled"
	TournamentStatusRegistrationOpen = "registration_open"
	TournamentStatusInProgress       = "in_progress"
	TournamentStatusPlayoff          = "playoff"
	TournamentStatusFinished         = "finished"

	RegistrationStatusPending   = "pending"
	RegistrationStatusConfirmed = "confirmed"
	RegistrationStatusDropped   = "dropped"
)

// CareerStats is a player's aggregated record across every finished
// tournament they played in. Matches played before a drop count; rounds after
// a drop do not exist, so they contribute nothing.
type CareerStats struct {
	Events        int `json:"events"`
	EventsWon     int `json:"events_won"`
	EventsDropped int `json:"events_dropped"`
	BestFinish    int `json:"best_finish"`

	Wins   int `json:"wins"`
	Losses int `json:"losses"`
	Draws  int `json:"draws"`
	Byes   int `json:"byes"`

	GameWins   int `json:"game_wins"`
	GameLosses int `json:"game_losses"`
	GameDraws  int `json:"game_draws"`

	MatchWinPct float64 `json:"match_win_pct"`
	GameWinPct  float64 `json:"game_win_pct"`
}

// TournamentResult is one player's finish in one tournament, joined with the
// tournament it belongs to.
type TournamentResult struct {
	TournamentID   int64      `json:"tournament_id"`
	TournamentName string     `json:"name"`
	ScheduledAt    *time.Time `json:"date,omitempty"`
	FinalRank      int        `json:"final_rank"`
	PlayerCount    int        `json:"player_count"`
	Points         int        `json:"points"`

	Wins   int `json:"wins"`
	Losses int `json:"losses"`
	Draws  int `json:"draws"`

	GameWins   int `json:"game_wins"`
	GameLosses int `json:"game_losses"`
	GameDraws  int `json:"game_draws"`

	OppMatchWinPct float64 `json:"opponent_match_win_pct"`
	GameWinPct     float64 `json:"game_win_pct"`
	OppGameWinPct  float64 `json:"opponent_game_win_pct"`

	Dropped        bool `json:"dropped"`
	DroppedInRound *int `json:"dropped_in_round,omitempty"`
}

// PlayerMatch is a single match from one player's point of view.
type PlayerMatch struct {
	TournamentID   int64  `json:"tournament_id"`
	TournamentName string `json:"tournament_name"`
	Round          int    `json:"round"`
	Playoff        bool   `json:"playoff"`
	RoundLabel     string `json:"round_label"`
	Outcome        string `json:"outcome"`

	GameWins   int `json:"game_wins"`
	GameLosses int `json:"game_losses"`
	GameDraws  int `json:"game_draws"`

	// OpponentName is the name the opponent played under, preserved from the
	// event. OpponentDisplayName is their current name, used for links, and is
	// empty for guests and for deleted accounts. Both are kept because a
	// rename makes them differ and neither can be derived from the other.
	OpponentUserID      *int64 `json:"-"`
	OpponentName        string `json:"-"`
	OpponentDisplayName string `json:"-"`
}

// IsBye reports whether this row records a bye rather than a played match.
func (m PlayerMatch) IsBye() bool { return m.Outcome == "bye" }

// HeadToHead is a player's aggregate record against one opponent. Only
// opponents with a user account appear; byes and guests are excluded, because
// guests have no identity that is stable across tournaments.
type HeadToHead struct {
	OpponentUserID      int64  `json:"-"`
	OpponentDisplayName string `json:"-"`
	Wins                int    `json:"wins"`
	Losses              int    `json:"losses"`
	Draws               int    `json:"draws"`
	Matches             int    `json:"matches"`
}
