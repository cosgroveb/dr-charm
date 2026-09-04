package ui

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"dr-charm/internal/agent"
	"dr-charm/internal/presentation"
)

type agentFunc func(context.Context, agent.Request) (agent.Result, error)

func (f agentFunc) Step(ctx context.Context, request agent.Request) (agent.Result, error) {
	return f(ctx, request)
}

func TestAgentToggleWhisperPromptAndPresentation(t *testing.T) {
	useANSI256(t)
	var requests []agent.Request
	stepper := agentFunc(func(_ context.Context, request agent.Request) (agent.Result, error) {
		requests = append(requests, request)
		return agent.Result{Text: "Wait.\nWatch.", History: "accepted"}, nil
	})
	model := newAgentTestModel(t, stepper)
	logger := &fakeLogger{enabled: true}
	model.logger = logger
	if !strings.Contains(model.buildStatusBar(), "AGENT off") {
		t.Fatalf("initial status=%q", model.buildStatusBar())
	}
	updated, cmd := model.Update(key(tea.KeyF6))
	model = updated.(EnhancedModel)
	if cmd != nil || len(requests) != 0 || !strings.Contains(model.buildStatusBar(), "AGENT idle") || !strings.Contains(model.renderInputPane(), "Whisper") || !strings.Contains(model.buildInput(), "whisper> ") {
		t.Fatalf("enabled state status=%q requests=%d cmd=%v", model.buildStatusBar(), len(requests), cmd)
	}
	model.input.SetValue("hold here")
	updated, cmd = model.Update(key(tea.KeyEnter))
	model = updated.(EnhancedModel)
	whisper := model.mainOutput[len(model.mainOutput)-1]
	if cmd == nil || model.input.Value() != "" || !strings.Contains(whisper, "\x1b[") || !strings.Contains(whisper, "[whisper]") || !strings.Contains(whisper, "hold here") || len(model.session.(*fakeSession).sent) != 0 || !strings.Contains(model.buildStatusBar(), "AGENT thinking") {
		t.Fatalf("whisper state input=%q output=%v", model.input.Value(), model.mainOutput)
	}
	updated, _ = model.Update(cmd())
	model = updated.(EnhancedModel)
	output := strings.Join(model.mainOutput, "\n")
	if len(model.mainOutput) < 2 {
		t.Fatalf("agent reply missing: %v", model.mainOutput)
	}
	for _, line := range model.mainOutput[len(model.mainOutput)-2:] {
		if !strings.Contains(line, "\x1b[") || !strings.Contains(line, "[agent]") {
			t.Fatalf("agent reply is not emphasized: %q", line)
		}
	}
	if len(requests) != 1 || len(requests[0].Whispers) != 1 || model.agent.history != "accepted" || !strings.Contains(output, "Wait.") || !strings.Contains(output, "Watch.") || len(logger.writes) != 0 {
		t.Fatalf("request=%+v model=%+v", requests, model.agent)
	}

	update := presentation.Update{Connection: presentation.Ready, Prompted: true, Prompt: ">"}
	updated, cmd = model.Update(update)
	model = updated.(EnhancedModel)
	message := cmd()
	batch, ok := message.(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("prompt command=%T %#v", message, message)
	}
	updated, _ = model.Update(batch[0]())
	model = updated.(EnhancedModel)
	if len(requests) != 2 || model.agent.status != "idle" {
		t.Fatalf("prompt requests=%d status=%q", len(requests), model.agent.status)
	}

	for _, noWake := range []presentation.Update{
		{Connection: presentation.Ready, Prompt: ">"},
		{Connection: presentation.Ready, Entries: []presentation.Entry{{Pane: presentation.Game, Text: "ordinary"}}},
		{Connection: presentation.Reconnecting, Prompted: true},
		{Connection: presentation.Reconnecting},
		{Connection: presentation.Ready},
	} {
		updated, next := model.Update(noWake)
		model = updated.(EnhancedModel)
		if next == nil {
			t.Fatal("session read was not rearmed")
		}
	}
	if len(requests) != 2 {
		t.Fatalf("ordinary updates woke agent: %d", len(requests))
	}
	if !strings.Contains(model.renderHelp(), "F6") {
		t.Fatal("help omits F6")
	}
	updated, _ = model.Update(key(tea.KeyF6))
	model = updated.(EnhancedModel)
	if model.agent.history != "accepted" || !strings.Contains(model.buildStatusBar(), "AGENT off") {
		t.Fatalf("F6 off history=%q status=%q", model.agent.history, model.buildStatusBar())
	}
}

