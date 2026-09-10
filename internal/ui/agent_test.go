package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"dr-charm/internal/agent"
	"dr-charm/internal/presentation"
	"dr-charm/internal/telemetry"
)

type agentFunc func(context.Context, agent.Request) (agent.Result, error)

func (f agentFunc) Step(ctx context.Context, request agent.Request) (agent.Result, error) {
	return f(ctx, request)
}

type fakeLogger struct {
	enabled           bool
	writeErr, stopErr error
	writes            []string
	stopCalls         int
}

func (l *fakeLogger) Start(string) (telemetry.StartResult, error) {
	l.enabled = true
	return telemetry.StartResult{}, nil
}
func (l *fakeLogger) Stop() error              { l.stopCalls++; l.enabled = false; return l.stopErr }
func (l *fakeLogger) Write(value string) error { l.writes = append(l.writes, value); return l.writeErr }
func (l *fakeLogger) IsEnabled() bool          { return l.enabled }
func (l *fakeLogger) Path() string             { return "" }

func newAgentTestModel(t *testing.T, stepper agentStepper) EnhancedModel {
	t.Helper()
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.agent.client = stepper
	model.agent.ctx = context.Background()
	model.snapshot.Connection = presentation.Ready
	return model
}

func TestAgentAndWhisperRecordsJoinTranscriptQueueWithoutLogging(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	model.snapshot.Connection = presentation.Ready
	logger := &fakeLogger{enabled: true}
	model.logger = logger
	model.appendAgentMessage("[agent]", "reply")
	if len(model.pendingTranscript) != 1 || !contains(model.pendingTranscript[0], "[agent]") {
		t.Fatalf("queue=%#v", model.pendingTranscript)
	}
	model.agent.enabled = true
	model.input.SetValue("hold here")
	if command := model.whisper(); command == nil || len(model.session.(*fakeSession).sent) != 0 || len(logger.writes) != 0 || len(model.agent.whispers) != 1 || model.agent.whispers[0] != "hold here" || !contains(strings.Join(model.pendingTranscript, "\n"), "[whisper]") || !contains(strings.Join(model.pendingTranscript, "\n"), "hold here") {
		t.Fatalf("whisper command=%v sent=%v writes=%v whispers=%v queue=%v", command, model.session.(*fakeSession).sent, logger.writes, model.agent.whispers, model.pendingTranscript)
	}
}

func TestAgentToggleWhisperPromptAndPresentation(t *testing.T) {
	var requests []agent.Request
	stepper := agentFunc(func(_ context.Context, request agent.Request) (agent.Result, error) {
		requests = append(requests, request)
		return agent.Result{Text: "Wait.\nWatch.", History: "accepted"}, nil
	})
	model := newAgentTestModel(t, stepper)
	logger := &fakeLogger{enabled: true}
	model.logger = logger
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyF6})
	model = updated.(EnhancedModel)
	if command != nil || !model.agent.enabled || model.agent.status != "idle" || !contains(model.buildInput(), "Whisper > ") {
		t.Fatalf("enabled agent=%+v input=%q", model.agent, model.buildInput())
	}
	model.input.SetValue("hold here")
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(EnhancedModel)
	if command == nil || model.input.Value() != "" || !contains(strings.Join(model.pendingTranscript, "\n"), "[whisper]") || model.agent.status != "thinking" {
		t.Fatalf("whisper queue=%q agent=%+v", model.pendingTranscript, model.agent)
	}
	updated, _ = model.Update(command())
	model = updated.(EnhancedModel)
	output := strings.Join(model.pendingTranscript, "\n")
	if len(requests) != 1 || len(requests[0].Whispers) != 1 || model.agent.history != "accepted" || !contains(output, "Wait.") || !contains(output, "Watch.") || len(logger.writes) != 0 {
		t.Fatalf("requests=%+v queue=%q agent=%+v writes=%q", requests, model.pendingTranscript, model.agent, logger.writes)
	}
	updated, command = model.Update(presentation.Update{Connection: presentation.Ready, Prompted: true, Prompt: ">"})
	model = updated.(EnhancedModel)
	if command == nil {
		t.Fatal("prompt did not schedule agent wakeup")
	}
	message := command()
	sequence := reflect.ValueOf(message)
	if sequence.Kind() != reflect.Slice || sequence.Len() < 2 {
		t.Fatalf("prompt command=%T %#v", message, message)
	}
	var result agentResultMsg
	for index := range sequence.Len() - 1 { // Final command waits for the next Session update.
		candidate, ok := sequence.Index(index).Interface().(tea.Cmd)
		if !ok {
			t.Fatalf("prompt command[%d]=%T", index, sequence.Index(index).Interface())
		}
		if got, ok := candidate().(agentResultMsg); ok {
			result = got
			break
		}
	}
	if result.generation == 0 {
		t.Fatal("prompt did not execute an agent step")
	}
	updated, _ = model.Update(result)
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
		t.Fatalf("ordinary or non-ready update woke agent: %d", len(requests))
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyF6})
	model = updated.(EnhancedModel)
	if model.agent.enabled || model.agent.history != "accepted" || !contains(model.buildStatusBar(), "AGENT off") {
		t.Fatalf("disabled agent=%+v", model.agent)
	}
}

