package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStepSendsResponsesRequestAndReturnsTextHistory(t *testing.T) {
	var got wireRequest
	var raw string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("request path=%q method=%q headers=%v", r.URL.Path, r.Method, r.Header)
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		raw = string(data)
		if err := json.Unmarshal(data, &got); err != nil {
			t.Error(err)
		}
		io.WriteString(w, completedText("Hold position."))
	}))
	defer server.Close()

	client := New(Config{Endpoint: server.URL + "/v1/responses", APIKey: "secret", Model: "local", Character: "A cautious Moon Mage."})
	request := Request{History: "Earlier record", Recent: "A goblin arrives.", Whispers: []string{"stay safe"}}
	result, err := client.Step(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "Hold position." || result.Command != "" || !strings.Contains(result.History, "Player whispered: stay safe\nAgent replied: Hold position.") {
		t.Fatalf("result=%+v", result)
	}
	if request.History != "Earlier record" || len(request.Whispers) != 1 {
		t.Fatalf("request mutated: %+v", request)
	}
	if got.Model != "local" || got.Store || got.MaxOutputTokens != 512 || got.ParallelToolCalls || len(got.Tools) != 1 || got.Tools[0].Name != "send_command" || !got.Tools[0].Strict {
		t.Fatalf("wire request=%+v", got)
	}
	for _, field := range []string{`"store":false`, `"parallel_tool_calls":false`, `"max_output_tokens":512`, `"additionalProperties":false`, `"required":["command"]`} {
		if !strings.Contains(raw, field) {
			t.Fatalf("request body omits %s: %s", field, raw)
		}
	}
	if !strings.Contains(got.Instructions, "A cautious Moon Mage.") || !strings.Contains(got.Input, "Earlier record") || !strings.Contains(got.Input, "A goblin arrives.") || !strings.Contains(got.Input, "stay safe") {
		t.Fatalf("instructions/input missing data: %+v", got)
	}
}

