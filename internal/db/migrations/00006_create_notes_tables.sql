-- Zettelkasten Notes Schema Migration
-- Migration: Create notes and note_links tables for zettelkasten functionality
-- Sequence: 00006 (based on existing migrations in database.go runMigrations function)

-- Notes table: stores individual zettelkasten notes
CREATE TABLE IF NOT EXISTS notes (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    file_path TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    tags TEXT
);

-- Note links table: stores bidirectional links between notes
CREATE TABLE IF NOT EXISTS note_links (
    id TEXT PRIMARY KEY,
    source_note_id TEXT NOT NULL,
    target_slug TEXT NOT NULL,
    target_note_id TEXT,
    link_type TEXT NOT NULL DEFAULT 'reference',
    created_at INTEGER NOT NULL,
    FOREIGN KEY (source_note_id) REFERENCES notes(id) ON DELETE CASCADE,
    FOREIGN KEY (target_note_id) REFERENCES notes(id) ON DELETE SET NULL,
    UNIQUE(source_note_id, target_slug, link_type)
);

-- Indices for faster queries
CREATE INDEX IF NOT EXISTS idx_notes_slug ON notes(slug);
CREATE INDEX IF NOT EXISTS idx_notes_created ON notes(created_at);
CREATE INDEX IF NOT EXISTS idx_notes_updated ON notes(updated_at);
CREATE INDEX IF NOT EXISTS idx_note_links_source ON note_links(source_note_id);
CREATE INDEX IF NOT EXISTS idx_note_links_target ON note_links(target_note_id);
CREATE INDEX IF NOT EXISTS idx_note_links_target_slug ON note_links(target_slug);
