-- Migration: 004_create_labels.up.sql
-- Label / tag kustom (seperti label Gmail)

CREATE TABLE labels (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL,
    color       VARCHAR(20) NOT NULL DEFAULT '#6B7280',  -- hex color
    icon        VARCHAR(50),
    is_system   BOOLEAN NOT NULL DEFAULT FALSE,          -- label sistem: inbox, sent, draft, dll
    imap_folder VARCHAR(255),                            -- mapping ke IMAP folder jika ada

    position    INTEGER NOT NULL DEFAULT 0,              -- urutan tampil di sidebar
    is_hidden   BOOLEAN NOT NULL DEFAULT FALSE,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_labels_user_name ON labels(user_id, name);
CREATE INDEX idx_labels_user_id          ON labels(user_id);

CREATE TRIGGER set_labels_updated_at
    BEFORE UPDATE ON labels
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- Seed default system labels untuk setiap user baru (akan dipanggil via application logic)
-- Tapi kita buat stored procedure helper-nya di sini
CREATE OR REPLACE FUNCTION create_default_labels(p_user_id UUID)
RETURNS VOID AS $$
BEGIN
    INSERT INTO labels (user_id, name, color, is_system, imap_folder, position) VALUES
        (p_user_id, 'INBOX',   '#1A56DB', TRUE, 'INBOX',         1),
        (p_user_id, 'Sent',    '#0E9F8E', TRUE, 'Sent',          2),
        (p_user_id, 'Drafts',  '#F59E0B', TRUE, 'Drafts',        3),
        (p_user_id, 'Trash',   '#EF4444', TRUE, 'Trash',         4),
        (p_user_id, 'Spam',    '#DC2626', TRUE, 'Junk',          5),
        (p_user_id, 'Archive', '#6B7280', TRUE, 'Archive',       6),
        (p_user_id, 'Starred', '#F59E0B', TRUE, NULL,            7)
    ON CONFLICT (user_id, name) DO NOTHING;
END;
$$ LANGUAGE plpgsql;