func TestAgentStepContextEndsAfterSuccessAndError(t *testing.T) {
	for _, test := range []struct {
		name   string
		result agent.Result
		err    error
	}{
		{name: "success", result: agent.Result{Text: "ok", History: "ok"}},
		{name: "error", err: errors.New("failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			var captured context.Context
			model := newAgentTestModel(t, agentFunc(func(ctx context.Context, _ agent.Request) (agent.Result, error) {
				captured = ctx
				return test.result, test.err
			}))
			model.agent.enabled = true
			message := model.wakeAgent()()
			if captured == nil || !errors.Is(captured.Err(), context.Canceled) {
				t.Fatalf("completed request context error=%v", captured.Err())
			}
			updated, _ := model.Update(message)
			model = updated.(EnhancedModel)
			if model.agent.cancel != nil {
				t.Fatal("completed request retained cancel function")
			}
		})
	}
}

func TestAgentMissingConfigurationAndPreReadyWhisper(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	updated, cmd := model.Update(key(tea.KeyF6))
	model = updated.(EnhancedModel)
	if cmd != nil || model.agent.enabled || !strings.Contains(strings.Join(model.mainOutput, "\n"), "agent is not configured") {
		t.Fatalf("missing configuration state=%+v", model.agent)
	}

	model = newAgentTestModel(t, agentFunc(func(context.Context, agent.Request) (agent.Result, error) {
		t.Fatal("agent called before ready")
		return agent.Result{}, nil
	}))
	model.agent.enabled = true
	model.snapshot.Connection = presentation.Reconnecting
	model.input.SetValue("wait")
	updated, cmd = model.Update(key(tea.KeyEnter))
	model = updated.(EnhancedModel)
	if cmd != nil || model.input.Value() != "wait" || len(model.agent.whispers) != 0 || !strings.Contains(strings.Join(model.mainOutput, "\n"), "connection is ready") {
		t.Fatalf("pre-ready whisper state=%+v input=%q", model.agent, model.input.Value())
	}
}

func TestAgentCommandUsesAliasPathAndCommitsHistoryAfterSend(t *testing.T) {
	session := &fakeSession{updates: make(chan presentation.Update)}
	stepper := agentFunc(func(context.Context, agent.Request) (agent.Result, error) {
		return agent.Result{Command: "n", History: "sent"}, nil
	})
	model := newTestModel(t, session, false)
	model.agent = agentState{client: stepper, ctx: context.Background(), enabled: true, status: "idle"}
	logger := &fakeLogger{enabled: true}
	model.logger = logger
	model.snapshot.Connection = presentation.Ready
	cmd := model.wakeAgent()
	updated, _ := model.Update(cmd())
	model = updated.(EnhancedModel)
	if len(session.sent) != 1 || session.sent[0] != "north" || model.agent.history != "sent" || !strings.Contains(strings.Join(model.mainOutput, "\n"), "[agent] > n") || len(logger.writes) != 1 || logger.writes[0] != "> n" {
		t.Fatalf("sent=%v history=%q output=%v writes=%v", session.sent, model.agent.history, model.mainOutput, logger.writes)
	}

	session.err = errors.New("unavailable")
	model.agent.history = "before"
	cmd = model.wakeAgent()
	updated, _ = model.Update(cmd())
	model = updated.(EnhancedModel)
	if model.agent.history != "before" || strings.Count(strings.Join(model.mainOutput, "\n"), "[agent] > n") != 1 || model.agent.status != "error" {
		t.Fatalf("failed send committed result: history=%q status=%q output=%v", model.agent.history, model.agent.status, model.mainOutput)
	}
}