func TestAgentPlayerWaitSuppressesPromptAcknowledgements(t *testing.T) {
	var requests int
	model := newAgentTestModel(t, agentFunc(func(context.Context, agent.Request) (agent.Result, error) {
		requests++
		return agent.Result{Text: "I will wait for you.", History: "wait", WaitUntil: agent.WaitForPlayer}, nil
	}))
	model.agent.enabled = true
	model.handleAgentResult(model.wakeAgent(false)().(agentResultMsg))
	if model.agent.status != "paused" || strings.Count(strings.Join(model.pendingTranscript, "\n"), "I will wait for you.") != 1 || len(model.session.(*fakeSession).sent) != 0 {
		t.Fatalf("status=%q transcript=%q sent=%q", model.agent.status, model.pendingTranscript, model.session.(*fakeSession).sent)
	}
	for range 3 {
		updated, command := model.Update(presentation.Update{Connection: presentation.Ready, Prompted: true, Entries: []presentation.Entry{{Pane: presentation.Game, Text: "ambient"}}})
		model = updated.(EnhancedModel)
		if command == nil {
			t.Fatal("session read was not rearmed")
		}
	}
	if requests != 1 || strings.Count(strings.Join(model.pendingTranscript, "\n"), "I will wait for you.") != 1 || len(model.session.(*fakeSession).sent) != 0 || model.agent.status != "paused" {
		t.Fatalf("requests=%d transcript=%q sent=%q status=%q", requests, model.pendingTranscript, model.session.(*fakeSession).sent, model.agent.status)
	}
}

func TestAgentWaitPhasesAndWhisperResume(t *testing.T) {
	var requests []agent.Request
	results := []agent.Result{
		{Text: "Watching.", History: "game", WaitUntil: agent.WaitForGameEvent},
		{Text: "Paused.", History: "player", WaitUntil: agent.WaitForPlayer},
		{Command: "look", History: "resumed"},
	}
	model := newAgentTestModel(t, agentFunc(func(_ context.Context, request agent.Request) (agent.Result, error) {
		requests = append(requests, request)
		result := results[0]
		results = results[1:]
		return result, nil
	}))
	model.agent.enabled = true
	model.handleAgentResult(model.wakeAgent(true)().(agentResultMsg))
	if model.agent.phase != agentWaitingForGameEvent || model.agent.status != "waiting" {
		t.Fatalf("game wait agent=%+v", model.agent)
	}
	model.handleAgentResult(model.wakeAgent(true)().(agentResultMsg))
	if model.agent.phase != agentWaitingForPlayer || model.agent.status != "paused" {
		t.Fatalf("player wait agent=%+v", model.agent)
	}
	if command := model.wakeAgent(true); command != nil || len(requests) != 2 {
		t.Fatalf("paused prompt command=%v requests=%d", command, len(requests))
	}
	model.input.SetValue("continue")
	command := model.whisper()
	if command == nil || model.input.Value() != "" || len(model.agent.whispers) != 1 || model.agent.whispers[0] != "continue" {
		t.Fatalf("whisper command=%v input=%q whispers=%q", command, model.input.Value(), model.agent.whispers)
	}
	if model.agent.phase != agentWaitingForPlayer || model.wakeAgent(true) != nil {
		t.Fatalf("prompt superseded paused whisper agent=%+v", model.agent)
	}
	model.handleAgentResult(command().(agentResultMsg))
	if model.agent.phase != agentActive || model.agent.status != "idle" || len(model.session.(*fakeSession).sent) != 1 || len(requests) != 3 || len(requests[2].Whispers) != 1 {
		t.Fatalf("resumed agent=%+v sent=%q requests=%+v", model.agent, model.session.(*fakeSession).sent, requests)
	}
}

