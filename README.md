# dr-charm

`dr-charm` is a personal terminal client for DragonRealms. It authenticates
through the Simutronics Game Entry service, connects to the DragonRealms game
socket, decodes the StormFront XML stream, and renders a Bubble Tea interface.

The client is intentionally DragonRealms-specific. It is under active
development and is built from source.

## Build and run

Requirements:

- Go 1.24.1 or a Go installation that honors the module toolchain directive
- A DragonRealms account and character

Create a private configuration file:

```sh
mkdir -p ~/.config/dr-charm
cp config.example.yaml ~/.config/dr-charm/config.yaml
chmod 600 ~/.config/dr-charm/config.yaml
```

Replace the placeholders in that file, then build and run:

```sh
make build
./dr-charm
```

Use another configuration file with `-config`:

```sh
./dr-charm -config /path/to/config.yaml
```

Credential flags are not supported. This keeps passwords out of shell history
and process listings.

Print the installed version without reading configuration or opening a game
connection:

```sh
dr-charm -version
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

Configuration precedence is:

1. The path passed to `-config`, when present.
2. Otherwise, the first existing file among `.dr-charm.yaml`,
   `~/.dr-charm/config.yaml`, and `~/.config/dr-charm/config.yaml`.
3. Nonempty `DR_ACCOUNT`, `DR_PASSWORD`, and `DR_CHARACTER` values override
   file values.

An explicit file error stops startup. An unreadable or invalid default file
also stops startup instead of falling through to a later file.

## Interface

The default view has game, room, hands, and familiar panes. The status bar
shows health, mana, fatigue, concentration, spirit, and posture. Common command
aliases such as `l` for `look` and `n` for `north` are expanded before sending.

Keys:

- `Enter`: send command
- `Up` and `Down`: command history
- `Page Up`, `Page Down`, `Home`, and `End`: scroll output
- `Tab`: cycle panes
- `F1`: help
- `F2`: switch between multi-pane and single-pane views
- `F3`: choose a theme
- `F4`: toggle session logging
- `Ctrl+C`: quit

Logs are written under `~/.dr-charm/logs/`. They contain game output and player
commands, so protect them like other account data.

Custom themes are JSON files under `~/.dr-charm/themes/`. Each file contains
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
border. The loader ignores files with unknown keys, nested legacy objects, or
additional JSON values.

The loader places custom themes after `default`, `dark`, and `high-contrast` in
lexical filename order. When two files use the same theme name, the value from
the later filename replaces the earlier value in the same position. A custom
file can replace a built-in theme without moving its position.

## Architecture

`internal/dragonrealms.Session` owns authentication, the game connection,
command serialization, reconnect behavior, XML decoding, and canonical game
state. It publishes detached `Update` values containing a semantic `Snapshot`
and display events. The UI consumes those values and sends player commands
through `Session.Send`; it does not read sockets or parse XML.

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
