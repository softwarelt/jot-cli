# jot

`jot` is a local-first CLI for capturing quick notes with tags, timestamps,
and keyword search — no server, no account, no network calls. Everything
lives in a single SQLite file under `~/.jot`.

## Using jot

### Capture a note

No subcommand needed for the most common action — just quote the text:

```bash
jot "renew the domain before it expires" domains reminders
```

Anything after the note text is treated as tags. Run `jot` with no
arguments at all to see your most recent notes (a default `list`).

If your note text happens to start with a word `jot` treats specially (see
Subcommands below) or a `-`, force it to be captured literally with `--`:

```bash
jot -- "list of things to buy" shopping
```

### Subcommands

```bash
jot list [flags]         # browse and filter notes
jot search <term>        # keyword search over note bodies
jot show <id>            # full detail for one note
jot tags                 # distinct tags with counts
jot rm <id>               # soft-delete a note
jot restore <id>          # undo a soft delete
jot config                # show resolved configuration
```

`<id>` accepts any unambiguous prefix of a note's id (4+ characters) — the
short id printed after `jot "..."` or in `jot list` is enough.

Useful flags on `list` and `search`:

| Flag | Meaning |
|---|---|
| `-t, --tag <tag>` | filter by tag |
| `-s, --since <date>` | only notes on/after this date (`RFC3339` or `YYYY-MM-DD`) |
| `-u, --until <date>` | only notes on/before this date (`list` only) |
| `-n, --limit <n>` | maximum notes to show (`list` only) |
| `-j, --json` | machine-readable output |
| `--include-deleted` | also include soft-deleted notes |
| `--deleted-only` | show only soft-deleted notes |

Deleting is soft by default, so `jot rm <id>` is always safely reversible
with `jot restore <id>`. To permanently purge a note, use `jot rm <id> --hard`
(prompts for confirmation unless you pass `-y/--yes` or you're running
non-interactively).

### Configuration

`jot config` prints the fully resolved configuration and where it came from.
There's no `config set` — edit `~/.jot/config.toml` directly (a plain TOML
file) and changes take effect on the next run:

```toml
[storage]
path = "/custom/path/data.sqlite"

[display]
timezone = "America/New_York"
default_list_limit = 50
```

Resolution order is CLI flag > environment variable > config file > built-in
default. `JOT_HOME` overrides where `~/.jot` lives; `JOT_CONFIG` overrides
the config file path specifically.

## Developing jot

### Requirements

- Go 1.27+ (see `go.mod`)
- No cgo, no C toolchain needed — `modernc.org/sqlite` is pure Go, which is
  what keeps `jot` a single static binary across platforms.

### Build & run

```bash
go build -o jot ./cmd/jot
./jot "hello world" test
```

Or run straight from source without building a binary:

```bash
go run ./cmd/jot "hello world" test
```

During development, point `JOT_HOME` at a scratch directory so you don't
pollute your real notes:

```bash
JOT_HOME=/tmp/jot-dev go run ./cmd/jot "scratch note"
```

### Test, vet, format

```bash
go build ./...
go vet ./...
go test ./... -race -count=1
gofmt -l .        # should print nothing
go mod tidy       # should produce no diff
```

This is exactly what CI (`.github/workflows/ci.yml`) runs on every push and
PR, on both Ubuntu and macOS.

### Project layout

```
cmd/jot/            os.Args dispatch — decides "capture" vs. subcommand vs. version
internal/cli/       cobra command tree, one file per subcommand
internal/store/     the only package that knows SQL exists (schema, queries)
internal/config/    config struct + TOML/env/flag resolution
internal/idgen/     UUIDv7 id generation
internal/output/    table and JSON rendering
```

See [DESIGN.md](DESIGN.md) for the full technical design, including the
schema, dispatch rules, and open questions.
