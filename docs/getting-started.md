# Getting started

Start `dr-charm`, enter DragonRealms, and send your first command.

Install `dr-charm` before you continue. You also need a DragonRealms account
with a character. See the [install instructions](../README.md#install) if
needed.

`dr-charm` logs the session by default. The transcript contains game output
and the commands you type.

## Create the configuration file

Run:

```sh
dr-charm
```

`dr-charm` creates a commented configuration template, prints its path, and
exits.

Open the printed path in a text editor. Replace the empty values with your
account name, password, and character name:

```yaml
account: YOUR_ACCOUNT_NAME
password: YOUR_PASSWORD
character: YOUR_CHARACTER_NAME
```

Save the file. Keep these credentials in the configuration file. Do not put
them in command arguments.

## Enter DragonRealms

Run the client again:

```sh
dr-charm
```

The dashboard starts at `CONNECTING`. Wait for it to show `READY`, type `look`,
and press Enter. The response appears above the dashboard in terminal
scrollback.

## Use the interface

Type a DragonRealms command on the input line and press Enter to send it. Up
and Down recall commands from this session. The dashboard shows your location,
exits, left and right hands, and a prepared spell when one is ready. Familiar
messages join the transcript with a `[familiar]` label.

The learned map appears automatically when the terminal has enough space. Press
Escape to navigate a visible map. Use `h` and `l` to pan horizontally, `j` and
`k` to pan vertically, Ctrl-U and Ctrl-D for half-screen movement, and `g` or
`G` for the top or bottom. Press Tab or Escape to return to command entry.

Press F1 to open the control list, F3 to select a theme, F4 to toggle session
logging, and F6 to toggle auto mode. Press Escape to leave help or the theme
selector. Press Ctrl-C when you want to quit.

## Next steps

The [auto mode guide](auto-mode.md) shows how to configure an LLM agent and
whisper instructions to it.

The [configuration reference](configuration.md) covers alternate configuration
files, transcript logging, custom themes, and the learned map.
