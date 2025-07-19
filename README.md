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
- **Multi-pane layout (default)** - Separate windows for game, room, hands, and familiar
- **Single-pane mode** - Toggle with F2 for a simplified view
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
- **MCP Integration** - Model Context Protocol support for Claude
- **Custom themes** - Create your own in ~/.dr-charm/themes/
- **Configurable triggers** - Add custom text highlights

## Usage

```bash
# Using command-line flags
./dr-charm -account <username> -password <password> -character <name>

# Using environment variables
DR_ACCOUNT=<username> DR_PASSWORD=<password> DR_CHARACTER=<name> ./dr-charm

# With MCP servers enabled
./dr-charm -account <username> -password <password> -character <name> -mcp-http -mcp-sse
```

The client starts with the enhanced multi-pane UI by default. Press F2 to toggle to single-pane mode.

### Configuration Options

**Required credentials** (must be provided via flags or environment variables):
- `-account` or `DR_ACCOUNT` - DragonRealms account name
- `-password` or `DR_PASSWORD` - Account password  
- `-character` or `DR_CHARACTER` - Character name to play

**Optional flags:**
- `-mcp-http` - Enable MCP HTTP server for commands (port 8080)
- `-mcp-http-port` - Custom port for MCP HTTP server (default: 8080)
- `-mcp-sse` - Enable MCP SSE server for events (port 8081)
- `-mcp-sse-port` - Custom port for MCP SSE server (default: 8081)

## Building

```bash
make build
# or
go build -o dr-charm ./cmd/dr-charm
```

## Keyboard Shortcuts

### General
- **Enter** - Send command
- **Up/Down** - Command history
- **Backspace** - Delete character
- **Ctrl+C** - Quit

### Navigation
- **F1** - Show help
- **F2** - Toggle multi/single-pane view
- **F3** - Theme selector
- **F4** - Toggle logging
- **F5** - Show performance stats
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

## MCP Integration

The client supports Model Context Protocol (MCP) for integration with Claude and other AI assistants. Enable MCP servers with the `-mcp-http` and `-mcp-sse` flags.

### MCP Tools Available:
- `send_command` - Send any command to the game
- `social_say` - Say something in the game
- `social_whisper` - Whisper to someone
- `get_social_events` - Get recent social interactions
- `get_connection_status` - Check connection health

For Claude integration, add the project's `.mcp.json` configuration to your Claude settings.

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