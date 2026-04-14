package db

import (
	"context"
	"database/sql"

	"github.com/dstathis/openswiss/internal/models"
)

func CreateTournament(ctx context.Context, db *sql.DB, t *models.Tournament) error {
	return db.QueryRowContext(ctx,
		`INSERT INTO tournaments (name, description, scheduled_at, location, max_players, num_rounds,
		 require_decklist, decklist_public, points_win, points_draw, points_loss, top_cut, status, organizer_id, engine_state)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 RETURNING id, created_at, updated_at`,
		t.Name, t.Description, t.ScheduledAt, t.Location, t.MaxPlayers, t.NumRounds,
		t.RequireDecklist, t.DecklistPublic, t.PointsWin, t.PointsDraw, t.PointsLoss,
		t.TopCut, t.Status, t.OrganizerID, t.EngineState,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
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

func CreateRegistration(ctx context.Context, dbtx interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}, tournamentID, userID int64) (*models.Registration, error) {
	r := &models.Registration{}
	err := dbtx.QueryRowContext(ctx,
		`INSERT INTO registrations (tournament_id, user_id, status)
		 VALUES ($1, $2, 'confirmed')
		 RETURNING id, tournament_id, user_id, decklist, status, engine_player_id, created_at`,
		tournamentID, userID,
	).Scan(&r.ID, &r.TournamentID, &r.UserID, &r.Decklist, &r.Status, &r.EnginePlayerID, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func CreatePendingRegistration(ctx context.Context, db *sql.DB, tournamentID, userID int64) (*models.Registration, error) {
	r := &models.Registration{}
	err := db.QueryRowContext(ctx,
		`INSERT INTO registrations (tournament_id, user_id, status)
		 VALUES ($1, $2, 'pending')
		 RETURNING id, tournament_id, user_id, decklist, status, engine_player_id, created_at`,
		tournamentID, userID,
	).Scan(&r.ID, &r.TournamentID, &r.UserID, &r.Decklist, &r.Status, &r.EnginePlayerID, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func GetRegistration(ctx context.Context, db *sql.DB, tournamentID, userID int64) (*models.Registration, error) {
	r := &models.Registration{}
	err := db.QueryRowContext(ctx,
		`SELECT r.id, r.tournament_id, r.user_id, r.decklist, r.status, r.engine_player_id, r.created_at, u.display_name
		 FROM registrations r JOIN users u ON r.user_id = u.id
		 WHERE r.tournament_id = $1 AND r.user_id = $2`,
		tournamentID, userID,
	).Scan(&r.ID, &r.TournamentID, &r.UserID, &r.Decklist, &r.Status, &r.EnginePlayerID, &r.CreatedAt, &r.DisplayName)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func ListRegistrations(ctx context.Context, db *sql.DB, tournamentID int64) ([]models.Registration, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT r.id, r.tournament_id, r.user_id, r.decklist, r.status, r.engine_player_id, r.created_at, u.display_name
		 FROM registrations r JOIN users u ON r.user_id = u.id
		 WHERE r.tournament_id = $1 ORDER BY r.created_at`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regs []models.Registration
	for rows.Next() {
		var r models.Registration
		if err := rows.Scan(&r.ID, &r.TournamentID, &r.UserID, &r.Decklist, &r.Status, &r.EnginePlayerID, &r.CreatedAt, &r.DisplayName); err != nil {
			return nil, err
		}
		regs = append(regs, r)
	}
	return regs, rows.Err()
}

func UpdateRegistrationDecklist(ctx context.Context, db *sql.DB, tournamentID, userID int64, decklist []byte) error {
	_, err := db.ExecContext(ctx,
		`UPDATE registrations SET decklist = $1, status = 'confirmed' WHERE tournament_id = $2 AND user_id = $3`,
		decklist, tournamentID, userID,
	)
	return err
}

func UpdateRegistrationEnginePlayerID(ctx context.Context, dbtx interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}, tournamentID, userID int64, enginePlayerID int) error {
	_, err := dbtx.ExecContext(ctx,
		`UPDATE registrations SET engine_player_id = $1 WHERE tournament_id = $2 AND user_id = $3`,
		enginePlayerID, tournamentID, userID,
	)
	return err
}

func UpdateRegistrationStatus(ctx context.Context, db *sql.DB, tournamentID, userID int64, status string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE registrations SET status = $1 WHERE tournament_id = $2 AND user_id = $3`,
		status, tournamentID, userID,
	)
	return err
}

func DeleteRegistration(ctx context.Context, db *sql.DB, tournamentID, userID int64) error {
	_, err := db.ExecContext(ctx,
		`DELETE FROM registrations WHERE tournament_id = $1 AND user_id = $2`,
		tournamentID, userID,
	)
	return err
}

func CountRegistrations(ctx context.Context, db *sql.DB, tournamentID int64) (int, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM registrations WHERE tournament_id = $1 AND status != 'dropped'`,
		tournamentID,
	).Scan(&count)
	return count, err
}

func GetRegistrationByEnginePlayerID(ctx context.Context, db *sql.DB, tournamentID int64, enginePlayerID int) (*models.Registration, error) {
	r := &models.Registration{}
	err := db.QueryRowContext(ctx,
		`SELECT r.id, r.tournament_id, r.user_id, r.decklist, r.status, r.engine_player_id, r.created_at, u.display_name
		 FROM registrations r JOIN users u ON r.user_id = u.id
		 WHERE r.tournament_id = $1 AND r.engine_player_id = $2`,
		tournamentID, enginePlayerID,
	).Scan(&r.ID, &r.TournamentID, &r.UserID, &r.Decklist, &r.Status, &r.EnginePlayerID, &r.CreatedAt, &r.DisplayName)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func ListUserRegistrations(ctx context.Context, db *sql.DB, userID int64) ([]models.Registration, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT r.id, r.tournament_id, r.user_id, r.decklist, r.status, r.engine_player_id, r.created_at, ''
		 FROM registrations r WHERE r.user_id = $1 AND r.status != 'dropped' ORDER BY r.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regs []models.Registration
	for rows.Next() {
		var r models.Registration
		if err := rows.Scan(&r.ID, &r.TournamentID, &r.UserID, &r.Decklist, &r.Status, &r.EnginePlayerID, &r.CreatedAt, &r.DisplayName); err != nil {
			return nil, err
		}
		regs = append(regs, r)
	}
	return regs, rows.Err()
}
