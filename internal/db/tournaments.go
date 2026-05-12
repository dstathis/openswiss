package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/dstathis/openswiss/internal/models"
)

func CreateTournament(ctx context.Context, database *sql.DB, t *models.Tournament) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := tx.QueryRowContext(ctx,
		`INSERT INTO tournaments (name, description, scheduled_at, location, max_players, num_rounds,
		 require_decklist, decklist_public, points_win, points_draw, points_loss, top_cut, status, organizer_id, engine_state)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 RETURNING id, created_at, updated_at`,
		t.Name, t.Description, t.ScheduledAt, t.Location, t.MaxPlayers, t.NumRounds,
		t.RequireDecklist, t.DecklistPublic, t.PointsWin, t.PointsDraw, t.PointsLoss,
		t.TopCut, t.Status, t.OrganizerID, t.EngineState,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return err
	}

	// Creator becomes the first Admin. All permission checks route through
	// tournament_staff, so a tournament with no admin row would be
	// unmanageable; doing this in the same tx preserves that invariant.
	if err := AddTournamentStaff(ctx, tx, &models.TournamentStaff{
		TournamentID: t.ID,
		UserID:       t.OrganizerID,
		Tier:         models.TierAdmin,
		GrantedBy:    &t.OrganizerID,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

func GetTournament(ctx context.Context, db *sql.DB, id int64) (*models.Tournament, error) {
	t := &models.Tournament{}
	err := db.QueryRowContext(ctx,
		`SELECT id, name, description, scheduled_at, location, max_players, num_rounds,
		 require_decklist, decklist_public, points_win, points_draw, points_loss, top_cut,
		 status, organizer_id, engine_state, created_at, updated_at
		 FROM tournaments WHERE id = $1`,
		id,
	).Scan(&t.ID, &t.Name, &t.Description, &t.ScheduledAt, &t.Location, &t.MaxPlayers,
		&t.NumRounds, &t.RequireDecklist, &t.DecklistPublic, &t.PointsWin, &t.PointsDraw,
		&t.PointsLoss, &t.TopCut, &t.Status, &t.OrganizerID, &t.EngineState, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetTournamentForUpdate locks the row for update within a transaction.
func GetTournamentForUpdate(ctx context.Context, tx *sql.Tx, id int64) (*models.Tournament, error) {
	t := &models.Tournament{}
	err := tx.QueryRowContext(ctx,
		`SELECT id, name, description, scheduled_at, location, max_players, num_rounds,
		 require_decklist, decklist_public, points_win, points_draw, points_loss, top_cut,
		 status, organizer_id, engine_state, created_at, updated_at
		 FROM tournaments WHERE id = $1 FOR UPDATE`,
		id,
	).Scan(&t.ID, &t.Name, &t.Description, &t.ScheduledAt, &t.Location, &t.MaxPlayers,
		&t.NumRounds, &t.RequireDecklist, &t.DecklistPublic, &t.PointsWin, &t.PointsDraw,
		&t.PointsLoss, &t.TopCut, &t.Status, &t.OrganizerID, &t.EngineState, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func UpdateTournamentEngineState(ctx context.Context, tx *sql.Tx, id int64, status string, engineState []byte) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE tournaments SET engine_state = $1, status = $2, updated_at = now() WHERE id = $3`,
		engineState, status, id,
	)
	return err
}

func UpdateTournamentStatus(ctx context.Context, db *sql.DB, id int64, status string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE tournaments SET status = $1, updated_at = now() WHERE id = $2`,
		status, id,
	)
	return err
}

func UpdateTournament(ctx context.Context, db *sql.DB, t *models.Tournament) error {
	_, err := db.ExecContext(ctx,
		`UPDATE tournaments SET name=$1, description=$2, scheduled_at=$3, location=$4,
		 max_players=$5, num_rounds=$6, require_decklist=$7, decklist_public=$8,
		 points_win=$9, points_draw=$10, points_loss=$11, top_cut=$12, updated_at=now()
		 WHERE id=$13`,
		t.Name, t.Description, t.ScheduledAt, t.Location, t.MaxPlayers, t.NumRounds,
		t.RequireDecklist, t.DecklistPublic, t.PointsWin, t.PointsDraw, t.PointsLoss,
		t.TopCut, t.ID,
	)
	return err
}

func DeleteTournament(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM tournaments WHERE id = $1`, id)
	return err
}

func ListTournaments(ctx context.Context, db *sql.DB, status string, page, perPage int) ([]models.Tournament, error) {
	offset := (page - 1) * perPage
	var rows *sql.Rows
	var err error

	if status != "" {
		rows, err = db.QueryContext(ctx,
			`SELECT id, name, description, scheduled_at, location, max_players, num_rounds,
			 require_decklist, decklist_public, points_win, points_draw, points_loss, top_cut,
			 status, organizer_id, created_at, updated_at
			 FROM tournaments WHERE status = $1 ORDER BY scheduled_at DESC NULLS LAST, id DESC LIMIT $2 OFFSET $3`,
			status, perPage, offset,
		)
	} else {
		rows, err = db.QueryContext(ctx,
			`SELECT id, name, description, scheduled_at, location, max_players, num_rounds,
			 require_decklist, decklist_public, points_win, points_draw, points_loss, top_cut,
			 status, organizer_id, created_at, updated_at
			 FROM tournaments ORDER BY scheduled_at DESC NULLS LAST, id DESC LIMIT $1 OFFSET $2`,
			perPage, offset,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tournaments []models.Tournament
	for rows.Next() {
		var t models.Tournament
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.ScheduledAt, &t.Location,
			&t.MaxPlayers, &t.NumRounds, &t.RequireDecklist, &t.DecklistPublic,
			&t.PointsWin, &t.PointsDraw, &t.PointsLoss, &t.TopCut,
			&t.Status, &t.OrganizerID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tournaments = append(tournaments, t)
	}
	return tournaments, rows.Err()
}

func ListUpcomingTournaments(ctx context.Context, db *sql.DB, limit int) ([]models.Tournament, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, description, scheduled_at, location, max_players, num_rounds,
		 require_decklist, decklist_public, points_win, points_draw, points_loss, top_cut,
		 status, organizer_id, created_at, updated_at
		 FROM tournaments WHERE status IN ('scheduled','registration_open')
		 ORDER BY scheduled_at ASC NULLS LAST LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tournaments []models.Tournament
	for rows.Next() {
		var t models.Tournament
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.ScheduledAt, &t.Location,
			&t.MaxPlayers, &t.NumRounds, &t.RequireDecklist, &t.DecklistPublic,
			&t.PointsWin, &t.PointsDraw, &t.PointsLoss, &t.TopCut,
			&t.Status, &t.OrganizerID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tournaments = append(tournaments, t)
	}
	return tournaments, rows.Err()
}

// Registrations
//
// Registrations may be either a real user (user_id NOT NULL, guest_name NULL)
// or a guest added by the organizer (user_id NULL, guest_name NOT NULL).
// display_name is denormalized onto the row so a single unique index
// (tournament_id, lower(display_name)) prevents collisions across both kinds.

const regCols = `id, tournament_id, user_id, guest_name, display_name, decklist, status, engine_player_id, created_at`

func scanRegistration(row interface {
	Scan(dest ...interface{}) error
}) (*models.Registration, error) {
	r := &models.Registration{}
	err := row.Scan(&r.ID, &r.TournamentID, &r.UserID, &r.GuestName, &r.DisplayName, &r.Decklist, &r.Status, &r.EnginePlayerID, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// existingDisplayNames returns the set of lower-cased display names already used
// in this tournament. Caller must hold a row lock on the tournament for the
// result to be safe to use.
func existingDisplayNames(ctx context.Context, tx *sql.Tx, tournamentID int64) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT lower(display_name) FROM registrations WHERE tournament_id = $1`, tournamentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	taken := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		taken[n] = true
	}
	return taken, rows.Err()
}

// nextFreeName returns the smallest variant of base — "base", "base (2)",
// "base (3)", … — whose lower-cased form is not in the taken set.
func nextFreeName(base string, taken map[string]bool) string {
	if !taken[strings.ToLower(base)] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s (%d)", base, i)
		if !taken[strings.ToLower(candidate)] {
			return candidate
		}
	}
}

// lockTournament acquires SELECT … FOR UPDATE on the tournament row to
// serialize registration writes for that tournament.
func lockTournament(ctx context.Context, tx *sql.Tx, tournamentID int64) error {
	var id int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM tournaments WHERE id = $1 FOR UPDATE`, tournamentID,
	).Scan(&id); err != nil {
		return fmt.Errorf("lock tournament: %w", err)
	}
	return nil
}

// CreateGuestRegistration inserts a guest (no user account) into a tournament.
// If the requested name collides with an existing player, the guest's name is
// suffixed with " (2)", " (3)", … until unique. Returns the stored registration
// (its DisplayName/GuestName may differ from the input name).
func CreateGuestRegistration(ctx context.Context, database *sql.DB, tournamentID int64, name string) (*models.Registration, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := lockTournament(ctx, tx, tournamentID); err != nil {
		return nil, err
	}
	taken, err := existingDisplayNames(ctx, tx, tournamentID)
	if err != nil {
		return nil, err
	}
	finalName := nextFreeName(name, taken)

	row := tx.QueryRowContext(ctx,
		`INSERT INTO registrations (tournament_id, user_id, guest_name, display_name, status)
		 VALUES ($1, NULL, $2, $2, 'confirmed')
		 RETURNING `+regCols,
		tournamentID, finalName,
	)
	r, err := scanRegistration(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r, nil
}

// createUserRegistration inserts a registration for a real user. If a guest
// already holds the user's display_name in this tournament, the guest is
// renamed to the next free suffix so the real user keeps their name.
func createUserRegistration(ctx context.Context, database *sql.DB, tournamentID, userID int64, displayName, status string) (*models.Registration, error) {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := lockTournament(ctx, tx, tournamentID); err != nil {
		return nil, err
	}

	var collidingID int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM registrations
		 WHERE tournament_id = $1 AND lower(display_name) = lower($2)`,
		tournamentID, displayName,
	).Scan(&collidingID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil {
		// Collision must be a guest — users.display_name is globally unique.
		// Keep the original name in the taken set so nextFreeName skips it and
		// returns the smallest "Name (n)" variant that's actually free.
		taken, err := existingDisplayNames(ctx, tx, tournamentID)
		if err != nil {
			return nil, err
		}
		bumped := nextFreeName(displayName, taken)
		if _, err := tx.ExecContext(ctx,
			`UPDATE registrations SET display_name = $1, guest_name = $1 WHERE id = $2`,
			bumped, collidingID,
		); err != nil {
			return nil, err
		}
	}

	row := tx.QueryRowContext(ctx,
		`INSERT INTO registrations (tournament_id, user_id, guest_name, display_name, status)
		 VALUES ($1, $2, NULL, $3, $4)
		 RETURNING `+regCols,
		tournamentID, userID, displayName, status,
	)
	r, err := scanRegistration(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r, nil
}

func CreateRegistration(ctx context.Context, database *sql.DB, tournamentID, userID int64, displayName string) (*models.Registration, error) {
	return createUserRegistration(ctx, database, tournamentID, userID, displayName, models.RegistrationStatusConfirmed)
}

func CreatePendingRegistration(ctx context.Context, database *sql.DB, tournamentID, userID int64, displayName string) (*models.Registration, error) {
	return createUserRegistration(ctx, database, tournamentID, userID, displayName, models.RegistrationStatusPending)
}

func GetRegistration(ctx context.Context, database *sql.DB, tournamentID, userID int64) (*models.Registration, error) {
	row := database.QueryRowContext(ctx,
		`SELECT `+regCols+` FROM registrations
		 WHERE tournament_id = $1 AND user_id = $2`,
		tournamentID, userID,
	)
	return scanRegistration(row)
}

func GetRegistrationByID(ctx context.Context, database *sql.DB, regID int64) (*models.Registration, error) {
	row := database.QueryRowContext(ctx,
		`SELECT `+regCols+` FROM registrations WHERE id = $1`,
		regID,
	)
	return scanRegistration(row)
}

func ListRegistrations(ctx context.Context, database *sql.DB, tournamentID int64) ([]models.Registration, error) {
	rows, err := database.QueryContext(ctx,
		`SELECT `+regCols+` FROM registrations
		 WHERE tournament_id = $1 ORDER BY created_at`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regs []models.Registration
	for rows.Next() {
		r, err := scanRegistration(rows)
		if err != nil {
			return nil, err
		}
		regs = append(regs, *r)
	}
	return regs, rows.Err()
}

// UpdateRegistrationDecklist updates the decklist for a real user's registration
// and marks it confirmed (the player-self-service path).
func UpdateRegistrationDecklist(ctx context.Context, database *sql.DB, tournamentID, userID int64, decklist []byte) error {
	_, err := database.ExecContext(ctx,
		`UPDATE registrations SET decklist = $1, status = 'confirmed'
		 WHERE tournament_id = $2 AND user_id = $3`,
		decklist, tournamentID, userID,
	)
	return err
}

// UpdateRegistrationDecklistByID updates a registration's decklist by its id
// (used by organizer-edit paths and works for guests too).
func UpdateRegistrationDecklistByID(ctx context.Context, database *sql.DB, regID int64, decklist []byte) error {
	_, err := database.ExecContext(ctx,
		`UPDATE registrations SET decklist = $1, status = 'confirmed' WHERE id = $2`,
		decklist, regID,
	)
	return err
}

// UpdateRegistrationEnginePlayerID sets the engine_player_id on a registration
// by registration id. Accepts a *sql.DB or *sql.Tx.
func UpdateRegistrationEnginePlayerID(ctx context.Context, dbtx interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}, regID int64, enginePlayerID int) error {
	_, err := dbtx.ExecContext(ctx,
		`UPDATE registrations SET engine_player_id = $1 WHERE id = $2`,
		enginePlayerID, regID,
	)
	return err
}

func UpdateRegistrationStatus(ctx context.Context, database *sql.DB, tournamentID, userID int64, status string) error {
	_, err := database.ExecContext(ctx,
		`UPDATE registrations SET status = $1
		 WHERE tournament_id = $2 AND user_id = $3`,
		status, tournamentID, userID,
	)
	return err
}

func UpdateRegistrationStatusByID(ctx context.Context, database *sql.DB, regID int64, status string) error {
	_, err := database.ExecContext(ctx,
		`UPDATE registrations SET status = $1 WHERE id = $2`, status, regID,
	)
	return err
}

func DeleteRegistration(ctx context.Context, database *sql.DB, tournamentID, userID int64) error {
	_, err := database.ExecContext(ctx,
		`DELETE FROM registrations WHERE tournament_id = $1 AND user_id = $2`,
		tournamentID, userID,
	)
	return err
}

func DeleteRegistrationByID(ctx context.Context, database *sql.DB, regID int64) error {
	_, err := database.ExecContext(ctx,
		`DELETE FROM registrations WHERE id = $1`, regID,
	)
	return err
}

func CountRegistrations(ctx context.Context, database *sql.DB, tournamentID int64) (int, error) {
	var count int
	err := database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM registrations WHERE tournament_id = $1 AND status != 'dropped'`,
		tournamentID,
	).Scan(&count)
	return count, err
}

func GetRegistrationByEnginePlayerID(ctx context.Context, database *sql.DB, tournamentID int64, enginePlayerID int) (*models.Registration, error) {
	row := database.QueryRowContext(ctx,
		`SELECT `+regCols+` FROM registrations
		 WHERE tournament_id = $1 AND engine_player_id = $2`,
		tournamentID, enginePlayerID,
	)
	return scanRegistration(row)
}

func ListUserRegistrations(ctx context.Context, database *sql.DB, userID int64) ([]models.Registration, error) {
	rows, err := database.QueryContext(ctx,
		`SELECT `+regCols+` FROM registrations
		 WHERE user_id = $1 AND status != 'dropped' ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regs []models.Registration
	for rows.Next() {
		r, err := scanRegistration(rows)
		if err != nil {
			return nil, err
		}
		regs = append(regs, *r)
	}
	return regs, rows.Err()
}