func TestAgentToggleAndDisconnectPreserveWaitPolicy(t *testing.T) {
	model := newAgentTestModel(t, agentFunc(func(context.Context, agent.Request) (agent.Result, error) { return agent.Result{}, nil }))
	model.agent.enabled = true
	model.agent.phase = agentWaitingForPlayer
	model.agent.status = "paused"
	updated, _ := model.Update(presentation.Update{Connection: presentation.Reconnecting})
	model = updated.(EnhancedModel)
	if model.agent.phase != agentWaitingForPlayer || model.agent.status != "paused" {
		t.Fatalf("disconnect agent=%+v", model.agent)
	}
	updated, command := model.Update(presentation.Update{Connection: presentation.Ready, Prompted: true})
	model = updated.(EnhancedModel)
	if command == nil || model.wakeAgent(true) != nil || model.agent.status != "paused" {
		t.Fatalf("reconnect agent=%+v command=%v", model.agent, command)
	}
	model.toggleAgent()
	model.toggleAgent()
	if model.agent.phase != agentActive || model.agent.status != "idle" {
		t.Fatalf("toggle did not reset phase agent=%+v", model.agent)
	}
}

func TestAgentStepContextEndsAfterSuccessAndError(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "error", err: errors.New("failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			var captured context.Context
			model := newAgentTestModel(t, agentFunc(func(ctx context.Context, _ agent.Request) (agent.Result, error) {
				captured = ctx
				return agent.Result{}, test.err
			}))
			model.agent.enabled = true
			message := model.wakeAgent(false)()
			if captured == nil || !errors.Is(captured.Err(), context.Canceled) {
				t.Fatalf("completed request context error=%v", captured.Err())
			}
			model.handleAgentResult(message.(agentResultMsg))
			if model.agent.cancel != nil {
				t.Fatal("completed request retained cancel function")
			}
		})
	}
}

func TestAgentMissingConfigurationAndPreReadyWhisper(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyF6})
	model = updated.(EnhancedModel)
	if command != nil || model.agent.enabled || !contains(strings.Join(model.pendingTranscript, "\n"), "agent is not configured") {
		t.Fatalf("missing configuration agent=%+v queue=%q", model.agent, model.pendingTranscript)
	}
	model = newAgentTestModel(t, agentFunc(func(context.Context, agent.Request) (agent.Result, error) {
		t.Fatal("agent called before ready")
		return agent.Result{}, nil
	}))
	model.agent.enabled = true
	model.snapshot.Connection = presentation.Reconnecting
	model.input.SetValue("wait")
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(EnhancedModel)
	if command != nil || model.input.Value() != "wait" || len(model.agent.whispers) != 0 || !contains(strings.Join(model.pendingTranscript, "\n"), "connection is ready") {
		t.Fatalf("pre-ready whisper agent=%+v input=%q queue=%q", model.agent, model.input.Value(), model.pendingTranscript)
	}
}

