package ui

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"dr-charm/internal/agent"
	"dr-charm/internal/presentation"
	"dr-charm/internal/terminaltext"
)

const recentLimit = 16 * 1024

type agentStepper interface {
	Step(context.Context, agent.Request) (agent.Result, error)
}

type agentState struct {
	client          agentStepper
	ctx             context.Context
	enabled         bool
	status          string
	history, recent string
	whispers        []string
	cancel          context.CancelFunc
	generation      uint64
	restart         bool
}

type agentResultMsg struct {
	generation uint64
	result     agent.Result
	err        error
}

func (m *EnhancedModel) toggleAgent() {
	if m.agent.client == nil {
		m.appendSystem("agent is not configured")
		return
	}
	if m.agent.enabled {
		m.agent.enabled = false
		m.cancelAgent()
		return
	}
	m.agent.enabled = true
	m.agent.status = "idle"
}

func (m *EnhancedModel) cancelAgent() {
	if m.agent.cancel != nil {
		m.agent.cancel()
	}
	m.agent.generation++
	m.agent.restart = false
	m.agent.whispers = nil
	if m.agent.enabled {
		m.agent.status = "idle"
	}
}

func (m *EnhancedModel) wakeAgent() tea.Cmd {
	if !m.agent.enabled || m.snapshot.Connection != presentation.Ready {
		return nil
	}
	if m.agent.cancel != nil {
		m.agent.restart = true
		m.agent.generation++
		m.agent.cancel()
		return nil
	}
	m.agent.generation++
	generation := m.agent.generation
	ctx, cancel := context.WithCancel(m.agent.ctx)
	m.agent.cancel = cancel
	m.agent.status = "thinking"
	request := agent.Request{History: m.agent.history, Recent: m.agent.recent, Whispers: append([]string(nil), m.agent.whispers...)}
	return func() tea.Msg {
		defer cancel()
		result, err := m.agent.client.Step(ctx, request)
		if cause := ctx.Err(); cause != nil {
			err = cause
		}
		return agentResultMsg{generation: generation, result: result, err: err}
	}
}

func (m *EnhancedModel) handleAgentResult(message agentResultMsg) tea.Cmd {
	stale := message.generation != m.agent.generation
	m.agent.cancel = nil
	if stale {
		if m.agent.restart && m.agent.enabled && m.snapshot.Connection == presentation.Ready {
			m.agent.restart = false
			return m.wakeAgent()
		}
		return nil
	}
	if !m.agent.enabled || m.snapshot.Connection != presentation.Ready {
		return nil
	}
	if message.err != nil {
		if !errors.Is(message.err, context.Canceled) {
			m.agent.status = "error"
			m.appendSystem("agent failed: " + safeAgentText(message.err.Error()))
		}
		return nil
	}
	if message.result.Command != "" {
		if !m.sendCommand(message.result.Command, "[agent] > ", false) {
			m.agent.status = "error"
			return nil
		}
	} else {
		m.appendPane(paneMain, "[agent] "+strings.ReplaceAll(message.result.Text, "\n", "\n[agent] "))
	}
	m.agent.history = message.result.History
	m.agent.whispers = nil
	m.agent.status = "idle"
	return nil
}

func (m *EnhancedModel) whisper() tea.Cmd {
	value := m.input.Value()
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if m.snapshot.Connection != presentation.Ready {
		m.appendSystem("agent waits until the connection is ready")
		return nil
	}
	m.agent.whispers = append(m.agent.whispers, value)
	m.appendPane(paneMain, "[whisper] "+value)
	m.input.Reset()
	return m.wakeAgent()
}

func (m *EnhancedModel) addRecent(text string) {
	if text == "" {
		return
	}
	m.agent.recent += text + "\n"
	if len(m.agent.recent) <= recentLimit {
		return
	}
	m.agent.recent = m.agent.recent[len(m.agent.recent)-recentLimit:]
	for !utf8.ValidString(m.agent.recent) {
		m.agent.recent = m.agent.recent[1:]
	}
}

func safeAgentText(text string) string {
	text = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(terminaltext.Sanitize(text))
	runes := []rune(text)
	if len(runes) > 256 {
		runes = runes[:256]
	}
	return string(runes)
}
