# DragonRealms Charm CLI

A minimal DragonRealms client built with Go and Charm's Bubble Tea framework.

## MVP Features

### Step 1: Authentication Core ✅
- Connects to eaccess.play.net:7900
- Implements XOR password encryption
- Authenticates and retrieves game connection info

### Step 2: Console Client ✅  
- Connects to game server
- Strips basic XML tags
- Interactive command input
- Real-time game output

### Step 3: Bubble Tea UI ✅
- Beautiful TUI with Lip Gloss styling
- Scrolling output window
- Input field with command history
- Ctrl+C or Esc to exit

## Usage

```bash
./dr-charm
```

The client uses hardcoded credentials in `main.go`:
- Account: cosgroveb4
- Password: [configured]
- Character: Cennedig

## Building

```bash
go build -o dr-charm .
```

## Controls

- Type commands and press Enter to send
- Ctrl+C or Esc to exit
- Backspace to delete characters

## Next Steps

Refer to the full specification for planned features:
- Status bar with health/mana
- Script execution
- Multi-pane layout
- Improved XML parsing