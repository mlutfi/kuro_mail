-- Migration: 005_create_email_metadata.up.sql
-- Cache metadata email dari IMAP/JMAP di PostgreSQL
-- Ini bukan storage email penuh — hanya metadata untuk search, thread, labels

CREATE TABLE email_metadata (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- IMAP/JMAP identifiers
    imap_uid        BIGINT,                     -- IMAP UID
    imap_folder     VARCHAR(255) NOT NULL,      -- Folder IMAP
    message_id      VARCHAR(1024),              -- Message-ID header (untuk threading)
    jmap_id         VARCHAR(255),               -- JMAP Email ID jika ada

    -- Thread info
    thread_id       VARCHAR(255),               -- JMAP Thread ID atau computed hash
    in_reply_to     VARCHAR(1024),              -- In-Reply-To header
    references_ids  TEXT[],                     -- References header dipecah jadi array

    -- Envelope
    subject         TEXT NOT NULL DEFAULT '',
    from_name       VARCHAR(255),
    from_email      VARCHAR(255),
    to_addresses    JSONB DEFAULT '[]'::jsonb,  -- [{name, email}]
    cc_addresses    JSONB DEFAULT '[]'::jsonb,
    bcc_addresses   JSONB DEFAULT '[]'::jsonb,
    reply_to        VARCHAR(255),

    -- Content flags
    has_attachments BOOLEAN NOT NULL DEFAULT FALSE,
    attachment_names TEXT[],
    body_preview    TEXT,                       -- Pertama 500 chars dari body (plain text)
    size_bytes      BIGINT NOT NULL DEFAULT 0,

    -- IMAP Flags
    is_read         BOOLEAN NOT NULL DEFAULT FALSE,
    is_starred      BOOLEAN NOT NULL DEFAULT FALSE,
    is_draft        BOOLEAN NOT NULL DEFAULT FALSE,
    is_deleted      BOOLEAN NOT NULL DEFAULT FALSE,
    imap_flags      TEXT[],                     -- Raw IMAP flags

    -- Custom labels (many-to-many via label_id array)
    label_ids       UUID[],

    -- Dates
    sent_at         TIMESTAMPTZ,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_meta_user_folder     ON email_metadata(user_id, imap_folder);
CREATE INDEX idx_email_meta_thread          ON email_metadata(user_id, thread_id);
CREATE INDEX idx_email_meta_message_id      ON email_metadata(message_id) WHERE message_id IS NOT NULL;
CREATE INDEX idx_email_meta_received_at     ON email_metadata(user_id, received_at DESC);
CREATE INDEX idx_email_meta_is_read         ON email_metadata(user_id, is_read) WHERE NOT is_read;
CREATE INDEX idx_email_meta_is_starred      ON email_metadata(user_id, is_starred) WHERE is_starred;
CREATE INDEX idx_email_meta_from_email      ON email_metadata(user_id, from_email);
CREATE UNIQUE INDEX idx_email_meta_uid      ON email_metadata(user_id, imap_folder, imap_uid);

-- Full text search index
CREATE INDEX idx_email_meta_fts ON email_metadata
    USING gin(to_tsvector('indonesian', coalesce(subject, '') || ' ' || coalesce(from_email, '') || ' ' || coalesce(body_preview, '')));

CREATE TRIGGER set_email_metadata_updated_at
    BEFORE UPDATE ON email_metadata
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();
