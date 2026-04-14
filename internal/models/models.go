package models

import (
	"time"
)

type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	PasswordHash string    `json:"-"`
	Roles        []string  `json:"roles"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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

type Registration struct {
	ID             int64     `json:"id"`
	TournamentID   int64     `json:"tournament_id"`
	UserID         int64     `json:"user_id"`
	Decklist       []byte    `json:"decklist,omitempty"`
	Status         string    `json:"status"`
	EnginePlayerID *int      `json:"engine_player_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	DisplayName    string    `json:"display_name,omitempty"` // joined from users table
}

type PasswordReset struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
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
