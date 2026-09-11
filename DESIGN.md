# jot — Technical Design Document

Status: draft v1. Companion to the [product plan](.) (requirements, FR-N/NFR-N ids
referenced throughout this doc live there). This document is the implementation
reference — where the product plan left something open, a decision is made here;
where something is still genuinely undecided, it's called out in §9 rather than
left ambiguous.

## 0. Non-negotiables carried from the product plan

- Local-only, zero network calls, zero telemetry (NFR-2).
- Single static binary, no runtime to install (NFR-7).
- Sub-50ms capture-to-exit (NFR-1).
- WAL-mode SQLite, transactional writes (NFR-3, NFR-4).
- `~/.jot` `0700`, `data.sqlite` `0600` (NFR-5).
- UUIDv7 ids; tags keyed on normalized name, not a synthetic id (schema section
  of the product plan).

## 1. Toolchain & dependencies

| Concern | Choice | Why |
|---|---|---|
| Language | Go 1.23+ | Already decided in the product plan |
| SQLite driver | `modernc.org/sqlite` | Pure Go, no cgo. This is the load-bearing choice for NFR-7: a cgo driver (`mattn/go-sqlite3`) needs a C toolchain per target OS/arch to cross-compile, which turns "single static binary" into a real CI problem. `modernc.org/sqlite` also has FTS5 compiled in, which the product plan already flagged as a requirement for whichever driver got picked, ahead of the FTS5 fast-follow. |
| CLI framework | `spf13/cobra` (+ its bundled `pflag`) | Gives short/long flag pairs, `--help`/usage generation, and a subcommand tree for free. **Caveat:** cobra's own guess at "is this a subcommand or a positional arg" is not trusted for top-level dispatch — see §4.1. Cobra owns everything *after* that dispatch decision is made. |
| UUIDs | `google/uuid` v1.6.0+, `uuid.NewV7()` | Standard, maintained, has v7 support. |
| TOML | `BurntSushi/toml` | Simple decode API, extremely well-established, no reason to reach for anything fancier for a handful of config keys. |
| TTY detection | `golang.org/x/term` | Not anticipated when this doc was first drafted — needed by `rm --hard`'s confirmation flow (§4.5) to tell an interactive terminal from a script/pipe. Standard, minimal, extended-stdlib. |
| Test assertions | stdlib `testing` + `testify/require` | No other framework needed. |

**Module path:** `github.com/softwarelt/jot-cli` — the intended home for this
repo. The binary itself is still invoked as `jot`, unaffected by the module
name: `go build`/`go install` name the executable after the `cmd/jot`
directory, not the module path.

## 2. Repository layout

```
jot-cli/
  cmd/jot/
    main.go              # os.Args dispatch (§4.1), then either runCapture() or cli.Execute()
  internal/
    cli/                 # cobra command tree — one file per subcommand
      root.go
      list.go
      search.go
      show.go
      tags.go
      rm.go
      restore.go
      config.go
    store/                # the ONLY package that imports the sqlite driver
      store.go            # Open(), pragmas, schema application
      schema.sql           # DDL, embedded via go:embed — single source of truth
      notes.go             # CreateNote, List, Search, GetByPrefix, SoftDelete, HardDelete, Restore
      tags.go              # NormalizeTag, ListTags
      errors.go            # ErrAmbiguous, ErrNotFound
    config/
      config.go            # struct + defaults + TOML decode + flag/env/file precedence
    idgen/
      idgen.go              # wraps uuid.NewV7 — isolates the dependency to one file
    output/
      table.go               # human-readable rendering
      json.go                 # --json rendering (§4.4)
  go.mod
  go.sum
  DESIGN.md                    # this file
```

**Why `internal/store` is walled off:** it's the only package that knows SQL exists.
Every other package talks to it through the `Note`/`TagCount` structs and the
methods in §3.4. This is what makes the FTS5 fast-follow (a decision already made
in the product plan) a change contained entirely inside this one package — the
CLI layer, output rendering, and config never need to know search moved from
`LIKE` to a virtual table.

## 3. Data layer

### 3.1 Schema (binding — supersedes the copy in the product plan if they ever diverge)

```sql
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
```

Applied idempotently (`IF NOT EXISTS`) on every `store.Open()`. No migration
framework for v1 — there's exactly one schema version. A real migration tool
is a fast-follow the moment the schema needs to change post-release (§9).

### 3.2 Connection setup

Run once per `store.Open()`, before any other query:

```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
```

- `journal_mode=WAL` — NFR-4 (concurrent writers).
- `busy_timeout=5000` — SQLite's own lock-wait-and-retry instead of hand-rolled
  retry logic around `SQLITE_BUSY`.
- `foreign_keys=ON` — SQLite has this **off** by default per-connection; without
  it, `note_tags`'s `ON DELETE CASCADE` silently does nothing.

