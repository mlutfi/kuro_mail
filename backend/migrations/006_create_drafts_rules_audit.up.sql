-- Migration: 006_create_drafts_rules_audit.up.sql

-- -------------------------------------------------------
-- DRAFTS
-- Simpan draft email yang belum terkirim
-- -------------------------------------------------------
CREATE TABLE drafts (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Bisa link ke thread yang sedang di-reply
    thread_id       VARCHAR(255),
    reply_to_uid    BIGINT,
    is_forward      BOOLEAN NOT NULL DEFAULT FALSE,

    -- Compose fields
    to_addresses    JSONB NOT NULL DEFAULT '[]'::jsonb,
    cc_addresses    JSONB NOT NULL DEFAULT '[]'::jsonb,
    bcc_addresses   JSONB NOT NULL DEFAULT '[]'::jsonb,
    subject         TEXT NOT NULL DEFAULT '',
    body_html       TEXT NOT NULL DEFAULT '',
    body_text       TEXT NOT NULL DEFAULT '',

    -- Attachments (metadata saja, file di object storage)
    attachments     JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- [{ id, filename, size, mime_type, storage_path }]

    -- Schedule
    send_at         TIMESTAMPTZ,               -- null = not scheduled

    -- Auto-save tracking
    last_saved_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    imap_uid        BIGINT,                    -- UID di folder Drafts IMAP setelah sync

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_drafts_user_id    ON drafts(user_id);
CREATE INDEX idx_drafts_send_at    ON drafts(send_at) WHERE send_at IS NOT NULL;

CREATE TRIGGER set_drafts_updated_at
    BEFORE UPDATE ON drafts
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();


-- -------------------------------------------------------
-- EMAIL FILTER RULES
-- Rules untuk auto-label, auto-forward, dll (via ManageSieve)
-- -------------------------------------------------------
CREATE TABLE email_rules (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    position        INTEGER NOT NULL DEFAULT 0,

    -- Conditions (JSON): [{field: "from"|"subject"|"to", op: "contains"|"equals"|"startsWith", value: "..."}]
    conditions      JSONB NOT NULL DEFAULT '[]'::jsonb,
    condition_mode  VARCHAR(5) NOT NULL DEFAULT 'AND',   -- AND | OR

    -- Actions (JSON): [{action: "label"|"move"|"forward"|"delete"|"read"|"star", value: "..."}]
    actions         JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- Sieve script hasil generate (sync ke Stalwart via ManageSieve)
    sieve_script    TEXT,
    sieve_synced_at TIMESTAMPTZ,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_rules_user_id  ON email_rules(user_id);
CREATE INDEX idx_email_rules_active   ON email_rules(user_id, is_active) WHERE is_active;

CREATE TRIGGER set_email_rules_updated_at
    BEFORE UPDATE ON email_rules
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();


-- -------------------------------------------------------
-- AUDIT LOG
-- Log semua event keamanan penting
-- -------------------------------------------------------
CREATE TABLE audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    event_type  VARCHAR(100) NOT NULL,
    -- Tipe event: login_success | login_fail | logout | 2fa_enabled | 2fa_disabled |
    --             2fa_success | 2fa_fail | password_change | session_revoked |
    --             email_sent | email_deleted | rule_created

    -- Detail event
    details     JSONB,
    ip_address  INET,
    user_agent  TEXT,
    status      VARCHAR(20) NOT NULL DEFAULT 'success',  -- success | failure

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_user_id    ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_event_type ON audit_logs(event_type);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC);
CREATE INDEX idx_audit_logs_ip         ON audit_logs(ip_address);

-- Auto-delete audit logs older than 1 year (via pg_cron or application job)
-- Tabel ini partitioned by month untuk performa di production besar


-- -------------------------------------------------------
-- SNOOZE
-- Email yang di-snooze untuk muncul kembali nanti
-- -------------------------------------------------------
CREATE TABLE email_snoozes (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    imap_uid        BIGINT NOT NULL,
    imap_folder     VARCHAR(255) NOT NULL,
    message_id      VARCHAR(1024),
    wake_at         TIMESTAMPTZ NOT NULL,
    is_done         BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_snoozes_wake_at ON email_snoozes(wake_at) WHERE NOT is_done;
CREATE INDEX idx_snoozes_user_id ON email_snoozes(user_id);
