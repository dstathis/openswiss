package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/dstathis/openswiss/internal/models"
	"github.com/lib/pq"
)

func CreateUser(ctx context.Context, db *sql.DB, email, displayName, passwordHash string) (*models.User, error) {
	u := &models.User{}
	err := db.QueryRowContext(ctx,
		`INSERT INTO users (email, display_name, password_hash) VALUES ($1, $2, $3)
		 RETURNING id, email, display_name, password_hash, roles, email_verified_at, failed_login_attempts, locked_until, created_at, updated_at`,
		email, displayName, passwordHash,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, pq.Array(&u.Roles), &u.EmailVerifiedAt, &u.FailedLoginAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func GetUserByEmail(ctx context.Context, db *sql.DB, email string) (*models.User, error) {
	u := &models.User{}
	err := db.QueryRowContext(ctx,
		`SELECT id, email, display_name, password_hash, roles, email_verified_at, failed_login_attempts, locked_until, created_at, updated_at FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, pq.Array(&u.Roles), &u.EmailVerifiedAt, &u.FailedLoginAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func GetUserByID(ctx context.Context, db *sql.DB, id int64) (*models.User, error) {
	u := &models.User{}
	err := db.QueryRowContext(ctx,
		`SELECT id, email, display_name, password_hash, roles, email_verified_at, failed_login_attempts, locked_until, created_at, updated_at FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, pq.Array(&u.Roles), &u.EmailVerifiedAt, &u.FailedLoginAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func UpdateUserRoles(ctx context.Context, db *sql.DB, userID int64, roles []string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE users SET roles = $1, updated_at = now() WHERE id = $2`,
		pq.Array(roles), userID,
	)
	return err
}

func ListUsers(ctx context.Context, db *sql.DB, page, perPage int) ([]models.User, error) {
	offset := (page - 1) * perPage
	rows, err := db.QueryContext(ctx,
		`SELECT id, email, display_name, password_hash, roles, email_verified_at, failed_login_attempts, locked_until, created_at, updated_at
		 FROM users ORDER BY id LIMIT $1 OFFSET $2`,
		perPage, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, pq.Array(&u.Roles), &u.EmailVerifiedAt, &u.FailedLoginAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// Sessions

func CreateSession(ctx context.Context, db *sql.DB, id string, userID int64, expiresAt time.Time) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, $3)`,
		id, userID, expiresAt,
	)
	return err
}

func GetSession(ctx context.Context, db *sql.DB, id string) (*models.Session, error) {
	s := &models.Session{}
	err := db.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at, created_at FROM sessions WHERE id = $1 AND expires_at > now()`,
		id,
	).Scan(&s.ID, &s.UserID, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func DeleteSession(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	return err
}

func DeleteExpiredSessions(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	return err
}

func DeleteExpiredPasswordResets(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM password_resets WHERE expires_at < now()`)
	return err
}

// API Keys

func CreateAPIKey(ctx context.Context, db *sql.DB, userID int64, keyHash, prefix, name string, expiresAt *time.Time) (*models.APIKey, error) {
	k := &models.APIKey{}
	err := db.QueryRowContext(ctx,
		`INSERT INTO api_keys (user_id, key_hash, prefix, name, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id, key_hash, prefix, name, last_used, created_at, expires_at`,
		userID, keyHash, prefix, name, expiresAt,
	).Scan(&k.ID, &k.UserID, &k.KeyHash, &k.Prefix, &k.Name, &k.LastUsed, &k.CreatedAt, &k.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func ListAPIKeysByUser(ctx context.Context, db *sql.DB, userID int64) ([]models.APIKey, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, user_id, prefix, name, last_used, created_at, expires_at
		 FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Prefix, &k.Name, &k.LastUsed, &k.CreatedAt, &k.ExpiresAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func GetAPIKeysByPrefix(ctx context.Context, db *sql.DB, prefix string) ([]models.APIKey, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, user_id, key_hash, prefix, name, last_used, created_at, expires_at
		 FROM api_keys WHERE prefix = $1 AND (expires_at IS NULL OR expires_at > now())`,
		prefix,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.KeyHash, &k.Prefix, &k.Name, &k.LastUsed, &k.CreatedAt, &k.ExpiresAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func UpdateAPIKeyLastUsed(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, `UPDATE api_keys SET last_used = now() WHERE id = $1`, id)
	return err
}

func DeleteAPIKey(ctx context.Context, db *sql.DB, id, userID int64) error {
	result, err := db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Password Resets

func CreatePasswordReset(ctx context.Context, db *sql.DB, userID int64, tokenHash string, expiresAt time.Time) error {
	// Delete any existing resets for this user first
	_, _ = db.ExecContext(ctx, `DELETE FROM password_resets WHERE user_id = $1`, userID)
	_, err := db.ExecContext(ctx,
		`INSERT INTO password_resets (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

func GetPasswordResetByTokenHash(ctx context.Context, db *sql.DB, tokenHash string) (*models.PasswordReset, error) {
	r := &models.PasswordReset{}
	err := db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, expires_at, created_at
		 FROM password_resets WHERE token_hash = $1 AND expires_at > now()`,
		tokenHash,
	).Scan(&r.ID, &r.UserID, &r.TokenHash, &r.ExpiresAt, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func DeletePasswordReset(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM password_resets WHERE id = $1`, id)
	return err
}

func UpdateUserPassword(ctx context.Context, db *sql.DB, userID int64, passwordHash string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`,
		passwordHash, userID,
	)
	return err
}

// Email Verifications

func CreateEmailVerification(ctx context.Context, db *sql.DB, userID int64, tokenHash string, expiresAt time.Time) error {
	// Wipe any existing pending verification so the most recent link is the only valid one.
	_, _ = db.ExecContext(ctx, `DELETE FROM email_verifications WHERE user_id = $1`, userID)
	_, err := db.ExecContext(ctx,
		`INSERT INTO email_verifications (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

func GetEmailVerificationByTokenHash(ctx context.Context, db *sql.DB, tokenHash string) (*models.EmailVerification, error) {
	v := &models.EmailVerification{}
	err := db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, expires_at, created_at
		 FROM email_verifications WHERE token_hash = $1 AND expires_at > now()`,
		tokenHash,
	).Scan(&v.ID, &v.UserID, &v.TokenHash, &v.ExpiresAt, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func DeleteEmailVerification(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM email_verifications WHERE id = $1`, id)
	return err
}

func MarkUserEmailVerified(ctx context.Context, db *sql.DB, userID int64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE users SET email_verified_at = now(), updated_at = now() WHERE id = $1 AND email_verified_at IS NULL`,
		userID,
	)
	return err
}

func DeleteExpiredEmailVerifications(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM email_verifications WHERE expires_at < now()`)
	return err
}

// Account lockout

// AuthLockoutThreshold is the number of consecutive failed logins that triggers a lock.
// AuthLockoutDuration is how long the account stays locked.
const (
	AuthLockoutThreshold = 5
	AuthLockoutDuration  = 15 * time.Minute
)

// RecordFailedLogin increments the user's failure counter and locks the
// account once the threshold is hit. Returns whether the account is now
// locked and the new attempt count.
func RecordFailedLogin(ctx context.Context, db *sql.DB, userID int64) (locked bool, attempts int, err error) {
	err = db.QueryRowContext(ctx,
		`UPDATE users
		   SET failed_login_attempts = failed_login_attempts + 1,
		       locked_until = CASE
		         WHEN failed_login_attempts + 1 >= $1 THEN now() + $2::interval
		         ELSE locked_until
		       END
		 WHERE id = $3
		 RETURNING failed_login_attempts, locked_until > now()`,
		AuthLockoutThreshold,
		fmt.Sprintf("%d seconds", int(AuthLockoutDuration.Seconds())),
		userID,
	).Scan(&attempts, &locked)
	return locked, attempts, err
}

// ResetFailedLogins clears the lockout state for a user. Call after a successful login.
func ResetFailedLogins(ctx context.Context, db *sql.DB, userID int64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE users SET failed_login_attempts = 0, locked_until = NULL WHERE id = $1`,
		userID,
	)
	return err
}