### 3.3 Timestamp format (binding, not just descriptive)

Every `created_at` / `updated_at` / `deleted_at` value is produced by exactly one
function:

```go
func nowStamp() string {
    return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}
```

Fixed millisecond precision, always UTC, always this one call site. This is what
makes plain string comparison in `WHERE created_at >= ?` sort and range-filter
correctly — if some rows ever had fractional seconds and others didn't,
lexicographic ordering would silently stop matching chronological ordering.

### 3.4 Store interface

```go
package store

type Note struct {
    ID        string
    Body      string
    CreatedAt time.Time
    UpdatedAt time.Time
    DeletedAt *time.Time // nil unless soft-deleted
    Tags      []string
}

type TagCount struct {
    Name  string
    Count int
}

type DeletedFilter int

const (
    ActiveOnly     DeletedFilter = iota // default everywhere
    IncludeDeleted
    DeletedOnly
)

type ListParams struct {
    Tag     string     // "" = no filter
    Since   *time.Time // nil = no lower bound
    Until   *time.Time // nil = no upper bound
    Limit   int
    Deleted DeletedFilter
}

func Open(path string) (*Store, error)

func (s *Store) CreateNote(ctx context.Context, body string, tags []string) (Note, error)
func (s *Store) List(ctx context.Context, p ListParams) ([]Note, error)
func (s *Store) Search(ctx context.Context, term, tag string, deleted DeletedFilter) ([]Note, error)

// GetByPrefix always searches across ALL notes regardless of deleted_at —
// per FR-9, a specific known id doesn't need a deleted-status filter, so
// there is deliberately no DeletedFilter parameter here.
func (s *Store) GetByPrefix(ctx context.Context, prefix string) (Note, error)

func (s *Store) SoftDelete(ctx context.Context, id string) error
func (s *Store) HardDelete(ctx context.Context, id string) error
func (s *Store) Restore(ctx context.Context, id string) error
func (s *Store) ListTags(ctx context.Context, deleted DeletedFilter) ([]TagCount, error)
```

`GetByPrefix` returns one of:
- the matching `Note`,
- `ErrNotFound` (no match),
- `ErrAmbiguous{Matches []Note}` — every match, **ordered chronologically**
  (trivial: UUIDv7 sorts lexicographically = chronologically, so `ORDER BY id`
  is sufficient), so the CLI layer can render FR-9's "what else was jotted
  around here" list. The store layer never prints anything — rendering is
  `internal/output`'s job.

Minimum prefix length (4 chars, per the product plan) is enforced in the CLI
layer's argument validation, not in the store — the store will happily look up
a 1-character prefix if asked; rejecting short prefixes is a UX rule about
lookup commands (`show`/`rm`/`restore`), not a data-layer invariant.

### 3.5 Tag normalization

```go
// NormalizeTag trims whitespace and lowercases (Unicode-aware). Returns an
// error if the result is empty or contains internal whitespace (FR-3).
func NormalizeTag(raw string) (string, error)
```

Applied uniformly at capture time **and** anywhere a `--tag` filter value is
accepted, so `jot list --tag Bug` matches notes stored under `bug`.

## 4. CLI layer

### 4.1 Top-level dispatch — implements FR-18 exactly

This runs in `cmd/jot/main.go`, **before** cobra ever sees the arguments:

```go
var reserved = map[string]bool{
    "list": true, "search": true, "show": true, "tags": true,
    "rm": true, "restore": true, "config": true,
    "help": true, "version": true,
    "-h": true, "--help": true, "-v": true, "--version": true,
}

func main() {
    args := os.Args[1:]

    switch {
    case len(args) > 0 && args[0] == "--":
        // `--` forces literal capture unconditionally — even over a reserved
        // word or a leading dash. This is the FR-18 escape hatch.
        runCapture(args[1:])

    case len(args) == 0:
        runDefaultList() // bare `jot`

    case reserved[args[0]]:
        cli.Execute(args) // hand off to the cobra tree; cobra owns everything from here

    default:
        runCapture(args) // args[0] = note body, args[1:] = tags
    }
}
```

`runCapture` never touches cobra. This is deliberate: FR-18's guarantee (a
one-word note that says "list" only gets captured via `jot -- list`) must not
depend on cobra's internal positional-arg heuristics, which could change
between versions. Cobra is only ever invoked with an args slice whose first
element is already known to be a reserved word.

**Implementer note:** write a test for each reserved word confirming
`jot <word>` dispatches to the subcommand and `jot -- <word>` captures it
literally, plus one for a leading-dash body (`jot -- "-1 story point"`).

### 4.2 Per-command flags

