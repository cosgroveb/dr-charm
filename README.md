# dr-charm

`dr-charm` is a terminal client for DragonRealms. Sign in with an
existing DragonRealms account and character, then play in a full-screen
terminal interface.

The client supports DragonRealms only.

## Install

Install a release on macOS, Debian 13 (Trixie), or Ubuntu 24.04 (Noble).
Releases include x86-64 and ARM64 builds.

### macOS

Install the Homebrew formula:

```sh
brew install cosgroveb/tap/dr-charm
```

### Debian and Ubuntu

Download the `.deb` for your distribution and processor from the
[latest release](https://github.com/cosgroveb/dr-charm/releases/latest). Choose
`trixie` for Debian 13 or `noble` for Ubuntu 24.04. Package names end in
`amd64.deb` for x86-64 and `arm64.deb` for ARM64.

Install the downloaded file, replacing `PACKAGE.deb` with its filename:

```sh
sudo apt install ./PACKAGE.deb
```

### Build from source

Install Git, Make, and Go 1.26.3, then build the client:

```sh
git clone https://github.com/cosgroveb/dr-charm.git
cd dr-charm
make build
```

Use `./dr-charm` in place of `dr-charm` in the examples below when running the
binary from the source directory.

## First login

Run the client once:

```sh
dr-charm
```

The first run creates a configuration file and prints its path. Open that file
and fill in the three empty values:

```yaml
account: YOUR_ACCOUNT_NAME
password: YOUR_PASSWORD
character: YOUR_CHARACTER_NAME
```

The client records game output and commands in a session log.

Run `dr-charm` again. Wait for the status bar to show `READY`, type `look`, and
press Enter. Press F5 to switch the Room pane to the learned map. Press F1 for
the controls and Ctrl-C to quit.

## Optional auto mode

Auto mode can reply in the client or choose one game command after a
DragonRealms prompt or your whisper. Configure a trusted OpenAI-compatible
`/responses` endpoint as described in the
[configuration reference](docs/configuration.md#auto-mode), then press F6.
Turning auto mode on does not contact the endpoint. A game prompt or whisper
wakes the agent. Each request includes recent text from the Game and Familiar
panes. While auto mode is on, text you enter becomes a whisper instead of a
command sent to DragonRealms. Press F6 again to stop auto mode and cancel any
request in progress.

## Documentation

- [Getting started](docs/getting-started.md) walks through the first session.
- [Configuration reference](docs/configuration.md) covers paths, logging,
  themes, the learned map, and optional auto mode.
- `dr-charm --help` lists command-line options. Debian and Ubuntu packages also
  install the `dr-charm(1)` man page.

## License

[Apache License 2.0](LICENSE)
