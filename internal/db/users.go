package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/dstathis/openswiss/internal/models"
	"github.com/lib/pq"
)

func CreateUser(ctx context.Context, db *sql.DB, email, displayName, passwordHash string) (*models.User, error) {
	u := &models.User{}
	err := db.QueryRowContext(ctx,
		`INSERT INTO users (email, display_name, password_hash) VALUES ($1, $2, $3)
		 RETURNING id, email, display_name, password_hash, roles, created_at, updated_at`,
		email, displayName, passwordHash,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, pq.Array(&u.Roles), &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func GetUserByEmail(ctx context.Context, db *sql.DB, email string) (*models.User, error) {
	u := &models.User{}
	err := db.QueryRowContext(ctx,
		`SELECT id, email, display_name, password_hash, roles, created_at, updated_at FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, pq.Array(&u.Roles), &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func GetUserByID(ctx context.Context, db *sql.DB, id int64) (*models.User, error) {
	u := &models.User{}
	err := db.QueryRowContext(ctx,
		`SELECT id, email, display_name, password_hash, roles, created_at, updated_at FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, pq.Array(&u.Roles), &u.CreatedAt, &u.UpdatedAt)
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
		`SELECT id, email, display_name, password_hash, roles, created_at, updated_at
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
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, pq.Array(&u.Roles), &u.CreatedAt, &u.UpdatedAt); err != nil {
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
