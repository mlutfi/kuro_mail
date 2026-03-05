package database

import (
	"context"

	"github.com/google/uuid"

	"github.com/webmail/backend/internal/models"
)

type PostgresSessionRepository struct {
	db *DB
}

func NewSessionRepository(db *DB) *PostgresSessionRepository {
	return &PostgresSessionRepository{db: db}
}

func (r *PostgresSessionRepository) Create(ctx context.Context, session *models.Session) error {
	query := `
		INSERT INTO sessions (
			id, user_id, access_jti, refresh_jti, device_id, device_name,
			device_type, user_agent, ip_address, is_trusted, is_revoked,
			created_at, last_active_at, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)
	`
	_, err := r.db.Pool.Exec(ctx, query,
		session.ID, session.UserID, session.AccessJTI, session.RefreshJTI,
		session.DeviceID, session.DeviceName, session.DeviceType,
		session.UserAgent, session.IPAddress, session.IsTrusted,
		session.IsRevoked, session.CreatedAt, session.LastActiveAt,
		session.ExpiresAt,
	)
	return err
}

func (r *PostgresSessionRepository) GetByAccessJTI(ctx context.Context, jti string) (*models.Session, error) {
	query := `
		SELECT id, user_id, access_jti, refresh_jti, device_id, device_name,
		       device_type, is_trusted, is_revoked, created_at, last_active_at, expires_at
		FROM sessions
		WHERE access_jti = $1
	`
	var s models.Session
	err := r.db.Pool.QueryRow(ctx, query, jti).Scan(
		&s.ID, &s.UserID, &s.AccessJTI, &s.RefreshJTI, &s.DeviceID, &s.DeviceName,
		&s.DeviceType, &s.IsTrusted, &s.IsRevoked, &s.CreatedAt, &s.LastActiveAt, &s.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *PostgresSessionRepository) GetByRefreshJTI(ctx context.Context, jti string) (*models.Session, error) {
	query := `
		SELECT id, user_id, access_jti, refresh_jti, device_id, device_name,
		       device_type, is_trusted, is_revoked, created_at, last_active_at, expires_at
		FROM sessions
		WHERE refresh_jti = $1
	`
	var s models.Session
	err := r.db.Pool.QueryRow(ctx, query, jti).Scan(
		&s.ID, &s.UserID, &s.AccessJTI, &s.RefreshJTI, &s.DeviceID, &s.DeviceName,
		&s.DeviceType, &s.IsTrusted, &s.IsRevoked, &s.CreatedAt, &s.LastActiveAt, &s.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *PostgresSessionRepository) RevokeByID(ctx context.Context, sessionID uuid.UUID) error {
	query := `UPDATE sessions SET is_revoked = true, revoked_at = NOW() WHERE id = $1`
	_, err := r.db.Pool.Exec(ctx, query, sessionID)
	return err
}

func (r *PostgresSessionRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE sessions SET is_revoked = true, revoked_at = NOW() WHERE user_id = $1 AND is_revoked = false`
	_, err := r.db.Pool.Exec(ctx, query, userID)
	return err
}

func (r *PostgresSessionRepository) GetActiveSessions(ctx context.Context, userID uuid.UUID) ([]*models.Session, error) {
	query := `
		SELECT id, user_id, access_jti, refresh_jti, device_id, device_name,
		       device_type, is_trusted, is_revoked, created_at, last_active_at, expires_at
		FROM sessions
		WHERE user_id = $1 AND is_revoked = false
		ORDER BY last_active_at DESC
	`
	rows, err := r.db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*models.Session
	for rows.Next() {
		var s models.Session
		err := rows.Scan(
			&s.ID, &s.UserID, &s.AccessJTI, &s.RefreshJTI, &s.DeviceID, &s.DeviceName,
			&s.DeviceType, &s.IsTrusted, &s.IsRevoked, &s.CreatedAt, &s.LastActiveAt, &s.ExpiresAt,
		)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, &s)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return sessions, nil
}

func (r *PostgresSessionRepository) UpdateLastActive(ctx context.Context, jti string) error {
	query := `UPDATE sessions SET last_active_at = NOW() WHERE access_jti = $1 OR refresh_jti = $1`
	_, err := r.db.Pool.Exec(ctx, query, jti)
	return err
}
