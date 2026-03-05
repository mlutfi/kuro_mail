-- Migration: 002_create_sessions.up.sql
-- Tabel sesi autentikasi dan device management

CREATE TABLE sessions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Token identifiers (actual tokens disimpan di Redis)
    access_jti      VARCHAR(128) NOT NULL UNIQUE,   -- JWT ID for access token
    refresh_jti     VARCHAR(128) NOT NULL UNIQUE,   -- JWT ID for refresh token

    -- Device info
    device_id       VARCHAR(255),
    device_name     VARCHAR(255),                   -- "Chrome on Windows", "Safari on iPhone"
    device_type     VARCHAR(50),                    -- browser | mobile | desktop
    user_agent      TEXT,
    ip_address      INET,
    country_code    VARCHAR(5),

    -- Status
    is_trusted      BOOLEAN NOT NULL DEFAULT FALSE, -- Trusted device: skip 2FA
    is_revoked      BOOLEAN NOT NULL DEFAULT FALSE,

    -- 2FA state saat proses login berlangsung
    two_fa_pending  BOOLEAN NOT NULL DEFAULT FALSE,
    two_fa_temp_token VARCHAR(255),                 -- short-lived token step-1 auth

    -- Timestamps
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_active_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ
);

CREATE INDEX idx_sessions_user_id      ON sessions(user_id);
CREATE INDEX idx_sessions_access_jti   ON sessions(access_jti);
CREATE INDEX idx_sessions_refresh_jti  ON sessions(refresh_jti);
CREATE INDEX idx_sessions_expires_at   ON sessions(expires_at);
CREATE INDEX idx_sessions_is_revoked   ON sessions(is_revoked) WHERE NOT is_revoked;