func TestAgentCommandUsesAliasPathAndCommitsHistoryAfterSend(t *testing.T) {
	session := &fakeSession{updates: make(chan presentation.Update)}
	model := newTestModel(t, session)
	model.agent = agentState{client: agentFunc(func(context.Context, agent.Request) (agent.Result, error) {
		return agent.Result{Command: "n", History: "sent"}, nil
	}), ctx: context.Background(), enabled: true, status: "idle"}
	model.snapshot.Connection = presentation.Ready
	logger := &fakeLogger{enabled: true}
	model.logger = logger
	model.handleAgentResult(model.wakeAgent(false)().(agentResultMsg))
	if len(session.sent) != 1 || session.sent[0] != "north" || model.agent.history != "sent" || !contains(strings.Join(model.pendingTranscript, "\n"), "[agent] > n") || len(logger.writes) != 1 || logger.writes[0] != "> n" {
		t.Fatalf("sent=%q history=%q queue=%q writes=%q", session.sent, model.agent.history, model.pendingTranscript, logger.writes)
	}
	session.err = errors.New("unavailable")
	model.agent.history = "before"
	model.handleAgentResult(model.wakeAgent(false)().(agentResultMsg))
	if model.agent.history != "before" || strings.Count(strings.Join(model.pendingTranscript, "\n"), "[agent] > n") != 1 || model.agent.status != "error" {
		t.Fatalf("failed send agent=%+v queue=%q", model.agent, model.pendingTranscript)
	}
}

func TestAgentErrorWritesSafeSystemErrorToTranscriptLog(t *testing.T) {
	model := newAgentTestModel(t, agentFunc(func(context.Context, agent.Request) (agent.Result, error) {
		return agent.Result{}, errors.New("agent response malformed wait")
	}))
	model.agent.enabled = true
	logger := &fakeLogger{enabled: true}
	model.logger = logger

	model.handleAgentResult(model.wakeAgent(false)().(agentResultMsg))

	if len(logger.writes) != 1 || logger.writes[0] != "agent failed: agent response malformed wait" {
		t.Fatalf("log writes=%q", logger.writes)
	}
	if len(model.pendingTranscript) != 1 || !contains(model.pendingTranscript[0], logger.writes[0]) {
		t.Fatalf("transcript=%q log=%q", model.pendingTranscript, logger.writes)
	}
}

func TestAgentReconnectWaitsForCanceledRequestBeforeRestart(t *testing.T) {
	requests := make(chan agent.Request, 2)
	release := make(chan struct{})
	var calls int32
	model := newAgentTestModel(t, agentFunc(func(_ context.Context, request agent.Request) (agent.Result, error) {
		call := atomic.AddInt32(&calls, 1)
		requests <- request
		if call == 1 {
			<-release
			return agent.Result{Command: "stale", History: "stale"}, nil
		}
		return agent.Result{Text: "current", History: "current"}, nil
	}))
	model.agent.enabled = true
	first := model.wakeAgent(false)
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
	replacement := model.handleAgentResult((<-results).(agentResultMsg))
	if replacement == nil || len(model.session.(*fakeSession).sent) != 0 {
		t.Fatal("stale request was accepted or replacement omitted")
	}
	model.handleAgentResult(replacement().(agentResultMsg))
	if model.agent.history != "current" || len(model.session.(*fakeSession).sent) != 0 {
		t.Fatalf("replacement history=%q sent=%q", model.agent.history, model.session.(*fakeSession).sent)
	}
}

func TestTranscriptWriteFailureStopsLoggingAndQueuesSanitizedNotice(t *testing.T) {
	model := newTestModel(t, &fakeSession{updates: make(chan presentation.Update)})
	logger := &fakeLogger{enabled: true, writeErr: errors.New("disk full"), stopErr: errors.New("sync failed")}
	model.logger = logger
	model.logState = logOn
	model.applySessionUpdate(presentation.Update{Connection: presentation.Ready, Entries: []presentation.Entry{{Pane: presentation.Game, Text: "line", Operation: presentation.Append}}})
	if model.logState != logFailed || logger.stopCalls != 1 || !contains(strings.Join(model.pendingTranscript, "\n"), "logging failed: disk full (close failed: sync failed)") {
		t.Fatalf("state=%v stops=%d queue=%v", model.logState, logger.stopCalls, model.pendingTranscript)
	}
}

