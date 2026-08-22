# AGENTS.md

## Project

`dr-charm` is a Go terminal client for DragonRealms. It authenticates through
`eaccess.play.net`, connects to the game socket, parses StormFront-style XML,
renders a Bubble Tea TUI, and can expose HTTP and SSE control surfaces for AI
agents.

Treat the repository as a working prototype. Preserve behavior observed in the
client, and verify README claims in code before relying on them.

## Safety boundaries

- Do not put real DragonRealms credentials in command arguments, tests, logs,
  fixtures, or examples. Sanitize captured authentication and XML traffic.
- Do not start a live game session or send a game command without an explicit
  user request.
- MCP tools such as `send_command`, `social_say`, and `social_whisper` affect the
  live character. Treat them as external writes.
- The current HTTP and SSE servers bind to `:<port>` and have no authentication.
  Do not expose them outside a trusted local machine.
- Observe a running TUI with `tmux capture-pane`. Do not send keys to its pane
  unless the user asks.

## Current architecture

1. `cmd/dr-charm/main.go` loads configuration, performs account and character
   authentication, opens the game connection, starts optional MCP servers, and
   launches the UI.
2. `internal/ui.EnhancedModel` owns the active socket read loop. It also writes
   player commands to the socket, applies aliases and highlights, logs output,
   and renders the screen.
3. `internal/game.XMLStreamParser` turns network chunks into display text while
   mutating a shared `GameState`. The UI performs more room parsing on that text.
4. `internal/game.GameClient` writes MCP commands, tracks connection activity,
   and stores output and social-event buffers.
5. `internal/mcp` exposes command tools over HTTP and social events over SSE.
   `.mcp.json` points to an external Node bridge that is not part of this repo.

Socket writes and game-state parsing have more than one owner. Do not add
another direct connection or state-mutation path. Confirm that code is active
before extending `XMLParser`, `VitalsParser`, `runGameLoop`, `SessionReplay`, or
the performance harness.

## Verification commands

Use a temporary output path so a review build does not replace the running
binary:

```sh
go build -o /tmp/dr-charm ./cmd/dr-charm
gofmt -l .
go vet ./...
go test -race ./...
```

`go build .` fails because the repository root has no Go package. The Dagger
`Build` function currently runs that command, so do not report the Dagger CI
pipeline as passing without running and fixing it. `make build` writes
`./dr-charm` in the repository.

## Tests

Only `internal/social` has automated tests. Add focused tests when changing
authentication, stream parsing, game state, MCP behavior, or Bubble Tea update
logic.

- Parser tests should split representative XML at arbitrary byte boundaries.
- Network tests should use `net.Pipe` or `httptest`, not a live account.
- MCP tests should cover authentication, bind address, input validation, and
  command visibility before the server is treated as safe for agent use.

## Change rules

- Read surrounding code before changing package boundaries.
- Keep one reason per change. Avoid cleanup unrelated to the requested work.
- Prefer one clear owner for connection lifecycle, command serialization, and
  canonical game state when overhaul work begins.
- Remove unused feature scaffolding instead of completing it without a current
  requirement.
- Run `gofmt` on changed Go files and repeat the narrow reproduction before the
  broader checks.
