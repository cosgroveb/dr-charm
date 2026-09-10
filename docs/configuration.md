# Configuration reference

`dr-charm` reads credentials from one YAML file. It stores custom themes,
session transcripts, and the learned map in separate XDG directories.

## Credential file

The file contains three required keys:

| Key | Value |
|---|---|
| `account` | DragonRealms account name |
| `password` | DragonRealms account password |
| `character` | Character name |

```yaml
account: YOUR_ACCOUNT_NAME
password: YOUR_PASSWORD
character: YOUR_CHARACTER_NAME
```

The YAML parser rejects unknown keys and additional YAML documents. Empty
values fail validation.

`dr-charm` selects the file in this order:

1. The path passed with `--config` or `-c`. A leading `~/` expands to the home
   directory. The client reports an error if this file does not exist.
2. `$XDG_CONFIG_HOME/dr-charm/config.yaml` when `XDG_CONFIG_HOME` contains an
   absolute path.
3. `~/.config/dr-charm/config.yaml` when `XDG_CONFIG_HOME` is unset.

When the default file does not exist, `dr-charm` creates its directory with
mode 0700, creates the file with mode 0600, and exits. `dr-charm` leaves the
modes of existing files and directories unchanged.

## Auto mode

The optional `agent` block connects auto mode to an OpenAI-compatible Responses
endpoint:

```yaml
agent:
  endpoint: http://localhost:4000/v1/responses
  api_key: LOCAL_API_KEY
  model: MODEL_NAME
  character: |
    Describe how the agent should play this character.
```

| Key | Value |
|---|---|
| `agent.endpoint` | Complete HTTP or HTTPS Responses URL. Its path must end in `/responses`. |
| `agent.api_key` | Optional bearer token. Leave it empty when your endpoint does not require one. |
| `agent.model` | Model name accepted by the endpoint. |
| `agent.character` | Instructions that describe how the agent should play this character. |

`agent.endpoint`, `agent.model`, and `agent.character` are required when you add
the block. `dr-charm` posts only to the exact URL in `agent.endpoint`. The URL
cannot contain embedded credentials or a fragment.

The endpoint must support streaming OpenAI Responses requests, function tools,
and required tool selection for action requests. Chat Completions endpoints
are not supported. The
[auto mode guide](auto-mode.md) covers setup and controls.

`dr-charm` does not set a reasoning effort or service tier. Those values come
from endpoint defaults.

### Request contents

Every model request sends recent game and familiar text from the terminal
transcript to the configured endpoint. Action requests also include:

- fixed instructions and a short DragonRealms command reference
- the configured `agent.character` instructions
- earlier player-agent conversation and command choices
- new whispers
- one `send_command` function tool
- one `wait` function tool for game-event or player waiting
- required selection of exactly one action tool for each action request

Summary requests include older conversation and recent game text. `dr-charm`
does not include your DragonRealms account or password. A proxy may add its own
instructions before forwarding the request.

### Session behavior

| Status | Meaning |
|---|---|
| `AGENT off` | Auto mode is available and disabled. |
| `AGENT idle` | Auto mode is waiting for a new game prompt or whisper. |
| `AGENT thinking` | A model request is in progress. |
| `AGENT waiting` | The agent waits for a relevant game event. A prompt or whisper may wake it. |
| `AGENT paused` | The agent waits for a player whisper. Prompts do not wake it. |
| `AGENT error` | The last request failed. The next eligible prompt or whisper tries again. |

Auto mode starts off. F6 toggles it without making a request. When auto mode is
on, Enter sends input-line text to the agent as a whisper instead of sending it
to DragonRealms. The agent can reply or choose one command.

A new eligible prompt or whisper while the agent is thinking cancels the
current request and replaces it after cancellation finishes. A paused agent
only accepts a new whisper. F6, a lost connection, quitting, or closing the
session also cancels a request. Auto mode stays selected across a reconnect and
preserves whether it was paused. Turning F6 off and back on returns it to
prompt-driven behavior.

`dr-charm` keeps the most recent 16 KiB of Game and Familiar text for the agent.
After agent history grows past 32 KiB, `dr-charm` asks the model to condense it
before the next action request. Neither history nor recent game text is saved
when you quit. Whispers and agent replies never enter the session transcript.
When logging is on, commands the agent sends and sanitized agent failure
categories do.

