# Service Package

This package provides business logic services for Reckon's notes and wiki links functionality.

## Notes Service

The `NotesService` handles all operations related to zettelkasten notes and their wiki-style links.

### Key Methods

#### UpdateNoteLinks

Extracts wiki links from a note's markdown content and syncs them to the database.

```go
// After creating or updating a note
err := notesService.UpdateNoteLinks(note)
```

**What it does:**
1. Reads the note's markdown file
2. Extracts all wiki-style links (`[[slug]]` or `[[slug|text]]`)
3. Deletes old wiki links for this note
4. Creates new link records
5. Resolves target_note_id when the target note exists
6. Commits everything in a transaction

**Use cases:**
- After creating a new note
- After editing a note's content
- During bulk note imports/migrations

#### ResolveOrphanedBacklinks

Connects orphaned links to newly created notes.

```go
// After creating a new note
err := notesService.ResolveOrphanedBacklinks(note)
```

**What it does:**
1. Finds all links with NULL `target_note_id` that point to this note's slug
2. Updates those links to set `target_note_id` to this note's ID

**Use cases:**
- After creating a new note that other notes already link to
- During system initialization/repair operations

### Complete Example

```go
package main

import (
    "github.com/MikeBiancalana/reckon/internal/models"
    "github.com/MikeBiancalana/reckon/internal/service"
    "github.com/MikeBiancalana/reckon/internal/storage"
)

func main() {
    // Setup
    db, _ := storage.NewDatabase("reckon.db")
    repo := service.NewNotesRepository(db)
    notesService := service.NewNotesService(repo)

    // Create a note
    note := models.NewNote(
        "Project Planning",
        "project-planning",
        "/path/to/notes/project-planning.md",
        []string{"project", "planning"},
    )

    // Save to database
    err := notesService.SaveNote(note)
    if err != nil {
        panic(err)
    }

    // Extract and save wiki links
    err = notesService.UpdateNoteLinks(note)
    if err != nil {
        panic(err)
    }

    // Resolve any orphaned backlinks
    err = notesService.ResolveOrphanedBacklinks(note)
    if err != nil {
        panic(err)
    }
}
```

## Database Schema

### notes table

Stores individual zettelkasten notes.

```sql
CREATE TABLE notes (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    file_path TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    tags TEXT
);
```

### note_links table

Stores bidirectional links between notes.

```sql
CREATE TABLE note_links (
    id TEXT PRIMARY KEY,
    source_note_id TEXT NOT NULL,
    target_slug TEXT NOT NULL,
    target_note_id TEXT,  -- NULL if target doesn't exist yet
    link_type TEXT NOT NULL DEFAULT 'reference',
    created_at INTEGER NOT NULL,
    FOREIGN KEY (source_note_id) REFERENCES notes(id) ON DELETE CASCADE,
    FOREIGN KEY (target_note_id) REFERENCES notes(id) ON DELETE SET NULL,
    UNIQUE(source_note_id, target_slug, link_type)
);
```

## Architecture

### Transaction Safety

All link operations are wrapped in database transactions to ensure consistency:
- Old links are deleted
- New links are inserted
- Everything commits together or rolls back

### Orphaned Links

When you create a link to a note that doesn't exist yet:
- The link is created with `target_slug` set but `target_note_id` NULL
- When the target note is later created, call `ResolveOrphanedBacklinks()`
- The link is updated to point to the actual note ID

This allows for:
- Creating notes in any order
- Forward references (linking to notes you haven't created yet)
- Gradual note creation without breaking links

## Testing

Run tests:

```bash
go test ./internal/service/
```

Run with coverage:

```bash
go test -cover ./internal/service/
```

## Integration Checklist

When integrating wiki links into a notes workflow:

1. Create or update note file on disk
2. Save note metadata: `notesService.SaveNote(note)`
3. Update links: `notesService.UpdateNoteLinks(note)`
4. Resolve backlinks: `notesService.ResolveOrphanedBacklinks(note)`
5. Handle errors appropriately

All operations should be called in this order for complete link synchronization.