func TestAgentSupersessionClonesWhispersAndNeverOverlaps(t *testing.T) {
	calls := make(chan agent.Request, 2)
	stepper := agentFunc(func(ctx context.Context, request agent.Request) (agent.Result, error) {
		calls <- request
		<-ctx.Done()
		return agent.Result{}, ctx.Err()
	})
	model := newAgentTestModel(t, stepper)
	model.agent.enabled = true
	model.agent.whispers = []string{"first"}
	first := model.wakeAgent()
	results := make(chan tea.Msg, 1)
	go func() { results <- first() }()
	request := <-calls
	model.agent.whispers[0] = "changed"
	model.agent.whispers = append(model.agent.whispers, "second")
	if request.Whispers[0] != "first" {
		t.Fatalf("in-flight request aliased model state: %+v", request)
	}
	if cmd := model.wakeAgent(); cmd != nil {
		t.Fatal("supersession overlapped request")
	}
	message := <-results
	replacement := model.handleAgentResult(message.(agentResultMsg))
	if replacement == nil {
		t.Fatal("supersession did not queue replacement")
	}
	go func() { results <- replacement() }()
	replacementRequest := <-calls
	if len(replacementRequest.Whispers) != 2 || replacementRequest.Whispers[1] != "second" {
		t.Fatalf("replacement request=%+v", replacementRequest)
	}
	model.cancelAgent()
	<-results
	if len(model.agent.whispers) != 0 {
		t.Fatal("lifecycle cancellation kept whispers")
	}
}

func TestAgentReconnectWaitsForCanceledRequestBeforeRestart(t *testing.T) {
	requests := make(chan agent.Request, 2)
	release := make(chan struct{})
	var calls int32
	stepper := agentFunc(func(_ context.Context, request agent.Request) (agent.Result, error) {
		call := atomic.AddInt32(&calls, 1)
		requests <- request
		if call == 1 {
			<-release
			return agent.Result{Command: "stale", History: "stale"}, nil
		}
		return agent.Result{Text: "current", History: "current"}, nil
	})
	model := newAgentTestModel(t, stepper)
	model.agent.enabled = true
	first := model.wakeAgent()
	results := make(chan tea.Msg, 1)
	go func() { results <- first() }()
	<-requests
	updated, _ := model.Update(presentation.Update{Connection: presentation.Reconnecting})
	model = updated.(EnhancedModel)
	updated, _ = model.Update(presentation.Update{Connection: presentation.Ready, Prompted: true})
	model = updated.(EnhancedModel)
	if len(requests) != 0 || !model.agent.restart {
		t.Fatalf("request overlapped before cancellation returned: queued=%d restart=%v", len(requests), model.agent.restart)
	}
	close(release)
	updated, replacement := model.Update(<-results)
	model = updated.(EnhancedModel)
	if replacement == nil || len(model.session.(*fakeSession).sent) != 0 {
		t.Fatal("stale request was accepted or replacement omitted")
	}
	updated, _ = model.Update(replacement())
	model = updated.(EnhancedModel)
	if model.agent.history != "current" || len(model.session.(*fakeSession).sent) != 0 {
		t.Fatalf("replacement history=%q sent=%v", model.agent.history, model.session.(*fakeSession).sent)
	}
}