| Command | Flag | Short | Type | Default | Notes |
|---|---|---|---|---|---|
| `list` | `--tag` | `-t` | string | `""` | normalized before querying |
| | `--since` | `-s` | string | `""` | RFC3339 or `YYYY-MM-DD`; parse error → exit code 2 |
| | `--until` | `-u` | string | `""` | same |
| | `--limit` | `-n` | int | `config.display.default_list_limit` | |
| | `--json` | `-j` | bool | `false` | |
| | `--include-deleted` | — | bool | `false` | mutually exclusive with `--deleted-only` |
| | `--deleted-only` | — | bool | `false` | mutually exclusive with `--include-deleted` |
| `search <term>` | `--tag` | `-t` | string | `""` | |
| | `--json` | `-j` | bool | `false` | |
| | `--include-deleted` / `--deleted-only` | — | bool | `false` | same rule as `list` |
| `show <id>` | — | | | | no flags; always searches all notes regardless of deleted status |
| `tags` | `--json` | `-j` | bool | `false` | |
| | `--include-deleted` / `--deleted-only` | — | bool | `false` | |
| `rm <id>` | `--hard` | — | bool | `false` | long-only by design (friction before permanent delete) |
| | `--yes` | `-y` | bool | `false` | required with `--hard` when stdin is not a TTY |
| `restore <id>` | — | | | | |
| `config` | — | | | | read-only in v1 (§9) |
| global | `--help` | `-h` | | | every command |
| root only | `--version` | `-v` | | | |

Passing both `--include-deleted` and `--deleted-only` on any command → exit
code 2, message names both flags.

### 4.3 Exit codes

Not specified in the product plan; pinned here since scripts calling `jot` will
care:

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | general/internal error (db I/O, unexpected failure) |
| 2 | usage error (bad flags, malformed date, empty note text, malformed tag, mutually-exclusive flags) |
| 3 | ambiguous id prefix — distinct from a general usage error so a script can catch it specifically (e.g. retry `show` with more characters) |

`store.ErrInvalidInput` is the mechanism behind code 2 for anything that
originates inside `internal/store` (an empty note, a malformed tag) rather
than the CLI's own flag parsing: `CreateNote` and the `--tag` filter path
both wrap validation failures with it (`fmt.Errorf("...: %w", ErrInvalidInput)`),
and both `internal/cli`'s `codeForError` and `cmd/jot/main.go`'s capture path
check `errors.Is(err, store.ErrInvalidInput)` before falling back to exit
code 1. Caught during implementation, not designed up front: a first pass
mapped every store error to exit 1, which put "tag must not contain
whitespace" in the same bucket as a database I/O failure.

### 4.4 Output rendering (`internal/output`)

**Human-readable** (`list`/`search`): one row per note — short id, timestamp
rendered in `config.display.timezone`, comma-joined tags, body truncated to
80 characters with `…` if longer. `show` prints the full, untruncated note.

