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

## Try auto mode (optional)

Add an `agent` block to the configuration file before starting `dr-charm`.
Use an endpoint you trust. The [auto mode reference](configuration.md#auto-mode)
lists each setting and the data sent with a request.

Once the status bar shows `READY`, press F6. The status changes from
`AGENT off` to `AGENT idle`. Pressing F6 does not contact the endpoint. The
agent waits for the next DragonRealms prompt or a whisper.

While auto mode is on, the Input pane becomes Whisper. Type a note for the
agent and press Enter to wake it. The note appears as `[whisper]` and is not
sent to DragonRealms. Agent replies appear as `[agent]`. Commands chosen by the
agent appear as `[agent] > command` and are sent to DragonRealms.

Press F6 to turn auto mode off. This cancels a request in progress and restores
normal command entry.

The [configuration reference](configuration.md) covers alternate configuration
files, transcript logging, custom themes, and the learned map.
