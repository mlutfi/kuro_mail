-- Migration: 003_create_contacts.up.sql
-- Buku alamat / kontak pengguna

CREATE TABLE contacts (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    email           VARCHAR(255) NOT NULL,
    display_name    VARCHAR(255) NOT NULL DEFAULT '',
    phone           VARCHAR(50),
    company         VARCHAR(255),
    notes           TEXT,
    avatar_url      TEXT,

    -- Metadata penggunaan untuk autocomplete ranking
    email_count     INTEGER NOT NULL DEFAULT 0,    -- Berapa kali email dikirim ke kontak ini
    last_emailed_at TIMESTAMPTZ,

    -- Source
    source          VARCHAR(50) NOT NULL DEFAULT 'manual', -- manual | imported | auto (dari sent)

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_contacts_user_email ON contacts(user_id, email);
CREATE INDEX idx_contacts_user_id          ON contacts(user_id);
CREATE INDEX idx_contacts_email_count      ON contacts(user_id, email_count DESC);

CREATE TRIGGER set_contacts_updated_at
    BEFORE UPDATE ON contacts
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();