**Short id length is 13, not 8.** The first draft of this document said "first
8 characters, the same convention as a git short hash" — a smoke test caught
that colliding within seconds: three notes captured back-to-back in a row all
printed the same 8-character `Saved <id>`, because (per the data model
section's own UUIDv7 callout) 8 shared hex characters only guarantee
uniqueness across a ~65-second window, not "the last few notes I just typed."
`ShortID` is now the literal first 13 characters of the canonical string —
8 hex, the dash, 4 hex — which is exactly the full UUIDv7 timestamp field
(unique to the millisecond) and, because the dash is kept, is always directly
usable as-is as a `GetByPrefix` argument.

**`--json`** — treated as a versioned contract (flagged in the product plan):
every JSON response is a top-level object carrying `schema_version`, never a
bare array, so a future field can be added without a breaking change.

```json
{
  "schema_version": 1,
  "notes": [
    {
      "id": "018f2c3a-7b1e-7c3a-9d2e-1a2b3c4d5e6f",
      "body": "Alex is blocked on the payments migration",
      "tags": ["alex", "1:1", "blocked"],
      "created_at": "2026-09-10T14:23:01.123Z",
      "updated_at": "2026-09-10T14:23:01.123Z",
      "deleted_at": null
    }
  ]
}
```

`jot tags --json` uses the same envelope with a `"tags"` array of
`{"name": "...", "count": N}` instead of `"notes"`.

### 4.5 `rm --hard` confirmation flow

`jot rm <id>` (soft delete) never prompts — it's reversible, so it just happens.
`jot rm <id> --hard` on an interactive TTY prompts, showing what's about to be
destroyed rather than confirming blind:

```
$ jot rm 018f2c3a --hard
About to permanently delete:
  018f2c3a  "Alex is blocked on the payments migration..."  (alex, 1:1, blocked)

This cannot be undone. Type "yes" to confirm: 
```

The check is an exact, case-insensitive match against `yes` — not `y`/`N` —
consistent with `--hard` already having no short flag: the one truly
destructive action in the tool gets a little extra friction on purpose.
Anything else (including just pressing enter) cancels:

```
This cannot be undone. Type "yes" to confirm: n
Cancelled — no changes made.
```

exiting **0** — declining is a normal outcome, not an error. Non-TTY (`--yes`
required instead) is unchanged from FR-12.

### 4.6 Command help text

Every cobra command has a one-line `Short` (shown in `jot --help`'s command
list) and a longer `Long` (shown by `jot <cmd> --help`). `config` is worth
pinning down explicitly now, since v1 has no `config set` and that needs to be
discoverable without reading this design doc:

```go
var configCmd = &cobra.Command{
    Use:   "config",
    Short: "Show resolved config (edit ~/.jot/config.toml to change it)",
    Long: `Prints the fully resolved configuration jot is currently using,
along with the path of the config file it was loaded from (if any).

jot has no "config set" command. To change a setting, edit that file
directly — it's a plain TOML file — and run any jot command again; changes
take effect immediately, no restart needed.

Resolution order: CLI flag > environment variable > config file > built-in
default. See JOT_HOME / JOT_CONFIG if you keep the file somewhere other than
the default ~/.jot/config.toml.`,
    ...
}
```

Putting the "edit the file directly" pointer in `Short` (not just `Long`) means
it's visible from the top-level `jot --help` listing, not just after already
running `jot config --help`.

## 5. Configuration (`internal/config`)

Resolution order (already decided in the product plan; this is the literal
implementation):

1. CLI flag (per-invocation)
2. Environment variable
3. `~/.jot/config.toml` (or wherever `JOT_HOME`/`JOT_CONFIG` point)
4. Built-in default

```go
type Config struct {
    Storage struct {
        Path string // default: filepath.Join(home, "data.sqlite")
    }
    Display struct {
        Timezone          string // "local" or an IANA name; default "local"
        DefaultListLimit  int    // default 20
    }
}
```

Path resolution:
- `JOT_CONFIG` set → read that file directly.
- else `JOT_HOME` set → read `$JOT_HOME/config.toml` if present.
- else → `~/.jot/config.toml` if present.
- No config file found anywhere → defaults, no error.
- Config file present but fails to parse → hard error naming the file path and
  the TOML parse error verbatim; never silently fall back to defaults.

First-run directory/file creation:
```go
os.MkdirAll(jotHome, 0700)
// data.sqlite created by modernc.org/sqlite on first Open(); chmod explicitly
// after creation since the driver doesn't guarantee the mode:
os.Chmod(dbPath, 0600)
```

## 6. Error handling conventions

- User-facing errors go to stderr, prefixed `jot: `.
- Wrap internal errors with context (`fmt.Errorf("opening database: %w", err)`)
  but print only the final message — no stack trace in v1.
- A malformed `config.toml` names the file and shows the parser's own error
  text unmodified (edge case table, product plan).

## 7. Testing strategy

- **`internal/store`**: table-driven tests against a `t.TempDir()` SQLite file,
  covering FR-1 through FR-13 directly against the store API. Include the
  concurrency acceptance criterion explicitly: N goroutines calling
  `CreateNote` concurrently against one `*Store`, assert N rows and zero
  errors.
- **`internal/cli` / dispatch**: cover §4.1 explicitly — every reserved word
  dispatches correctly, `jot -- <reserved word>` captures it literally, a
  leading-dash body requires `--`, both delete filters together is a usage
  error (exit 2).
- **End-to-end**: a test invoking the built binary against a temp `JOT_HOME`,
  covering the full round trip from the product plan's acceptance criteria:
  capture → list → search → show → rm → restore.

## 8. Build & distribution

- `go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags --always)"`
- `CGO_ENABLED=0` for every target — only possible because of the
  `modernc.org/sqlite` choice in §1.
- GoReleaser targets: `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`.
- GoReleaser also generates the Homebrew tap formula (NFR-7).

## 9. Resolved questions (decision log)

No open implementation questions remain for v1. Kept here for traceability
rather than deleted outright:

- **`rm --hard` confirmation wording** — resolved in §4.5 (show what's being
  destroyed, require typing `yes` in full, decline exits 0).
- **`jot config set`** — not built. v1 stays read-only; config is edited by
  hand in the TOML file. Discoverability handled via help text, §4.6.
- **Migration framework** — not built for v1's single schema version. Revisit
  deliberately if/when a second version is ever needed, rather than adopting
  a framework speculatively now.
- **`--since`/`--until` format** — confirmed as exactly RFC3339 or
  `YYYY-MM-DD`; no looser or relative formats in v1 (§4.2).

New questions that surface once implementation actually starts belong here,
not scattered across code comments.
