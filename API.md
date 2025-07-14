# DragonRealms API Documentation

The dr-charm client exposes an HTTP API on `http://localhost:8080` that allows external control of the game session.

## Endpoints

### POST /command
Send a command to the game.

**Request:**
```json
{
  "command": "look"
}
```

**Response:**
```json
{
  "status": "sent",
  "id": 123
}
```

### GET /output
Get recent game output.

**Query Parameters:**
- `since` (optional): Only return output lines with ID greater than this value

**Response:**
```json
{
  "lines": [
    {
      "id": 124,
      "text": "You are standing in a small room.",
      "timestamp": "2024-01-14T12:00:00Z"
    },
    {
      "id": 125,
      "text": "Obvious exits: north, south",
      "timestamp": "2024-01-14T12:00:01Z"
    }
  ],
  "last_id": 125
}
```

### GET /health
Check API and connection status.

**Response:**
```json
{
  "status": "connected",
  "character": "Cennedig",
  "uptime": "5m30s"
}
```

## Usage Examples

### Send a command
```bash
curl -X POST http://localhost:8080/command \
  -H "Content-Type: application/json" \
  -d '{"command": "look"}'
```

### Get all output
```bash
curl http://localhost:8080/output
```

### Get output since a specific ID
```bash
curl http://localhost:8080/output?since=100
```

### Check connection status
```bash
curl http://localhost:8080/health
```

## Usage Pattern for Claude

1. Send a command using POST /command
2. Wait 1-2 seconds for the game to respond
3. Get output using GET /output with the last seen ID
4. Parse the output to understand the game state
5. Decide on the next action and repeat

## Notes

- The API server starts automatically when dr-charm runs
- Output buffer stores the last 1000 lines
- Commands are sent exactly as provided (add your own newlines if needed)
- The API adds "> " prefix to commands in the output for clarity