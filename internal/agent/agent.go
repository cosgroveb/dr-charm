package agent

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"dr-charm/internal/terminaltext"
)

const (
	historyLimit = 32 * 1024
	outputLimit  = 8 * 1024
	bodyLimit    = 1 << 20
)

//go:embed prompt.txt
var instructions string

type Config struct{ Endpoint, APIKey, Model, Character string }
type Request struct {
	History, Recent string
	Whispers        []string
}
type Result struct{ Command, Text, History string }

type Client struct {
	config Config
	http   *http.Client
}

func New(config Config) *Client {
	return &Client{config: config, http: &http.Client{Timeout: 240 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type wireRequest struct {
	Model             string `json:"model"`
	Instructions      string `json:"instructions"`
	Input             string `json:"input"`
	Tools             []tool `json:"tools,omitempty"`
	ParallelToolCalls bool   `json:"parallel_tool_calls"`
	Store             bool   `json:"store"`
	MaxOutputTokens   int    `json:"max_output_tokens"`
}
type tool struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Strict      bool   `json:"strict"`
	Parameters  any    `json:"parameters"`
}
type wireResponse struct {
	Status string       `json:"status"`
	Output []outputItem `json:"output"`
}
type outputItem struct {
	Type, Name, Arguments string
	Content               []content
}
type content struct{ Type, Text string }

func (c *Client) Step(ctx context.Context, request Request) (Result, error) {
	history := request.History
	if len(history) > historyLimit {
		response, err := c.call(ctx, wireRequest{Model: c.config.Model, Instructions: "Summarize only the accumulated player-agent history. Preserve facts needed to continue play.", Input: input(history, request.Recent, nil), Store: false, MaxOutputTokens: 512})
		if err != nil {
			return Result{}, fmt.Errorf("agent compaction failed: %w", err)
		}
		summary, err := textResult(response)
		if err != nil {
			return Result{}, fmt.Errorf("agent compaction failed: %w", err)
		}
		label := "Earlier play:\n"
		history = label + truncate(summary, outputLimit-len(label))
	}
	response, err := c.call(ctx, wireRequest{
		Model: c.config.Model, Instructions: instructions + "\nCharacter:\n" + c.config.Character,
		Input: input(history, request.Recent, request.Whispers), Tools: []tool{commandTool()},
		ParallelToolCalls: false, Store: false, MaxOutputTokens: 512,
	})
	if err != nil {
		return Result{}, err
	}
	result, err := result(response)
	if err != nil {
		return Result{}, err
	}
	var records []string
	for _, whisper := range request.Whispers {
		records = append(records, "Player whispered: "+terminaltext.Sanitize(whisper))
	}
	if result.Command != "" {
		records = append(records, "Agent chose: "+result.Command)
	} else {
		records = append(records, "Agent replied: "+result.Text)
	}
	result.History = strings.TrimSpace(strings.Join([]string{history, strings.Join(records, "\n")}, "\n"))
	return result, nil
}

func commandTool() tool {
	return tool{Type: "function", Name: "send_command", Description: "Send one DragonRealms command.", Strict: true, Parameters: map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"command"},
		"properties": map[string]any{"command": map[string]any{"type": "string"}},
	}}
}

func input(history, recent string, whispers []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "PLAYER-AGENT HISTORY\n%s\n\nRECENT GAME TEXT\n%s", history, recent)
	if len(whispers) > 0 {
		b.WriteString("\n\nCURRENT WHISPERS")
		for _, whisper := range whispers {
			b.WriteString("\n- " + terminaltext.Sanitize(whisper))
		}
	}
	return b.String()
}

func (c *Client) call(ctx context.Context, payload wireRequest) (wireResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return wireResponse{}, errors.New("agent request invalid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return wireResponse{}, errors.New("agent request invalid")
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return wireResponse{}, requestError(ctx, err, "agent transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return wireResponse{}, fmt.Errorf("agent HTTP status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, bodyLimit+1))
	if err != nil {
		return wireResponse{}, requestError(ctx, err, "agent response unreadable")
	}
	if len(data) > bodyLimit {
		return wireResponse{}, errors.New("agent response too large")
	}
	if !utf8.Valid(data) {
		return wireResponse{}, errors.New("agent response invalid")
	}
	var decoded wireResponse
	decoder := json.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&decoded) != nil || decoder.Decode(&struct{}{}) != io.EOF || decoded.Status != "completed" {
		return wireResponse{}, errors.New("agent response invalid")
	}
	return decoded, nil
}

func requestError(ctx context.Context, err error, fallback string) error {
	if ctx.Err() != nil {
		return fmt.Errorf("agent request canceled: %w", ctx.Err())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("agent request timed out: %w", context.DeadlineExceeded)
	}
	return errors.New(fallback)
}

func result(response wireResponse) (Result, error) {
	text, calls, invalid := collect(response)
	if invalid || len(calls) > 1 {
		return Result{}, errors.New("agent response invalid")
	}
	if len(calls) == 0 {
		text = terminaltext.Sanitize(text)
		if strings.TrimSpace(text) == "" || len(text) > outputLimit {
			return Result{}, errors.New("agent response invalid")
		}
		return Result{Text: text}, nil
	}
	call := calls[0]
	if call.Name != "send_command" {
		return Result{}, errors.New("agent response invalid")
	}
	command, ok := commandArgument(call.Arguments)
	if !ok || !utf8.ValidString(command) || len(command) > 4096 || strings.ContainsAny(command, "\r\n\x00") {
		return Result{}, errors.New("agent response invalid")
	}
	if terminaltext.Sanitize(command) != command || strings.TrimSpace(command) == "" {
		return Result{}, errors.New("agent response invalid")
	}
	return Result{Command: command}, nil
}

func commandArgument(raw string) (string, bool) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') || !decoder.More() {
		return "", false
	}
	key, err := decoder.Token()
	var command string
	if err != nil || key != "command" || decoder.Decode(&command) != nil || decoder.More() {
		return "", false
	}
	token, err = decoder.Token()
	return command, err == nil && token == json.Delim('}') && decoder.Decode(&struct{}{}) == io.EOF
}

func textResult(response wireResponse) (string, error) {
	text, calls, invalid := collect(response)
	text = terminaltext.Sanitize(text)
	if invalid || len(calls) > 0 || strings.TrimSpace(text) == "" {
		return "", errors.New("agent response invalid")
	}
	return text, nil
}

func collect(response wireResponse) (string, []outputItem, bool) {
	var texts []string
	var calls []outputItem
	invalid := false
	textSeen := false
	for _, item := range response.Output {
		switch item.Type {
		case "function_call":
			invalid = invalid || len(item.Content) > 0
			calls = append(calls, item)
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" {
					textSeen = true
					texts = append(texts, part.Text)
				} else {
					invalid = true
				}
			}
		case "reasoning":
		default:
			invalid = true
		}
	}
	return strings.Join(texts, ""), calls, invalid || textSeen && len(calls) > 0
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