func TestAgentCancellationAndLateResultsAreIgnored(t *testing.T) {
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	stepper := agentFunc(func(ctx context.Context, _ agent.Request) (agent.Result, error) {
		started <- ctx
		<-release
		return agent.Result{Command: "north", History: "late"}, nil
	})
	for _, cancel := range []struct {
		name  string
		apply func(*EnhancedModel)
	}{
		{name: "F6 off", apply: func(m *EnhancedModel) { updated, _ := m.Update(key(tea.KeyF6)); *m = updated.(EnhancedModel) }},
		{name: "disconnect", apply: func(m *EnhancedModel) {
			updated, _ := m.Update(presentation.Update{Connection: presentation.Reconnecting})
			*m = updated.(EnhancedModel)
		}},
		{name: "source close", apply: func(m *EnhancedModel) { updated, _ := m.Update(sessionClosedMsg{}); *m = updated.(EnhancedModel) }},
		{name: "Ctrl-C", apply: func(m *EnhancedModel) { updated, _ := m.Update(ctrlKey('c')); *m = updated.(EnhancedModel) }},
	} {
		t.Run(cancel.name, func(t *testing.T) {
			model := newAgentTestModel(t, stepper)
			model.agent.enabled = true
			model.agent.whispers = []string{"pending"}
			cmd := model.wakeAgent()
			result := make(chan tea.Msg, 1)
			go func() { result <- cmd() }()
			ctx := <-started
			cancel.apply(&model)
			if ctx.Err() == nil || len(model.agent.whispers) != 0 {
				t.Fatalf("request not canceled: err=%v whispers=%v", ctx.Err(), model.agent.whispers)
			}
			close(release)
			updated, next := model.Update(<-result)
			model = updated.(EnhancedModel)
			if next != nil || len(model.session.(*fakeSession).sent) != 0 || model.agent.history == "late" {
				t.Fatalf("late result accepted: sent=%v history=%q", model.session.(*fakeSession).sent, model.agent.history)
			}
			release = make(chan struct{})
		})
	}

	model := newAgentTestModel(t, stepper)
	model.agent.enabled = true
	cmd := model.wakeAgent()
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	ctx := <-started
	if err := model.Close(); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("Close did not cancel request")
	}
	close(release)
	updated, next := model.Update(<-result)
	model = updated.(EnhancedModel)
	if next != nil || len(model.session.(*fakeSession).sent) != 0 || model.agent.history == "late" {
		t.Fatalf("Close accepted late result: sent=%v history=%q", model.session.(*fakeSession).sent, model.agent.history)
	}
}

func TestAgentRecentContextIsBoundedCleanPresentationText(t *testing.T) {
	useANSI256(t)
	var got agent.Request
	stepper := agentFunc(func(_ context.Context, request agent.Request) (agent.Result, error) {
		got = request
		return agent.Result{Text: "ok", History: "ok"}, nil
	})
	model := newAgentTestModel(t, stepper)
	model.applySessionUpdate(presentation.Update{Connection: presentation.Ready, Entries: []presentation.Entry{
		{Pane: presentation.Game, Text: "Goblin just arrived.", Operation: presentation.Append},
		{Pane: presentation.Familiar, Text: "Familiar speaks.", Operation: presentation.Append},
		{Pane: presentation.Game, Text: "replacement", Operation: presentation.Replace},
	}})
	model.appendSystem("local notice")
	model.appendPane(paneMain, "> local echo")
	model.agent.enabled = true
	updated, _ := model.Update(model.wakeAgent()())
	model = updated.(EnhancedModel)
	if !strings.Contains(got.Recent, "Goblin just arrived.") || !strings.Contains(got.Recent, "Familiar speaks.") || strings.ContainsAny(got.Recent, "\x1b") || strings.Contains(got.Recent, "local notice") || strings.Contains(got.Recent, "local echo") || strings.Contains(got.Recent, "replacement") {
		t.Fatalf("recent=%q", got.Recent)
	}
	model.addRecent(strings.Repeat("界", recentLimit))
	if len(model.agent.recent) > recentLimit || !utf8.ValidString(model.agent.recent) {
		t.Fatalf("bounded recent bytes=%d valid=%v", len(model.agent.recent), utf8.ValidString(model.agent.recent))
	}
}

func TestAgentErrorIsOneSanitizedBoundedLine(t *testing.T) {
	stepper := agentFunc(func(context.Context, agent.Request) (agent.Result, error) {
		return agent.Result{}, errors.New(strings.Repeat("x", 300) + "\nsecret\x1b[31m")
	})
	model := newAgentTestModel(t, stepper)
	model.agent.enabled = true
	updated, _ := model.Update(model.wakeAgent()())
	model = updated.(EnhancedModel)
	line := model.mainOutput[len(model.mainOutput)-1]
	if model.agent.status != "error" || !strings.Contains(model.buildStatusBar(), "AGENT error") || strings.ContainsAny(line, "\n\x1b") || len([]rune(strings.TrimPrefix(line, "[system 01:02:03] agent failed: "))) != 256 {
		t.Fatalf("status=%q line=%q", model.agent.status, line)
	}
}

func newAgentTestModel(t *testing.T, stepper agentStepper) EnhancedModel {
	t.Helper()
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)}, false)
	model.agent.client = stepper
	model.agent.ctx = context.Background()
	model.snapshot.Connection = presentation.Ready
	return model
}
