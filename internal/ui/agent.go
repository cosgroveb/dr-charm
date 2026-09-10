package ui

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"dr-charm/internal/agent"
	"dr-charm/internal/presentation"
	"dr-charm/internal/terminaltext"
)

const recentLimit = 16 * 1024

type agentStepper interface {
	Step(context.Context, agent.Request) (agent.Result, error)
}

type agentState struct {
	client           agentStepper
	ctx              context.Context
	enabled          bool
	status           string
	history, recent  string
	whispers         []string
	cancel           context.CancelFunc
	cancelGeneration uint64
	generation       uint64
	restart          bool
	phase            agentPhase
}

type agentPhase uint8

const (
	agentActive agentPhase = iota
	agentWaitingForGameEvent
	agentWaitingForPlayer
)

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
	m.agent.phase = agentActive
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
		m.agent.status = statusForAgentPhase(m.agent.phase)
	}
}

func statusForAgentPhase(phase agentPhase) string {
	switch phase {
	case agentWaitingForGameEvent:
		return "waiting"
	case agentWaitingForPlayer:
		return "paused"
	default:
		return "idle"
	}
}

func (m *EnhancedModel) wakeAgent(prompt bool) tea.Cmd {
	if !m.agent.enabled || m.snapshot.Connection != presentation.Ready {
		return nil
	}
	if prompt && m.agent.phase == agentWaitingForPlayer {
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
	m.agent.cancelGeneration = generation
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
	if m.agent.cancelGeneration == message.generation {
		m.agent.cancel = nil
		m.agent.cancelGeneration = 0
	}
	if stale {
		if m.agent.restart && m.agent.enabled && m.snapshot.Connection == presentation.Ready {
			m.agent.restart = false
			return m.wakeAgent(false)
		}
		return nil
	}
	if !m.agent.enabled || m.snapshot.Connection != presentation.Ready {
		return nil
	}
	if message.err != nil {
		if !errors.Is(message.err, context.Canceled) {
			m.agent.status = "error"
			text := "agent failed: " + safeAgentText(message.err.Error())
			m.appendSystem(text)
			m.writeLog(text)
		}
		return nil
	}
	if message.result.Command != "" {
		if !m.sendCommand(message.result.Command, "[agent] > ", false) {
			m.agent.status = "error"
			return nil
		}
	} else if message.result.Text != "" {
		m.appendAgentMessage("[agent]", message.result.Text)
	}
	m.agent.history = message.result.History
	m.agent.whispers = nil
	switch message.result.WaitUntil {
	case agent.WaitForGameEvent:
		m.agent.phase = agentWaitingForGameEvent
		m.agent.status = "waiting"
	case agent.WaitForPlayer:
		m.agent.phase = agentWaitingForPlayer
		m.agent.status = "paused"
	default:
		m.agent.phase = agentActive
		m.agent.status = "idle"
	}
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
	m.appendAgentMessage("[whisper]", value)
	m.input.Reset()
	return m.wakeAgent(false)
}

func (m *EnhancedModel) appendAgentMessage(label, text string) {
	badge := lipgloss.NewStyle().Bold(true).Reverse(true).Render(label)
	body := lipgloss.NewStyle().Bold(true)
	lines := splitLines(text)
	for index := range lines {
		lines[index] = badge + " " + body.Render(lines[index])
	}
	m.enqueueTranscript(strings.Join(lines, "\n"))
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