func TestAgentSupersessionClonesWhispersAndRestartsAfterCancellation(t *testing.T) {
	calls := make(chan agent.Request, 2)
	stepper := agentFunc(func(ctx context.Context, request agent.Request) (agent.Result, error) {
		calls <- request
		<-ctx.Done()
		return agent.Result{}, ctx.Err()
	})
	model := newAgentTestModel(t, stepper)
	model.agent.enabled = true
	model.agent.whispers = []string{"first"}
	first := model.wakeAgent(false)
	results := make(chan any, 1)
	go func() { results <- first() }()
	request := <-calls
	model.agent.whispers[0] = "changed"
	model.agent.whispers = append(model.agent.whispers, "second")
	if request.Whispers[0] != "first" {
		t.Fatalf("request aliased state: %#v", request)
	}
	if cmd := model.wakeAgent(false); cmd != nil {
		t.Fatal("overlapping request")
	}
	replacement := model.handleAgentResult((<-results).(agentResultMsg))
	if replacement == nil {
		t.Fatal("missing replacement")
	}
	go func() { results <- replacement() }()
	next := <-calls
	if len(next.Whispers) != 2 || next.Whispers[1] != "second" {
		t.Fatalf("replacement=%#v", next)
	}
	model.cancelAgent()
	<-results
}

func TestAgentCancellationAndLateResultAreIgnored(t *testing.T) {
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	stepper := agentFunc(func(ctx context.Context, _ agent.Request) (agent.Result, error) {
		started <- ctx
		<-release
		return agent.Result{Command: "north", History: "late"}, nil
	})
	model := newAgentTestModel(t, stepper)
	model.agent.enabled = true
	command := model.wakeAgent(false)
	result := make(chan any, 1)
	go func() { result <- command() }()
	ctx := <-started
	model.cancelAgent()
	if ctx.Err() == nil {
		t.Fatal("request not canceled")
	}
	close(release)
	model.handleAgentResult((<-result).(agentResultMsg))
	if len(model.session.(*fakeSession).sent) != 0 || model.agent.history == "late" {
		t.Fatalf("late result accepted: %#v", model.agent)
	}
}

func TestStaleAgentResultDoesNotClearReplacementCancellation(t *testing.T) {
	started := make(chan int, 2)
	releaseFirst := make(chan struct{})
	var calls int32
	model := newAgentTestModel(t, agentFunc(func(ctx context.Context, _ agent.Request) (agent.Result, error) {
		call := int(atomic.AddInt32(&calls, 1))
		started <- call
		if call == 1 {
			<-releaseFirst
			return agent.Result{Text: "stale", History: "stale"}, nil
		}
		<-ctx.Done()
		return agent.Result{}, ctx.Err()
	}))
	model.agent.enabled = true
	first := model.wakeAgent(false)
	firstResult := make(chan tea.Msg, 1)
	go func() { firstResult <- first() }()
	if call := <-started; call != 1 {
		t.Fatalf("first call=%d", call)
	}
	model.toggleAgent()
	model.toggleAgent()
	model.input.SetValue("resume")
	if second := model.whisper(); second != nil {
		t.Fatal("replacement started before canceled request returned")
	}
	close(releaseFirst)
	firstMessage := (<-firstResult).(agentResultMsg)
	second := model.handleAgentResult(firstMessage)
	if second == nil {
		t.Fatal("canceled request did not restart for whisper")
	}
	secondResult := make(chan tea.Msg, 1)
	go func() { secondResult <- second() }()
	if call := <-started; call != 2 {
		t.Fatalf("replacement call=%d", call)
	}
	model.handleAgentResult(firstMessage)
	if model.agent.cancel == nil {
		t.Fatal("stale result cleared replacement cancellation")
	}
	if command := model.wakeAgent(false); command != nil || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("overlapping wake command=%v calls=%d", command, calls)
	}
	model.cancelAgent()
	<-secondResult
}

