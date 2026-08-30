# dr-charm

`dr-charm` is a personal terminal client for DragonRealms. It authenticates
through the Simutronics Game Entry service, connects to the DragonRealms game
socket, decodes the StormFront XML stream, and renders a Bubble Tea interface.

The client is intentionally DragonRealms-specific. It is under active
development and is built from source.

## Build and run

Requirements:

- Go 1.25 or a Go installation that honors the Go 1.26.3 module toolchain directive
- A DragonRealms account and character

Build and run once. The first run creates an owner-only XDG configuration
template and exits.

```sh
make build
./dr-charm
```

Edit the file named in the error, fill in all three fields, then run again:

```sh
${EDITOR:-vi} "${XDG_CONFIG_HOME:-$HOME/.config}/dr-charm/config.yaml"
./dr-charm
```

Use another configuration file with `--config` (or `-c`):

```sh
./dr-charm --config /path/to/config.yaml
```

Credential flags are not supported. This keeps passwords out of shell history
and process listings.

`--no-log` disables the session transcript for one run. Logging is enabled by
default. `--version` prints the installed version without reading configuration
or opening a game connection.

Print the installed version without reading configuration or opening a game
connection:

```sh
dr-charm --version
```

## Install a release

Homebrew installs the prebuilt macOS binary from the personal tap:

```sh
brew install cosgroveb/tap/dr-charm
```

GitHub Releases publish native packages for Debian Trixie and Ubuntu Noble on
`amd64` and `arm64`. Download the package and its release checksum manifest,
verify the manifest, then install the package for the local suite and
architecture:

```sh
gh release download vX.Y.Z --repo cosgroveb/dr-charm --dir dr-charm-X.Y.Z
(cd dr-charm-X.Y.Z && sha256sum -c SHA256SUMS)
sudo apt install ./dr-charm-X.Y.Z/dr-charm_X.Y.Z-1.noble_arm64.deb
```

Replace the tag and package filename with the selected release, suite, and
architecture. Releases contain no account configuration or credentials.

Maintainers publish a release by pushing a `vMAJOR.MINOR.PATCH` tag. The release
workflow builds and verifies all six binaries, publishes the checksummed GitHub
Release assets, then updates `cosgroveb/homebrew-tap`. Configure
`HOMEBREW_TAP_TOKEN` as a fine-grained Actions secret with only
`cosgroveb/homebrew-tap` contents-write access before pushing the first tag.

## Configuration

All three fields are required:

```yaml
account: your_account_name
password: your_password
character: your_character_name
```

Configuration selection is:

1. The path passed to `--config`, when present. `~/` is expanded for this path.
2. Otherwise, `$XDG_CONFIG_HOME/dr-charm/config.yaml`, or
   `~/.config/dr-charm/config.yaml` when `XDG_CONFIG_HOME` is unset.

The default file is created as a 0600 commented template in a 0700 directory.
Existing configuration file and directory modes are accepted. Invalid or
unreadable files stop startup. `DR_ACCOUNT`, `DR_PASSWORD`, and
`DR_CHARACTER` are not production configuration inputs.

An explicit file error stops startup. An unreadable or invalid default file
also stops startup instead of falling through to a later file.

## Interface

The default view has game, room, hands, and familiar panes. The status bar
shows health, mana, fatigue, concentration, spirit, and posture. Common command
aliases such as `l` for `look` and `n` for `north` are expanded before sending.

Keys:

- `Enter`: send command
- `Up` and `Down`: command history
- `Page Up`, `Page Down`, `Home`, and `End`: scroll the active pane
- `Mouse wheel`: scroll the active pane
- `Tab` and `Shift+Tab`: cycle panes
- `Ctrl+G`: open the configured command editor
- `F1`: help
- `F2`: switch between multi-pane and single-pane views
- `F3`: choose a theme
- `F4`: toggle session logging
- `Ctrl+C`: quit

The connection and logging state are shown in the status bar. A disconnect is
shown in the interface while the session closes or reconnects. Output remains
available to scroll while the connection state changes.

Logs are written under `$XDG_STATE_HOME/dr-charm/logs/` (or
`~/.local/state/dr-charm/logs`). They contain game output and player commands,
so protect them like other account data. Logging is enabled by default and can
be disabled with `--no-log` or toggled with `F4`. New log directories are 0700
and log files are 0600. Closed matching files are retained up to 30 files and
100 MiB as a soft limit.

Custom themes are JSON files under `$XDG_CONFIG_HOME/dr-charm/themes/` (or
`~/.config/dr-charm/themes`). Each file contains
one flat object:

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

Set `name` to a nonempty string. The loader trims surrounding whitespace. The
other keys are optional. `border_type` accepts `normal`, `hidden`, `thick`,
`double`, or `rounded`. An empty or unknown `border_type` uses the rounded
border. Files with unknown keys, nested legacy objects, or additional JSON
values are reported as theme warnings and skipped.

The loader places custom themes after `default`, `dark`, and `high-contrast` in
lexical filename order. When two files use the same theme name, the value from
the later filename replaces the earlier value in the same position. A custom
file can replace a built-in theme without moving its position.

## Architecture

`internal/dragonrealms.Session` owns authentication, the game connection,
command serialization, reconnect behavior, XML decoding, and canonical game
state. It publishes detached `Update` values containing a semantic `Snapshot`
and display events. `internal/dragonrealms/presenter` translates those updates
into `internal/presentation` values for the UI; the UI does not read sockets or
parse XML.

`cmd/dr-charm` only loads configuration, creates the session and UI, and runs
Bubble Tea. Aliases, highlighting, themes, and logging remain UI concerns.

## Development

Run local checks:

```sh
gofmt -l .
go vet ./...
go test -race -shuffle=on ./...
go build -o /tmp/dr-charm ./cmd/dr-charm
```

Release-only archive checks run separately and do not require a game account:

```sh
make test-release
```

The protocol integration test uses loopback TCP servers and runs with the normal
suite. The live test is opt-in and uses real DragonRealms services. It never
falls back to a fake or skips when selected:

```sh
DR_E2E_CONFIG=/path/to/config.yaml \
  go test -tags=e2e ./cmd/dr-charm \
  -run TestDragonRealmsEndToEnd -count=1 -timeout=60s
```

## License

Licensed under the [Apache License 2.0](LICENSE).
