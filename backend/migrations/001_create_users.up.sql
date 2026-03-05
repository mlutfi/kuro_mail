-- Migration: 001_create_users.up.sql
-- Tabel utama pengguna webmail

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email           VARCHAR(255) NOT NULL UNIQUE,
    display_name    VARCHAR(255) NOT NULL DEFAULT '',
    avatar_url      TEXT,

    -- Auth
    password_hash   TEXT,                        -- nullable jika hanya OAuth
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    is_admin        BOOLEAN NOT NULL DEFAULT FALSE,

    -- 2FA
    totp_secret     TEXT,                        -- encrypted AES-256-GCM
    totp_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
    totp_verified_at TIMESTAMPTZ,

    -- Backup codes untuk recovery 2FA (JSON array of hashed codes)
    backup_codes    JSONB DEFAULT '[]'::jsonb,

    -- Preferences
    timezone        VARCHAR(100) NOT NULL DEFAULT 'Asia/Jakarta',
    language        VARCHAR(10)  NOT NULL DEFAULT 'id',
    theme           VARCHAR(20)  NOT NULL DEFAULT 'light',  -- light | dark | system
    emails_per_page INTEGER      NOT NULL DEFAULT 50,
    signature       TEXT         NOT NULL DEFAULT '',
    reply_style     VARCHAR(20)  NOT NULL DEFAULT 'reply_all', -- reply | reply_all

    -- Stalwart IMAP config (per user jika multi-domain)
    imap_host       VARCHAR(255),
    imap_port       INTEGER      NOT NULL DEFAULT 993,
    imap_use_tls    BOOLEAN      NOT NULL DEFAULT TRUE,
    smtp_host       VARCHAR(255),
    smtp_port       INTEGER      NOT NULL DEFAULT 587,

    -- Timestamps
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_login_at   TIMESTAMPTZ,
    last_login_ip   INET,
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX idx_users_email        ON users(email);
CREATE INDEX idx_users_is_active    ON users(is_active);
CREATE INDEX idx_users_deleted_at   ON users(deleted_at) WHERE deleted_at IS NULL;

-- Auto-update updated_at trigger
CREATE OR REPLACE FUNCTION trigger_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER set_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();