func TestPublicAgentCancellationPaths(t *testing.T) {
	for _, test := range []struct {
		name          string
		clearWhispers bool
		trigger       func(*EnhancedModel)
	}{
		{"F6", true, func(model *EnhancedModel) {
			updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyF6})
			*model = updated.(EnhancedModel)
		}},
		{"reconnect", true, func(model *EnhancedModel) {
			updated, _ := model.Update(presentation.Update{Connection: presentation.Reconnecting})
			*model = updated.(EnhancedModel)
		}},
		{"source close", true, func(model *EnhancedModel) {
			updated, _ := model.Update(sessionClosedMsg{})
			*model = updated.(EnhancedModel)
		}},
		{"Ctrl-C", true, func(model *EnhancedModel) {
			updated, _ := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			*model = updated.(EnhancedModel)
		}},
		{"Close", false, func(model *EnhancedModel) { _ = model.Close() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			started := make(chan context.Context, 1)
			release := make(chan struct{})
			released := false
			releaseWorker := func() {
				if !released {
					close(release)
					released = true
				}
			}
			t.Cleanup(releaseWorker)
			model := newAgentTestModel(t, agentFunc(func(ctx context.Context, _ agent.Request) (agent.Result, error) {
				started <- ctx
				<-release
				return agent.Result{Command: "north", History: "late"}, nil
			}))
			model.agent.enabled = true
			model.agent.whispers = []string{"pending"}
			command := model.wakeAgent(false)
			result := make(chan tea.Msg, 1)
			go func() { result <- command() }()
			ctx := <-started
			test.trigger(&model)
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("agent cancellation error=%v whispers=%v", ctx.Err(), model.agent.whispers)
			}
			if test.clearWhispers && len(model.agent.whispers) != 0 {
				t.Fatalf("agent cancellation kept whispers=%v", model.agent.whispers)
			}
			releaseWorker()
			updated, next := model.Update(<-result)
			model = updated.(EnhancedModel)
			if next != nil || len(model.session.(*fakeSession).sent) != 0 || model.agent.history == "late" {
				t.Fatalf("late result accepted: sent=%v history=%q", model.session.(*fakeSession).sent, model.agent.history)
			}
		})
	}
}

func TestAgentRecentContextAndErrorRemainBoundedAndUnstyled(t *testing.T) {
	var request agent.Request
	stepper := agentFunc(func(_ context.Context, got agent.Request) (agent.Result, error) {
		request = got
		return agent.Result{Text: "ok", History: "ok"}, nil
	})
	useANSI256(t)
	model := newAgentTestModel(t, stepper)
	model.applySessionUpdate(presentation.Update{Connection: presentation.Ready, Entries: []presentation.Entry{{Pane: presentation.Game, Text: "Goblin just arrived.", Operation: presentation.Append}, {Pane: presentation.Familiar, Text: "Familiar speaks.", Operation: presentation.Append}}})
	if !contains(strings.Join(model.pendingTranscript, "\n"), "\x1b[") {
		t.Fatalf("presentation text was not styled: %q", model.pendingTranscript)
	}
	model.appendSystem("local")
	model.agent.enabled = true
	model.handleAgentResult(model.wakeAgent(false)().(agentResultMsg))
	if !contains(request.Recent, "Goblin just arrived.") || !contains(request.Recent, "Familiar speaks.") || contains(request.Recent, "local") || strings.Contains(request.Recent, "\x1b") {
		t.Fatalf("recent=%q", request.Recent)
	}
	model.addRecent(strings.Repeat("界", recentLimit))
	if len(model.agent.recent) > recentLimit || !utf8.ValidString(model.agent.recent) {
		t.Fatalf("recent bytes=%d valid=%v", len(model.agent.recent), utf8.ValidString(model.agent.recent))
	}
	errorModel := newAgentTestModel(t, agentFunc(func(context.Context, agent.Request) (agent.Result, error) {
		return agent.Result{}, errors.New(strings.Repeat("x", 300) + "\nsecret\x1b[31m")
	}))
	errorModel.agent.enabled = true
	errorModel.handleAgentResult(errorModel.wakeAgent(false)().(agentResultMsg))
	line := errorModel.pendingTranscript[len(errorModel.pendingTranscript)-1]
	if errorModel.agent.status != "error" || strings.ContainsAny(line, "\n\x1b") || len([]rune(strings.TrimPrefix(line, "[system 01:02:03] agent failed: "))) != 256 {
		t.Fatalf("line=%q status=%q", line, errorModel.agent.status)
	}
}