func TestStepOmitsAuthorizationAndReturnsStrictCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if value, ok := r.Header["Authorization"]; ok {
			t.Errorf("authorization present: %q", value)
		}
		io.WriteString(w, completedCall("send_command", `{"command":"look"}`))
	}))
	defer server.Close()
	result, err := New(Config{Endpoint: server.URL, Model: "m", Character: "c"}).Step(context.Background(), Request{})
	if err != nil || result.Command != "look" || result.History != "Agent chose: look" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestStepRejectsInvalidOutcomesWithoutHistory(t *testing.T) {
	tests := map[string]string{
		"failed status":            `{"status":"failed","output":[{"type":"message","content":[{"type":"output_text","text":"act"}]}]}`,
		"incomplete status":        `{"status":"incomplete","output":[{"type":"message","content":[{"type":"output_text","text":"act"}]}]}`,
		"mixed":                    `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"act"}]},{"type":"function_call","name":"send_command","arguments":"{\"command\":\"look\"}"}]}`,
		"blank text and call":      `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":""}]},{"type":"function_call","name":"send_command","arguments":"{\"command\":\"look\"}"}]}`,
		"multiple calls":           `{"status":"completed","output":[{"type":"function_call","name":"send_command","arguments":"{\"command\":\"look\"}"},{"type":"function_call","name":"send_command","arguments":"{\"command\":\"north\"}"}]}`,
		"call with text content":   `{"status":"completed","output":[{"type":"function_call","name":"send_command","arguments":"{\"command\":\"look\"}","content":[{"type":"output_text","text":"hidden"}]}]}`,
		"call with refusal":        `{"status":"completed","output":[{"type":"function_call","name":"send_command","arguments":"{\"command\":\"look\"}","content":[{"type":"refusal","refusal":"hidden"}]}]}`,
		"unknown call":             completedCall("other", `{"command":"look"}`),
		"uppercase argument":       completedCall("send_command", `{"Command":"look"}`),
		"duplicate argument":       completedCall("send_command", `{"command":"look","command":"north"}`),
		"unknown argument":         completedCall("send_command", `{"command":"look","other":1}`),
		"missing argument":         completedCall("send_command", `{}`),
		"extra JSON":               completedCall("send_command", `{"command":"look"} {}`),
		"blank command":            completedCall("send_command", `{"command":" "}`),
		"multiline command":        completedCall("send_command", `{"command":"look\nnorth"}`),
		"carriage return command":  completedCall("send_command", `{"command":"look\rnorth"}`),
		"nul command":              completedCall("send_command", `{"command":"look\u0000"}`),
		"control command":          completedCall("send_command", `{"command":"\u001b[31m"}`),
		"embedded control command": completedCall("send_command", `{"command":"north\u001b[31msouth"}`),
		"refusal":                  `{"status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"no"}]}]}`,
		"unknown content":          `{"status":"completed","output":[{"type":"message","content":[{"type":"other","text":"act"}]}]}`,
		"unknown output":           `{"status":"completed","output":[{"type":"other"},{"type":"message","content":[{"type":"output_text","text":"act"}]}]}`,
		"blank text":               completedText(" \t"),
		"control text":             completedText("\u001b[31m"),
		"empty":                    `{"status":"completed","output":[]}`,
		"malformed":                `{`,
		"second document":          completedText("act") + `{}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			client, closeServer := clientReturning(t, body, http.StatusOK)
			defer closeServer()
			result, err := client.Step(context.Background(), Request{History: "unchanged", Whispers: []string{"one"}})
			if err == nil || result != (Result{}) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestStepRejectsBoundsAndUnsafeBytes(t *testing.T) {
	tests := map[string]string{
		"large text":    completedText(strings.Repeat("x", outputLimit+1)),
		"large command": completedCall("send_command", fmt.Sprintf(`{"command":%q}`, strings.Repeat("x", 4097))),
		"invalid UTF-8": "{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"" + string([]byte{0xff}) + "\"}]}]}",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			client, closeServer := clientReturning(t, body, http.StatusOK)
			defer closeServer()
			if _, err := client.Step(context.Background(), Request{}); err == nil {
				t.Fatal("unsafe result accepted")
			}
		})
	}
	large := strings.Repeat("x", bodyLimit+1)
	client, closeServer := clientReturning(t, large, http.StatusOK)
	defer closeServer()
	if _, err := client.Step(context.Background(), Request{}); err == nil || err.Error() != "agent response too large" {
		t.Fatalf("large body error=%v", err)
	}
}

func TestStepCompactsHistoryWithSameRecentContext(t *testing.T) {
	var mu sync.Mutex
	var requests []wireRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request wireRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		mu.Lock()
		requests = append(requests, request)
		count := len(requests)
		mu.Unlock()
		if count == 1 {
			io.WriteString(w, completedText(strings.Repeat("s", outputLimit)))
			return
		}
		io.WriteString(w, completedText("Ready."))
	}))
	defer server.Close()
	history := strings.Repeat("old", historyLimit/3+1)
	result, err := New(Config{Endpoint: server.URL, Model: "m", Character: "c"}).Step(context.Background(), Request{History: history, Recent: "room", Whispers: []string{"go"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || len(requests[0].Tools) != 0 || len(requests[1].Tools) != 1 || !strings.Contains(requests[0].Input, "room") || !strings.Contains(requests[1].Input, "room") {
		t.Fatalf("requests=%+v", requests)
	}
	if !strings.Contains(requests[0].Input, history) || strings.Contains(requests[1].Input, history) || !strings.Contains(requests[1].Input, "Earlier play:\n") {
		t.Fatal("history was not replaced before normal request")
	}
	if !strings.HasPrefix(result.History, "Earlier play:\n") || strings.Contains(result.History, "room") {
		t.Fatalf("returned history=%q", result.History)
	}
	if got := strings.Index(result.History, "Player whispered: go"); got != outputLimit+1 {
		t.Fatalf("summary record size or whisper order wrong: index=%d", got)
	}
}

func TestStepCompactsOnlyAboveThreshold(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		io.WriteString(w, completedText("ok"))
	}))
	defer server.Close()
	client := New(Config{Endpoint: server.URL, Model: "m", Character: "c"})
	if _, err := client.Step(context.Background(), Request{History: strings.Repeat("x", historyLimit)}); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests at threshold=%d", requests)
	}
}

func TestStepCompactionFailureReturnsNoReplacementHistory(t *testing.T) {
	client, closeServer := clientReturning(t, `{`, http.StatusOK)
	defer closeServer()
	result, err := client.Step(context.Background(), Request{History: strings.Repeat("x", historyLimit+1)})
	if err == nil || !strings.Contains(err.Error(), "agent compaction failed") || result != (Result{}) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestStepActionFailureAfterCompactionReturnsNoReplacementHistory(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			io.WriteString(w, completedText("summary"))
			return
		}
		io.WriteString(w, `{`)
	}))
	defer server.Close()
	result, err := New(Config{Endpoint: server.URL, Model: "m", Character: "c"}).Step(context.Background(), Request{History: strings.Repeat("x", historyLimit+1)})
	if requests != 2 || err == nil || result != (Result{}) {
		t.Fatalf("requests=%d result=%+v err=%v", requests, result, err)
	}
}

func TestStepSanitizesAcceptedOutput(t *testing.T) {
	body := `{"status":"completed","output":[{"type":"reasoning"},{"type":"message","content":[{"type":"output_text","text":"safe\u001b[31m text"}]}]}`
	client, closeServer := clientReturning(t, body, http.StatusOK)
	defer closeServer()
	result, err := client.Step(context.Background(), Request{})
	if err != nil || result.Text != "safe text" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestStepReportsOnlyBoundedHTTPAndTransportErrors(t *testing.T) {
	client, closeServer := clientReturning(t, "secret response", http.StatusTeapot)
	defer closeServer()
	if _, err := client.Step(context.Background(), Request{}); err == nil || err.Error() != "agent HTTP status 418" || strings.Contains(err.Error(), "secret") {
		t.Fatalf("HTTP error=%v", err)
	}
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := closed.URL + "/private/responses"
	closed.Close()
	client = New(Config{Endpoint: endpoint, APIKey: "secret", Model: "m", Character: "c"})
	if _, err := client.Step(context.Background(), Request{Recent: "game secret"}); err == nil || err.Error() != "agent transport failed" {
		t.Fatalf("transport error=%v", err)
	}
}

func TestStepDoesNotFollowRedirects(t *testing.T) {
	t.Run("same host", func(t *testing.T) {
		redirected := 0
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/target" {
				redirected++
				return
			}
			w.Header().Set("Location", server.URL+"/target")
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		defer server.Close()
		client := New(Config{Endpoint: server.URL + "/responses", APIKey: "secret", Model: "m", Character: "c"})
		if _, err := client.Step(context.Background(), Request{Recent: "game text"}); err == nil || err.Error() != "agent HTTP status 307" {
			t.Fatalf("redirect error=%v", err)
		}
		if redirected != 0 {
			t.Fatalf("redirect target received %d requests", redirected)
		}
	})

	t.Run("cross host", func(t *testing.T) {
		redirected := 0
		target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected++ }))
		defer target.Close()
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", target.URL)
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		defer source.Close()
		client := New(Config{Endpoint: source.URL, APIKey: "secret", Model: "m", Character: "c"})
		if _, err := client.Step(context.Background(), Request{Recent: "game text"}); err == nil || err.Error() != "agent HTTP status 307" {
			t.Fatalf("redirect error=%v", err)
		}
		if redirected != 0 {
			t.Fatalf("redirect target received %d requests", redirected)
		}
	})
}

func TestStepHonorsCancellationAndClientTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	client := New(Config{Endpoint: server.URL, Model: "m", Character: "c"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := client.Step(ctx, Request{History: "unchanged"}); done <- err }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}

	started = make(chan struct{})
	client.http.Timeout = time.Millisecond
	if _, err := client.Step(context.Background(), Request{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error=%v", err)
	}
}

func TestStepPreservesCancellationWhileReadingResponseBody(t *testing.T) {
	for _, test := range []struct {
		name    string
		timeout bool
		want    error
	}{
		{name: "caller cancellation", want: context.Canceled},
		{name: "client timeout", timeout: true, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				close(started)
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			client := New(Config{Endpoint: server.URL, Model: "m", Character: "c"})
			ctx, cancel := context.WithCancel(context.Background())
			if test.timeout {
				client.http.Timeout = 100 * time.Millisecond
			}
			done := make(chan error, 1)
			go func() { _, err := client.Step(ctx, Request{}); done <- err }()
			<-started
			if !test.timeout {
				cancel()
			}
			err := <-done
			cancel()
			close(release)
			server.Close()
			if !errors.Is(err, test.want) {
				t.Fatalf("body-read error=%v, want errors.Is %v", err, test.want)
			}
		})
	}
}

func clientReturning(t *testing.T, body string, status int) (*Client, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	return New(Config{Endpoint: server.URL, Model: "m", Character: "c"}), server.Close
}

func completedText(text string) string {
	data, _ := json.Marshal(map[string]any{
		"status": "completed",
		"output": []any{map[string]any{
			"type":    "message",
			"content": []any{map[string]any{"type": "output_text", "text": text}},
		}},
	})
	return string(data)
}

func completedCall(name, arguments string) string {
	data, _ := json.Marshal(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "name": name, "arguments": arguments}}})
	return string(data)
}
