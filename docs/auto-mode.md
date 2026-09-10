# How to use auto mode

Configure an agent endpoint and use F6 to control auto mode.

## Configure the agent

Stop `dr-charm`. Open its configuration file and add an `agent` block:

```yaml
agent:
  endpoint: https://YOUR_ENDPOINT/v1/responses
  api_key: YOUR_API_KEY
  model: MODEL_NAME
  character: |
    Follow my whispers. Otherwise act cautiously and avoid irreversible choices.
```

Replace each placeholder with a value accepted by your model provider. Leave
`api_key` empty when the endpoint does not require one. The
endpoint must support streaming OpenAI Responses requests, function tools, and
`tool_choice: "required"` for action requests.
A Chat Completions endpoint will not work.

Choose an endpoint you trust. Each request includes recent game and familiar
text from the terminal transcript.

See the [configuration reference](configuration.md#auto-mode) for field details
and the data included with each request.

## Turn auto mode on

Start `dr-charm` and wait for the status bar to show `READY`. Press F6. The
agent status changes from `AGENT off` to `AGENT idle`.

F6 does not contact the endpoint. The next DragonRealms prompt wakes the agent.
To wake it now, type a whisper and press Enter.

## Whisper to the agent

While auto mode is on, the input line becomes Whisper. Text entered there goes
to the agent instead of DragonRealms. The agent can reply in the terminal
transcript or send one game command.

Whispers and replies use bold text with reverse-video `[whisper]` and `[agent]`
labels. Commands start with `[agent] >`.

The agent can wait for a relevant game event or pause until you whisper again.
The status bar shows `AGENT waiting` for game-event waiting and `AGENT paused`
when only a player whisper can resume it. Prompts do not wake a paused agent.
F6 off and back on returns auto mode to its normal prompt-driven behavior.

## Turn auto mode off

Press F6 again. `dr-charm` cancels any request in progress and restores normal
command entry.

If the status bar shows `AGENT error`, check the endpoint URL, API key, and
model name. The next eligible wake starts another request. A player-paused
agent wakes only for a whisper.
