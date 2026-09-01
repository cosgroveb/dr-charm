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

Transcripts contain game output and player commands. New log directories use
mode 0700 and new files use mode 0600. When logging starts, `dr-charm` also sets
an existing log directory to 0700 and matching transcript files to 0600.

The logger uses soft limits of 30 files and 100 MiB. When logging starts,
`dr-charm` removes the oldest closed matching files when either limit is
exceeded. It leaves the active transcript alone. A cleanup failure produces a
warning and leaves logging on when the new file is usable.

## Learned map

`dr-charm` learns rooms as you move and saves them as Genie-compatible XML at
the path above. Press F5 to switch the Room pane between room details and the
learned map.

## Custom themes

The theme directory contains JSON files. `dr-charm` loads them at startup in
filename order after the built-in `default`, `dark`, and `high-contrast`
themes. F3 opens the theme list. Up and Down change the selection, and Enter
returns to the game.

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
| `foreground` | Text color. |
| `border` | Pane border color. |
| `title_bar` | Title color. |
| `status_bar` | Status text color. |
| `status_bar_bg` | Status background color. |
| `border_type` | `normal`, `hidden`, `thick`, `double`, or `rounded`. |
| `padding` | Space inside pane borders. |

An empty or unknown `border_type` uses a rounded border. `dr-charm` warns and
skips a file with an unknown key, more than one JSON value, or an empty `name`.

When two files use the same theme name, `dr-charm` uses the definition from the
later filename. A custom theme can replace a built-in without moving its
position in the F3 theme list.
