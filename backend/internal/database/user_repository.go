package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/webmail/backend/internal/models"
)

type PostgresUserRepository struct {
	db *DB
}

func NewUserRepository(db *DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) Create(ctx context.Context, user *models.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	query := `
		INSERT INTO users (
			id, email, display_name, avatar_url, password_hash, is_active, is_admin,
			totp_enabled, backup_codes, timezone, language, theme, emails_per_page,
			signature, reply_style, imap_host, imap_port, imap_use_tls, smtp_host, smtp_port,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, '[]'::jsonb, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, NOW(), NOW()
		)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		user.ID, user.Email, user.DisplayName, user.AvatarURL, user.PasswordHash, user.IsActive, user.IsAdmin,
		user.TOTPEnabled, user.Timezone, user.Language, user.Theme, user.EmailsPerPage,
		user.Signature, user.ReplyStyle, user.IMAPHost, user.IMAPPort, user.IMAPUseTLS, user.SMTPHost, user.SMTPPort,
	)
	return err
}

func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, display_name, avatar_url, password_hash, is_active, is_admin,
		       totp_secret, totp_enabled, totp_verified_at, backup_codes,
		       timezone, language, theme, emails_per_page, signature, reply_style,
		       imap_host, imap_port, imap_use_tls, smtp_host, smtp_port,
		       created_at, updated_at, last_login_at
		FROM users
		WHERE email = $1 AND deleted_at IS NULL
	`

	var u models.User
	var backupCodes []string

	err := r.db.Pool.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.PasswordHash, &u.IsActive, &u.IsAdmin,
		&u.TOTPSecret, &u.TOTPEnabled, &u.TOTPVerifiedAt, &backupCodes,
		&u.Timezone, &u.Language, &u.Theme, &u.EmailsPerPage, &u.Signature, &u.ReplyStyle,
		&u.IMAPHost, &u.IMAPPort, &u.IMAPUseTLS, &u.SMTPHost, &u.SMTPPort,
		&u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}
	u.BackupCodes = backupCodes

	return &u, nil
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, email, display_name, avatar_url, password_hash, is_active, is_admin,
		       totp_secret, totp_enabled, totp_verified_at, backup_codes,
		       timezone, language, theme, emails_per_page, signature, reply_style,
		       imap_host, imap_port, imap_use_tls, smtp_host, smtp_port,
		       created_at, updated_at, last_login_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`

	var u models.User
	var backupCodes []string

	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.PasswordHash, &u.IsActive, &u.IsAdmin,
		&u.TOTPSecret, &u.TOTPEnabled, &u.TOTPVerifiedAt, &backupCodes,
		&u.Timezone, &u.Language, &u.Theme, &u.EmailsPerPage, &u.Signature, &u.ReplyStyle,
		&u.IMAPHost, &u.IMAPPort, &u.IMAPUseTLS, &u.SMTPHost, &u.SMTPPort,
		&u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}
	u.BackupCodes = backupCodes

	return &u, nil
}

func (r *PostgresUserRepository) UpdateTOTPSecret(ctx context.Context, userID uuid.UUID, secret string) error {
	query := `UPDATE users SET totp_secret = $1 WHERE id = $2`
	_, err := r.db.Pool.Exec(ctx, query, secret, userID)
	return err
}

func (r *PostgresUserRepository) EnableTOTP(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE users SET totp_enabled = true, totp_verified_at = NOW() WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, userID)
	return err
}

func (r *PostgresUserRepository) DisableTOTP(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE users SET totp_enabled = false, totp_secret = NULL, backup_codes = '[]'::jsonb WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, userID)
	return err
}

func (r *PostgresUserRepository) UpdateBackupCodes(ctx context.Context, userID uuid.UUID, codes []string) error {
	jsonCodes, err := json.Marshal(codes)
	if err != nil {
		return fmt.Errorf("failed to marshal backup codes: %w", err)
	}

	query := `UPDATE users SET backup_codes = $1 WHERE id = $2`
	_, err = r.db.Pool.Exec(ctx, query, jsonCodes, userID)
	return err
}

func (r *PostgresUserRepository) UpdateLastLogin(ctx context.Context, userID uuid.UUID, ip string) error {
	query := `UPDATE users SET last_login_at = NOW(), last_login_ip = $1 WHERE id = $2`
	_, err := r.db.Pool.Exec(ctx, query, ip, userID)
	return err
}

func (r *PostgresUserRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, hash string) error {
	query := `UPDATE users SET password_hash = $1 WHERE id = $2`
	_, err := r.db.Pool.Exec(ctx, query, hash, userID)
	return err
}