## Command-line options

| Option | Description |
|---|---|
| `-c, --config PATH` | Read credentials from `PATH` instead of the default file. |
| `-V, --version` | Print the installed version and exit. |
| `-h, --help` | Print command help and exit. |

Help and version do not read the configuration file or open a network
connection.

## Files

| Purpose | XDG path | Fallback |
|---|---|---|
| Credentials | `$XDG_CONFIG_HOME/dr-charm/config.yaml` | `~/.config/dr-charm/config.yaml` |
| Custom themes | `$XDG_CONFIG_HOME/dr-charm/themes/` | `~/.config/dr-charm/themes/` |
| Session transcripts | `$XDG_STATE_HOME/dr-charm/logs/` | `~/.local/state/dr-charm/logs/` |
| Learned map | `$XDG_DATA_HOME/dr-charm/maps/Map00_Learned.xml` | `~/.local/share/dr-charm/maps/Map00_Learned.xml` |

`XDG_CONFIG_HOME`, `XDG_STATE_HOME`, and `XDG_DATA_HOME` must contain absolute
paths when set.

## Session transcripts

Logging starts with each session. F4 turns logging on or off while the client
runs. The status bar shows `LOG on`, `LOG off`, or `LOG failed`.

Transcripts contain game output, player and agent commands, and sanitized agent
failure categories. New log directories use mode 0700 and new files use mode
0600. When logging starts, `dr-charm` also sets an existing log directory to
0700 and matching transcript files to 0600.

The logger uses soft limits of 30 files and 100 MiB. When logging starts,
`dr-charm` removes the oldest closed matching files when either limit is
exceeded. It leaves the active transcript alone. A cleanup failure produces a
warning and leaves logging on when the new file is usable.

## Learned map

`dr-charm` learns rooms as you move and saves them as Genie-compatible XML at
the path above. The dashboard shows the map automatically when the terminal has
enough space. Press Escape to navigate it, then press Tab or Escape to return
to command entry.

## Custom themes

The theme directory contains JSON files. `dr-charm` loads them at startup in
filename order after the built-in `default`, `dark`, and `high-contrast`
themes. F3 opens the theme list. Up and Down change the selection, and Enter
or Escape returns to the dashboard. The selection previews the theme while the
list remains open.

Color values use a quoted ANSI color number from `0` through `255` or a hex
color in `#RGB` or `#RRGGBB` form.

Each file contains one flat JSON object:

```json
{
  "name": "green-screen",
  "foreground": "46",
  "border": "22",
  "title_bar": "46",
  "status_bar": "0",
  "status_bar_bg": "46",
  "border_type": "double",
  "padding": 1
}
```

| Key | Description |
|---|---|
| `name` | Theme name. This is the only required value. |
| `foreground` | Dashboard text color. An empty value uses `7`. |
| `border` | Dashboard border and divider color. An empty value uses `foreground`. |
| `title_bar` | Top-rule title and active map-navigation color. An empty value uses `foreground`. |
| `status_bar` | Status and command-input text color. An empty value uses `foreground`. |
| `status_bar_bg` | Status and command-input background color. An empty value leaves the terminal background in use. |
| `border_type` | Dashboard border style: `rounded`, `normal`, `thick`, or `double`. |
| `padding` | Horizontal dashboard padding, clamped from `0` through `2` cells. |

An empty or unknown `border_type` uses a rounded border. `dr-charm` warns and
skips a file with an unknown key, more than one JSON value, or an empty `name`.
On narrow or short terminals, the dashboard reduces horizontal padding and
then removes its frame before hiding status, hand, or location rows. The game
transcript remains unboxed.

Changing the terminal width can make the terminal reflow an already displayed
dashboard into scrollback before `dr-charm` redraws it. Those dashboard
remnants may remain in the terminal's history. `dr-charm` preserves every game
record and does not delete terminal history to hide the remnants.

When two files use the same theme name, `dr-charm` uses the definition from the
later filename. A custom theme can replace a built-in without moving its
position in the F3 theme list.
