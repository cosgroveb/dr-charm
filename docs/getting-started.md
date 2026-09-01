# Getting started

Start `dr-charm`, enter DragonRealms, and send your first command.

Install `dr-charm` before you continue. You also need a DragonRealms account
with a character. See the [install instructions](../README.md#install) if
needed.

This tutorial uses the default configuration path and leaves session logging
on, so the transcript contains game output and the commands you type.

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

The status bar starts at `CONNECTING`. Wait for it to show `READY`, type
`look` in the Input pane, and press Enter. The Game pane shows the response.

## Use the interface

Type a DragonRealms command in the Input pane and press Enter to send it. Up
and Down recall commands from this session.

Press Tab to focus the Input pane or a visible Game, Room, Hands, or Familiar
pane. Use Page Up and Page Down to scroll the focused output pane. Shift-Tab
moves focus in reverse.

Press F5 to switch the Room pane between room details and the learned map.

Press F1 to open the control list. Press Escape to leave help, then press
Ctrl-C when you want to quit.

The [configuration reference](configuration.md) covers alternate configuration
files, transcript logging, custom themes, and the learned map.
