CREATE TABLE IF NOT EXISTS notes (
  id          TEXT PRIMARY KEY,
  body        TEXT NOT NULL,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  deleted_at  TEXT
);

CREATE TABLE IF NOT EXISTS tags (
  name        TEXT PRIMARY KEY,
  created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS note_tags (
  note_id     TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  tag_name    TEXT NOT NULL REFERENCES tags(name),
  PRIMARY KEY (note_id, tag_name)
);

CREATE INDEX IF NOT EXISTS idx_notes_created_at ON notes(created_at);
CREATE INDEX IF NOT EXISTS idx_note_tags_tag    ON note_tags(tag_name);
