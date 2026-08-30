# AGENTS.md

## Project

`dr-charm` is a personal Go terminal client for DragonRealms. It authenticates
through `eaccess.play.net`, connects to the game socket, decodes StormFront XML,
and renders a Bubble Tea TUI.

The client is DragonRealms-specific. Do not add generic Simutronics support,
compatibility layers for the removed prototype, or features for other clients.

## Safety boundaries

- Do not put real DragonRealms credentials in command arguments, tests, logs,
  fixtures, examples, or tool output.
- Do not print SGE responses, game keys, authentication bytes, or raw captured
  XML. Protocol errors and diagnostics must remain sanitized.
- Do not start a live game session or send a game command without an explicit
  user request.
- Observe a running TUI with `tmux capture-pane`. Do not send keys to its pane
  unless the user asks.
- Treat `config.example.yaml` as placeholders, not a usable credential source.

## Architecture

1. `cmd/dr-charm/main.go` loads configuration, creates one
   `dragonrealms.Session`, constructs the presentation client and UI, and runs
   Bubble Tea.
2. `internal/dragonrealms.Session` owns SGE authentication, the game connection,
   retries, reconnects, command serialization, XML decoding, protocol state,
   update publication, cancellation, and goroutine joins.
3. The private decoder converts arbitrary network chunks into protocol events.
   The private reducer is the only owner of canonical game state.
4. Session publishes detached `dragonrealms.Update` values. Public `Snapshot`
   fields are semantic UI state; raw component dictionaries stay private.
5. `internal/dragonrealms/presenter` translates Session updates into the
   protocol-neutral `internal/presentation` model. `internal/ui.EnhancedModel`
   consumes presentation updates and sends commands through that client. The UI
   owns presentation history, aliases, highlighting, themes, and logging. It
   must not know XML tags, SGE fields, endpoints, socket rules, or retry policy.

Do not introduce a second socket reader, socket writer, protocol parser, or
mutable game-state owner. `Snapshot.Connection` is the connection truth exposed
to consumers.

## Configuration

`config.LoadResolved` applies this order:

1. An explicit `--config` path, with `~/` expansion, when supplied.
2. Otherwise, `$XDG_CONFIG_HOME/dr-charm/config.yaml`, or
   `~/.config/dr-charm/config.yaml`.
3. Required-field validation.

The default template is created in a 0700 directory with mode 0600. Existing
file and directory modes are accepted. Existing unreadable or invalid files are
errors. Production does not read `DR_ACCOUNT`, `DR_PASSWORD`, or `DR_CHARACTER`.
The tagged test-only E2E path may use `DR_E2E_CONFIG` or the complete credential
tuple.

`$XDG_CONFIG_HOME/dr-charm/themes` contains custom themes. Session logs use
`$XDG_STATE_HOME/dr-charm/logs`, with `~/.config` and `~/.local/state` fallbacks.
New log directories are 0700 and new log files are 0600.

## Terminal text and presentation

Every terminal-visible string crossing the Session boundary is sanitized.
Sanitization repairs invalid UTF-8, normalizes line endings, strips terminal
control sequences, preserves printable Unicode, TAB, and LF, and removes other
controls. Presentation code may add local styling for the terminal, but raw
network text and protocol values must not enter public state.

## Tests

Run focused tests before the broader checks. Parser tests must cover arbitrary
byte splits, split UTF-8, partial markup, literal angle brackets, and detached
published data. Network tests use `net.Pipe` or loopback TCP servers, not a live
account.

The tagged E2E test is the only live test. Selecting it requires either
`DR_E2E_CONFIG` or the complete `DR_ACCOUNT`, `DR_PASSWORD`, and `DR_CHARACTER`
tuple. Missing or placeholder credentials fail setup. This environment access
is test-only. The test must not skip, substitute a fake, or send commands
beyond automatic `look` and `flags`.

Use a temporary output path so a review build does not replace the running
binary:

```sh
gofmt -l .
go vet ./...
go test -race -shuffle=on ./...
go build -o /tmp/dr-charm ./cmd/dr-charm
```

Run the live test only with explicit approval and a non-placeholder credential
source:

```sh
DR_E2E_CONFIG=/path/to/config.yaml \
  go test -tags=e2e ./cmd/dr-charm \
  -run TestDragonRealmsEndToEnd -count=1 -timeout=60s
```

## Change rules

- Read surrounding code before changing package boundaries.
- Keep one reason per change and avoid unrelated cleanup.
- Preserve the single Session boundary instead of adding adapters or producer
  interfaces.
- Use unexported dependency injection only for package tests. The UI may keep
  its small consumer-owned session interface.
- Every session goroutine needs a cancellation and join path. Every update send
  must unblock on session cancellation.
- Never expose credentials, auth payloads, game keys, raw responses, or raw
  component maps through errors, diagnostics, or public types.
- Run `gofmt` on changed Go files, repeat focused tests, then run the full suite.
