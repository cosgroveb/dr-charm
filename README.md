# DragonRealms Charm CLI

A feature-rich DragonRealms MUD client built with Go and Charm's Bubble Tea framework.

## Features

### Core Features ✅
- Secure authentication to eaccess.play.net
- Full DragonRealms XML protocol support
- Beautiful terminal UI with Charm's Bubble Tea
- Real-time game output with ANSI color support
- Command history with up/down navigation
- Automatic "look" command on connection

### UI Features ✅
- **Single and Multi-pane layouts** - Toggle with F2
- **Scrollback buffer** - Page Up/Down, Home/End navigation
- **Status bar** - Health, mana, stamina, concentration, spirit
- **Room tracking** - Current room name in title bar
- **Vitals display** - All character vitals with color coding
- **Theme support** - Multiple color themes (F3 to switch)

### Game Features ✅
- **Triggers and highlights** - Combat, whispers, arrivals/departures
- **Command aliases** - Short commands (n→north, l→look, etc.)
- **Hands tracking** - What you're holding in each hand
- **Room window** - Exits, objects, and other players
- **Familiar window** - Companion messages and status
- **Spell tracking** - Currently prepared spell

### Utility Features ✅
- **Session logging** - Save sessions to ~/.dr-charm/logs/
- **HTTP API** - Control client via REST API on port 8080
- **Custom themes** - Create your own in ~/.dr-charm/themes/
- **Configurable triggers** - Add custom text highlights

## Usage

### Standard UI (default)
```bash
./dr-charm
```

### Enhanced UI with multi-pane support
```bash
DR_CHARM_ENHANCED=true ./dr-charm
```

The client uses hardcoded credentials in `main.go`:
- Account: cosgroveb4
- Password: [configured]
- Character: Cennedig

## Building

```bash
go build -o dr-charm .
```

## Keyboard Shortcuts

### General
- **Enter** - Send command
- **Up/Down** - Command history
- **Backspace** - Delete character
- **Ctrl+C** - Quit

### Enhanced UI Only
- **F1** - Show help
- **F2** - Toggle single/multi-pane view
- **F3** - Theme selector
- **F4** - Toggle logging
- **Tab** - Cycle through panes (multi-pane mode)

### Scrolling
- **Page Up/Down** - Scroll by page
- **Home/End** - Jump to top/bottom

## Command Aliases

### Movement
- `n` → `north`
- `s` → `south`
- `e` → `east`
- `w` → `west`
- `ne` → `northeast`
- `nw` → `northwest`
- `se` → `southeast`
- `sw` → `southwest`
- `u` → `up`
- `d` → `down`
- `o` → `out`

### Common Commands
- `l` → `look`
- `i` → `inventory`
- `sta` → `stand`
- `sit` → `sit`
- `kne` → `kneel`
- `hi` → `hide`

### Combat
- `att` → `attack`
- `ki` → `kill`
- `sk` → `skin`
- `loot` → `loot all`

## API Server

The client automatically starts an HTTP API server on port 8080 that allows external control:

```bash
# Send a command
curl -X POST http://localhost:8080/command \
  -H "Content-Type: application/json" \
  -d '{"command": "look"}'

# Get recent output
curl http://localhost:8080/output
```

See [API.md](API.md) for full documentation.

## Configuration

### Themes
Custom themes can be created in `~/.dr-charm/themes/` as JSON files. See the built-in themes for examples.

### Logs
Session logs are saved to `~/.dr-charm/logs/` with timestamps. Logs include all game output and commands.

## Development

### Running Tests
```bash
go test ./...
```

### CI/CD with Dagger
```bash
# Run full CI pipeline
dagger call ci --source .

# Run individual tasks
dagger call lint --source .
dagger call test --source .
dagger call build --source .
```

## Contributing

Pull requests are welcome! Please ensure:
1. Code passes linting (`gofmt`)
2. Tests pass (when added)
3. Dagger CI pipeline is green