package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/dstathis/openswiss/internal/models"
)

// ErrStaffNotFound is returned when an operation targets a staff row that
// doesn't exist (already revoked, or never granted).
var ErrStaffNotFound = errors.New("tournament staff: not found")

// ErrLastAdmin is returned when an operation would leave a tournament with
// zero admins (removing the last admin, or demoting the last admin to a
// lower tier). Callers should translate this to a 409 / user-facing message
// rather than a generic 500.
var ErrLastAdmin = errors.New("tournament staff: refused, would leave the tournament with no admin")

// StaffMember is a tournament_staff row joined with the user's display name.
// Returned by listing endpoints so the UI/API can render staff without a
// second user lookup per row.
type StaffMember struct {
	UserID      int64                 `json:"user_id"`
	DisplayName string                `json:"display_name"`
	Tier        models.TournamentTier `json:"tier"`
	GrantedBy   *int64                `json:"granted_by,omitempty"`
	GrantedAt   time.Time             `json:"granted_at"`
}

// GetTournamentTier returns the tier the user holds on the tournament, or
// an empty TournamentTier ("") if they hold no per-tournament role. A global
// admin (users.roles contains 'admin') is not implicitly upgraded here —
// callers layer that check on top.
func GetTournamentTier(ctx context.Context, db DBTX, tournamentID, userID int64) (models.TournamentTier, error) {
	var tier models.TournamentTier
	err := db.QueryRowContext(ctx,
		`SELECT tier FROM tournament_staff WHERE tournament_id = $1 AND user_id = $2`,
		tournamentID, userID,
	).Scan(&tier)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return tier, nil
}

// EffectiveTournamentTier returns the tier the user can act with on the
// tournament. A nil user has no tier. A global admin (RoleAdmin) is treated
// as TierAdmin regardless of the staff table. Otherwise the value comes
// from tournament_staff. Returns "" when the user has no access.
func EffectiveTournamentTier(ctx context.Context, db DBTX, tournamentID int64, user *models.User) (models.TournamentTier, error) {
	if user == nil {
		return "", nil
	}
	if user.HasRole(models.RoleAdmin) {
		return models.TierAdmin, nil
	}
	return GetTournamentTier(ctx, db, tournamentID, user.ID)
}

// AddTournamentStaff inserts a staff row. Caller is responsible for any
// authorization or last-admin checks; this is the raw write.
func AddTournamentStaff(ctx context.Context, db DBTX, s *models.TournamentStaff) error {
	return db.QueryRowContext(ctx,
		`INSERT INTO tournament_staff (tournament_id, user_id, tier, granted_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING granted_at`,
		s.TournamentID, s.UserID, s.Tier, s.GrantedBy,
	).Scan(&s.GrantedAt)
}

// ListTournamentStaff returns all staff rows for a tournament joined with
// the user's display name, ordered most-privileged tier first then by
// granted_at. Useful for the management page and the public staff list.
func ListTournamentStaff(ctx context.Context, db DBTX, tournamentID int64) ([]StaffMember, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT ts.user_id, u.display_name, ts.tier, ts.granted_by, ts.granted_at
		 FROM tournament_staff ts
		 JOIN users u ON u.id = ts.user_id
		 WHERE ts.tournament_id = $1
		 ORDER BY CASE ts.tier WHEN 'admin' THEN 0 WHEN 'co_organizer' THEN 1 ELSE 2 END,
		          ts.granted_at`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StaffMember
	for rows.Next() {
		var m StaffMember
		if err := rows.Scan(&m.UserID, &m.DisplayName, &m.Tier, &m.GrantedBy, &m.GrantedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// RemoveTournamentStaff deletes a staff row, refusing (ErrLastAdmin) if the
// removed user is the only remaining admin. The whole check+delete runs in
// one transaction with FOR UPDATE on the row so two concurrent revocations
// can't both pass the check and leave the tournament admin-less.
// Returns ErrStaffNotFound if there's no row for (tournamentID, userID).
func RemoveTournamentStaff(ctx context.Context, database *sql.DB, tournamentID, userID int64) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var current models.TournamentTier
	err = tx.QueryRowContext(ctx,
		`SELECT tier FROM tournament_staff WHERE tournament_id = $1 AND user_id = $2 FOR UPDATE`,
		tournamentID, userID,
	).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaffNotFound
	}
	if err != nil {
		return err
	}

	if current == models.TierAdmin {
		var count int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM tournament_staff WHERE tournament_id = $1 AND tier = 'admin'`,
			tournamentID,
		).Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastAdmin
		}
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM tournament_staff WHERE tournament_id = $1 AND user_id = $2`,
		tournamentID, userID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateTournamentStaffTier changes a staff member's tier. Demoting the only
// remaining admin to a lower tier returns ErrLastAdmin; the change is rolled
// back. Same row-level FOR UPDATE pattern as RemoveTournamentStaff so two
// concurrent demotions can't race past the last-admin check.
// Returns ErrStaffNotFound if there's no row for (tournamentID, userID).
func UpdateTournamentStaffTier(ctx context.Context, database *sql.DB, tournamentID, userID int64, newTier models.TournamentTier) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var current models.TournamentTier
	err = tx.QueryRowContext(ctx,
		`SELECT tier FROM tournament_staff WHERE tournament_id = $1 AND user_id = $2 FOR UPDATE`,
		tournamentID, userID,
	).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaffNotFound
	}
	if err != nil {
		return err
	}
	if current == newTier {
		return nil // no-op
	}

	if current == models.TierAdmin && newTier != models.TierAdmin {
		var count int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM tournament_staff WHERE tournament_id = $1 AND tier = 'admin'`,
			tournamentID,
		).Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastAdmin
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE tournament_staff SET tier = $3 WHERE tournament_id = $1 AND user_id = $2`,
		tournamentID, userID, newTier,
	); err != nil {
		return err
	}
	return tx.Commit()
}
