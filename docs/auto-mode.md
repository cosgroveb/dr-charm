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
endpoint must support streaming OpenAI Responses requests and function tools.
A Chat Completions endpoint will not work.

Choose an endpoint you trust. Each request includes recent Game and Familiar
text.

See the [configuration reference](configuration.md#auto-mode) for field details
and the data included with each request.

## Turn auto mode on

Start `dr-charm` and wait for the status bar to show `READY`. Press F6. The
agent status changes from `AGENT off` to `AGENT idle`.

F6 does not contact the endpoint. The next DragonRealms prompt wakes the agent.
To wake it now, type a whisper and press Enter.

## Whisper to the agent

While auto mode is on, the Input pane becomes Whisper. Text entered there goes
to the agent instead of DragonRealms. The agent can reply in the Game pane or
send one game command.

Whispers and replies use bold text with reverse-video `[whisper]` and `[agent]`
labels. Commands start with `[agent] >`.

## Turn auto mode off

Press F6 again. `dr-charm` cancels any request in progress and restores normal
command entry.

If the status bar shows `AGENT error`, check the endpoint URL, API key, and
model name. The next game prompt or whisper starts another request.
