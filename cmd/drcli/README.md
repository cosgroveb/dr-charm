# DragonRealms CLI Client

A command-line interface for interacting with the DragonRealms Charm API.

## Installation

```bash
go install ./cmd/drcli
```

Or build locally:
```bash
go build -o drcli ./cmd/drcli
```

## Usage

### Interactive Mode (default)
```bash
drcli
```

Starts an interactive session where you can type commands.

### Single Command
```bash
# Send a single command
drcli -c "look"

# Send to a different server
drcli -url http://remote:8080 -c "inventory"
```

### Watch Mode
```bash
# Continuously display new game output
drcli -w

# Watch with custom interval
drcli -w -i 500ms
```

### Show Output
```bash
# Show last 20 lines (default)
drcli -o

# Show last 50 lines
drcli -o -n 50
```

### Health Check
```bash
drcli -health
```

## Examples

### Scripting
```bash
#!/bin/bash
# Simple hunting script
drcli -c "hunt"
sleep 2
drcli -c "skin"
drcli -c "loot"

# Or pipe commands
echo -e "hunt\nskin\nloot" | drcli

# From a file
cat commands.txt | drcli
```

### Monitoring
```bash
# Watch game output and grep for whispers
drcli -w | grep -i whisper
```

### Remote Access
```bash
# Connect to dr-charm running on another machine
drcli -url http://192.168.1.100:8080 -c "look"
```

## Interactive Commands

When in interactive mode:
- `help` - Show available commands
- `health` - Check API connection
- `output` or `o` - Show recent game output  
- `quit` or `exit` - Exit the CLI
- Any other text is sent as a game command

## Flags

- `-url` - API base URL (default: http://localhost:8080)
- `-c` - Send a single command and exit
- `-o` - Show recent output and exit
- `-n` - Number of output lines to show (default: 20)
- `-w` - Watch mode - continuously show new output
- `-i` - Watch interval (default: 1s)
- `-health` - Check API health and exit
- `-timeout` - Request timeout (default: 10s)