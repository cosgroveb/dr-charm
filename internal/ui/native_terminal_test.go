package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"dr-charm/internal/agent"
	"dr-charm/internal/presentation"
)

// TestNativeTerminalHelper is launched by TestNativeTerminalScrollback inside
// an isolated tmux server. It deliberately drives the real program rather than
// a viewport or a byte-stream substitute.
func TestNativeTerminalHelper(t *testing.T) {
	if os.Getenv("DR_CHARM_NATIVE_TERMINAL_HELPER") != "1" {
		return
	}
	updates := make(chan presentation.Update, 3)
	first := presentation.Update{
		Connection: presentation.Ready,
		Prompt:     ">",
		Location:   presentation.Location{Title: "[Test Hall]", Exits: []string{"north", "south"}},
		Hands:      presentation.Hands{Left: "shield", Right: "sword", PreparedSpell: "Fire"},
		Map:        presentation.Map{Lines: []string{"o---@", "    |", "    o", "    |", "    o", "    |", "    o"}, CurrentToken: "hall", CurrentLine: 0, CurrentColumn: 4},
		Entries: []presentation.Entry{
			{Pane: presentation.Game, Text: "GAME-ONE", Operation: presentation.Append},
			{Pane: presentation.Game, Text: "GAME-ONE", Operation: presentation.Append},
			{Pane: presentation.Game, Text: "", Operation: presentation.Append},
			{Pane: presentation.Familiar, Text: "FAMILIAR-ONE", Operation: presentation.Append},
			{Pane: presentation.Game, Text: "CLEAR-ME", Operation: presentation.Clear},
			{Pane: presentation.Game, Text: "REPLACE-ONE", Operation: presentation.Replace},
		},
		Notices: []presentation.Notice{{Text: "SYSTEM-ONE"}},
	}
	for number := 0; number < 36; number++ {
		first.Entries = append(first.Entries, presentation.Entry{Pane: presentation.Game, Text: fmt.Sprintf("TALL-%02d", number), Operation: presentation.Append})
	}
	first.Entries = append(first.Entries,
		presentation.Entry{Pane: presentation.Game, Text: "UNICODE-WRAP-BEGIN café 你好 👋 café 你好 👋 café 你好 👋 UNICODE-WRAP-END", Operation: presentation.Append},
		presentation.Entry{Pane: presentation.Game, Text: nativeTerminalMultilineRecord(), Operation: presentation.Append},
	)
	readyUpdate := func(entries []presentation.Entry) presentation.Update {
		return presentation.Update{
			Connection: presentation.Ready,
			Prompt:     ">",
			Location:   first.Location,
			Hands:      first.Hands,
			Map:        first.Map,
			Entries:    entries,
		}
	}
	updates <- first
	session := &nativeTerminalSession{updates: updates}
	session.send = func(command string) {
		switch command {
		case "async":
			updates <- readyUpdate([]presentation.Entry{{Pane: presentation.Game, Text: "ASYNC-TWO", Operation: presentation.Append}})
		case "tail":
			tail := make([]presentation.Entry, 0, 36)
			for number := 0; number < 36; number++ {
				tail = append(tail, presentation.Entry{Pane: presentation.Game, Text: fmt.Sprintf("TAIL-%02d", number), Operation: presentation.Append})
			}
			updates <- readyUpdate(tail)
		case "boundary-seed":
			seed := make([]presentation.Entry, 0, 8)
			for number := 0; number < 8; number++ {
				seed = append(seed, presentation.Entry{Pane: presentation.Game, Text: fmt.Sprintf("BOUNDARY-SEED-%02d", number), Operation: presentation.Append})
			}
			updates <- readyUpdate(seed)
		case "boundary-advance":
			advance := make([]presentation.Entry, 0, 14)
			for number := 0; number < 14; number++ {
				advance = append(advance, presentation.Entry{Pane: presentation.Game, Text: fmt.Sprintf("BOUNDARY-ADVANCE-%02d", number), Operation: presentation.Append})
			}
			updates <- readyUpdate(advance)
		case "flush":
			flush := make([]presentation.Entry, 0, 36)
			for number := 0; number < 36; number++ {
				flush = append(flush, presentation.Entry{Pane: presentation.Game, Text: fmt.Sprintf("FLUSH-%02d", number), Operation: presentation.Append})
			}
			updates <- readyUpdate(flush)
			close(updates)
		}
	}
	model := InitialEnhancedModel(session, Options{Context: context.Background()})
	model.agent.client = nativeTerminalAgent{}
	model.now = func() time.Time { return time.Date(2026, 1, 1, 1, 2, 3, 0, time.UTC) }
	program := tea.NewProgram(model)
	if _, err := program.Run(); err != nil {
		t.Fatal(err)
	}
}

type nativeTerminalAgent struct{}

func (nativeTerminalAgent) Step(context.Context, agent.Request) (agent.Result, error) {
	return agent.Result{Text: "AGENT-ONE"}, nil
}

type nativeTerminalSession struct {
	updates chan presentation.Update
	send    func(string)
}

func (session *nativeTerminalSession) Send(command string) error {
	session.send(command)
	return nil
}

func (session *nativeTerminalSession) Next() (presentation.Update, bool) {
	update, ok := <-session.updates
	return update, ok
}

func TestNativeTerminalScrollback(t *testing.T) {
	if os.Getenv("DR_CHARM_NATIVE_TERMINAL_HELPER") == "1" {
		return
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatalf("tmux is required for terminal-state proof: %v", err)
	}
	server := fmt.Sprintf("dr-charm-native-%d", os.Getpid())
	session := "proof"
	tmux := func(args ...string) ([]byte, error) {
		return exec.Command("tmux", append([]string{"-L", server}, args...)...).CombinedOutput()
	}
	t.Cleanup(func() { _, _ = tmux("kill-server") })
	editorReady := filepath.Join(t.TempDir(), "editor-ready")
	editor := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf 'EDITOR-ONE' > \"$1\"\n: > \"$DR_CHARM_EDITOR_READY\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(t.TempDir(), "native-ui.test")
	dependency, err := exec.Command("go", "list", "-m", "-f", "{{with .Replace}}{{.Path}}{{end}}", "charm.land/bubbletea/v2").CombinedOutput()
	if err != nil {
		t.Fatalf("inspect terminal helper dependency: %v\n%s", err, dependency)
	}
	if strings.TrimSpace(string(dependency)) != "./third_party/bubbletea" {
		t.Fatalf("terminal helper would not use local Bubble Tea correction: %q", dependency)
	}
	build := exec.Command("go", "test", "-c", "-o", helper)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("prebuild terminal helper: %v\n%s", err, output)
	}
	command := fmt.Sprintf("DR_CHARM_NATIVE_TERMINAL_HELPER=1 DR_CHARM_EDITOR_READY=%s EDITOR=%s %s -test.run '^TestNativeTerminalHelper$' -test.count=1; exec sh", shellQuote(editorReady), shellQuote(editor), shellQuote(helper))
	if out, err := tmux("new-session", "-d", "-x", "100", "-y", "30", "-s", session, command); err != nil {
		t.Fatalf("start isolated tmux: %v\n%s", err, out)
	}
	waitFor := func(record string) string {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			capture, err := tmux("capture-pane", "-p", "-t", session, "-S", "-1000")
			if err != nil {
				t.Fatalf("poll terminal readiness: %v\n%s", err, capture)
			}
			if strings.Contains(string(capture), record) {
				return string(capture)
			}
			if time.Now().After(deadline) {
				t.Fatalf("terminal never emitted %q\n%s", record, capture)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	waitForVisible := func(record string) string {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			capture, err := tmux("capture-pane", "-p", "-t", session)
			if err != nil {
				t.Fatalf("poll visible terminal: %v\n%s", err, capture)
			}
			if strings.Contains(string(capture), record) {
				return string(capture)
			}
			if time.Now().After(deadline) {
				t.Fatalf("visible terminal never emitted %q\n%s", record, capture)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	waitForVisibleAll := func(records ...string) string {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			capture, err := tmux("capture-pane", "-p", "-t", session)
			if err != nil {
				t.Fatalf("poll visible terminal: %v\n%s", err, capture)
			}
			visible := string(capture)
			ready := true
			for _, record := range records {
				ready = ready && strings.Contains(visible, record)
			}
			if ready {
				return visible
			}
			if time.Now().After(deadline) {
				t.Fatalf("visible terminal never emitted %q\n%s", records, capture)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	waitForVisibleWithout := func(record, absent string) string {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			capture, err := tmux("capture-pane", "-p", "-t", session)
			if err != nil {
				t.Fatalf("poll visible terminal: %v\n%s", err, capture)
			}
			if strings.Contains(string(capture), record) && !strings.Contains(string(capture), absent) {
				return string(capture)
			}
			if time.Now().After(deadline) {
				t.Fatalf("visible terminal did not contain %q without %q\n%s", record, absent, capture)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	waitForFile := func(path string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			if _, err := os.Stat(path); err == nil {
				return
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if time.Now().After(deadline) {
				t.Fatalf("editor did not signal completion")
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	send := func(keys ...string) {
		t.Helper()
		if out, err := tmux(append([]string{"send-keys", "-t", session}, keys...)...); err != nil {
			t.Fatalf("send %q: %v\n%s", keys, err, out)
		}
	}
	captureHistory := func() string {
		t.Helper()
		capture, err := tmux("capture-pane", "-p", "-t", session, "-S", "-1000", "-E", "-1")
		if err != nil {
			t.Fatalf("capture isolated terminal history: %v\n%s", err, capture)
		}
		return string(capture)
	}
	captureComplete := func() string {
		t.Helper()
		capture, err := tmux("capture-pane", "-p", "-t", session, "-S", "-1000")
		if err != nil {
			t.Fatalf("capture complete terminal state: %v\n%s", err, capture)
		}
		return string(capture)
	}
	captureCompleteJoined := func() string {
		t.Helper()
		capture, err := tmux("capture-pane", "-J", "-p", "-t", session, "-S", "-1000")
		if err != nil {
			t.Fatalf("capture joined terminal state: %v\n%s", err, capture)
		}
		return string(capture)
	}
	capturePane := func() string {
		t.Helper()
		capture, err := tmux("capture-pane", "-p", "-t", session)
		if err != nil {
			t.Fatalf("capture visible terminal pane: %v\n%s", err, capture)
		}
		return string(capture)
	}
	sequence := nativeTerminalSequence()
	assertCheckpoint := func(name, visible, absent string) {
		t.Helper()
		complete := captureComplete()
		assertOrderedOnce(t, complete, sequence)
		assertUnicodeRecord(t, captureCompleteJoined())
		history := captureHistory()
		assertNativeHistory(t, history)
		pane := capturePane()
		if !strings.Contains(pane, visible) || (absent != "" && strings.Contains(pane, absent)) {
			t.Fatalf("%s visible pane missing %q or retained %q\n%s", name, visible, absent, pane)
		}
		cursor, err := tmux("display-message", "-p", "-t", session, "#{cursor_flag}:#{cursor_x}:#{cursor_y}")
		if err != nil {
			t.Fatalf("read %s cursor state: %v\n%s", name, err, cursor)
		}
		if !strings.HasPrefix(string(cursor), "0:") {
			t.Fatalf("%s exposed a hardware cursor: %q", name, cursor)
		}
	}
	waitFor("TALL-35")
	send("F1")
	waitForVisible("Help")
	send("Escape")
	waitForVisible("READY | LOG off")
	send("F3")
	waitForVisible("Theme")
	send("Escape")
	waitForVisible("READY | LOG off")
	send("F6")
	waitForVisible("whisper>")
	send("hello-whisper", "Enter")
	waitFor("AGENT-ONE")
	send("F6")
	deadline := time.Now().Add(10 * time.Second)
	for strings.Contains(waitForVisible("READY | LOG off"), "whisper>") {
		if time.Now().After(deadline) {
			t.Fatal("agent mode remained enabled")
		}
		time.Sleep(25 * time.Millisecond)
	}
	send("async", "Enter")
	waitFor("ASYNC-TWO")
	send("tail", "Enter")
	waitForVisible("TAIL-35")
	send("boundary-seed", "Enter")
	waitFor("BOUNDARY-SEED-07")
	boundaryBefore := captureComplete()
	boundaryHistory := captureHistory()
	send("boundary-advance", "Enter")
	waitFor("BOUNDARY-ADVANCE-13")
	boundaryPane := capturePane()
	boundaryAfter := captureComplete()
	assertCaptureBoundaryRace(t, boundaryBefore, boundaryHistory, boundaryPane, boundaryAfter)
	send("flush", "Enter")
	waitFor("FLUSH-35")
	waitForVisible("DISCONNECTED")
	assertCheckpoint("before resize", "o---@", "")
	if out, err := tmux("resize-window", "-t", session, "-x", "44", "-y", "30"); err != nil {
		t.Fatalf("width-only resize: %v\n%s", err, out)
	}
	waitForVisible("o---@")
	assertCheckpoint("width-only resize", "o---@", "")
	if out, err := tmux("resize-window", "-t", session, "-x", "44", "-y", "18"); err != nil {
		t.Fatalf("below-map resize: %v\n%s", err, out)
	}
	waitForVisibleWithout("L: shield", "o---@")
	assertCheckpoint("below-map resize", "L: shield", "o---@")
	if out, err := tmux("resize-window", "-t", session, "-x", "100", "-y", "19"); err != nil {
		t.Fatalf("at-map resize: %v\n%s", err, out)
	}
	waitForVisible("o---@")
	assertCheckpoint("at-map resize", "o---@", "")
	for _, size := range [][2]string{{"100", "30"}, {"44", "20"}, {"100", "30"}} {
		if out, err := tmux("resize-window", "-t", session, "-x", size[0], "-y", size[1]); err != nil {
			t.Fatalf("rapid resize %s×%s: %v\n%s", size[0], size[1], err, out)
		}
	}
	widePane := waitForVisibleAll("o---@", "L: shield", "R: sword", "DISCONNECTED")
	assertCheckpoint("rapid resize", "o---@", "")
	if !strings.Contains(widePane, "L: shield") || !strings.Contains(widePane, "R: sword") || !strings.Contains(widePane, "DISCONNECTED") {
		t.Fatalf("wide dashboard omitted map-adjacent hands or status\n%s", widePane)
	}
	send("C-g")
	waitForFile(editorReady)
	waitForVisible("EDITOR-ONE")
	disconnectedPane := waitForVisible("DISCONNECTED")
	if !strings.Contains(disconnectedPane, "[Test Hall]") {
		t.Fatalf("source close did not retain disconnected dashboard\n%s", disconnectedPane)
	}
	assertCheckpoint("editor completion", "DISCONNECTED", "")
	activeComplete := captureComplete()
	activeHistory := captureHistory()
	if out, err := tmux("send-keys", "-t", session, "C-c"); err != nil {
		t.Fatalf("quit: %v\n%s", err, out)
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		capture, err := tmux("capture-pane", "-p", "-t", session, "-S", "-1000")
		if err != nil {
			t.Fatalf("capture post-quit terminal: %v\n%s", err, capture)
		}
		if strings.Contains(string(capture), "PASS") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Ctrl-C did not terminate the native terminal helper")
		}
		time.Sleep(25 * time.Millisecond)
	}
	postQuitComplete := captureComplete()
	postQuitHistory := captureHistory()
	if directory := os.Getenv("DR_CHARM_TERMINAL_ARTIFACT_DIR"); directory != "" {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		for name, contents := range map[string]string{"terminal-active-history.txt": activeHistory} {
			if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	assertOrderedOnce(t, activeComplete, sequence)
	assertUnicodeRecord(t, captureCompleteJoined())
	for _, record := range []string{"[familiar] FAMILIAR-ONE", "REPLACE-ONE", "ASYNC-TWO", "[whisper] hello-whisper", "AGENT-ONE"} {
		if strings.Count(activeComplete, record) != 1 {
			t.Fatalf("terminal record %q count=%d\n%s", record, strings.Count(activeHistory, record), activeHistory)
		}
	}
	if strings.Count(activeComplete, "GAME-ONE") != 2 {
		t.Fatalf("duplicate game record not retained twice\n%s", activeHistory)
	}
	assertNativeHistory(t, activeHistory)
	if !strings.Contains(activeComplete, "GAME-ONE\nGAME-ONE\n\n[familiar] FAMILIAR-ONE") {
		t.Fatalf("empty append did not create a visually blank terminal line\n%s", activeHistory)
	}
	if !strings.Contains(widePane, "o---@") {
		t.Fatalf("visible pane lost map after wide resize\n%s", widePane)
	}
	assertOrderedOnce(t, postQuitComplete, sequence)
	assertUnicodeRecord(t, captureCompleteJoined())
	assertNativeHistory(t, postQuitHistory)
	postQuitCursor, err := tmux("display-message", "-p", "-t", session, "#{cursor_flag}:#{cursor_x}:#{cursor_y}")
	if err != nil {
		t.Fatalf("read post-quit cursor state: %v\n%s", err, postQuitCursor)
	}
	if !strings.HasPrefix(string(postQuitCursor), "1:") {
		t.Fatalf("post-quit cursor was not restored: %q", postQuitCursor)
	}
}

func nativeTerminalMultilineRecord() string {
	lines := make([]string, 36)
	for number := range lines {
		lines[number] = fmt.Sprintf("MULTILINE-%02d", number)
	}
	return strings.Join(lines, "\n")
}

func nativeTerminalSequence() []string {
	sequence := []string{"GAME-ONE", "GAME-ONE", "[familiar] FAMILIAR-ONE", "REPLACE-ONE"}
	for number := 0; number < 36; number++ {
		sequence = append(sequence, fmt.Sprintf("TALL-%02d", number))
	}
	sequence = append(sequence, "UNICODE-WRAP-BEGIN", "UNICODE-WRAP-END")
	for number := 0; number < 36; number++ {
		sequence = append(sequence, fmt.Sprintf("MULTILINE-%02d", number))
	}
	sequence = append(sequence, "[system 01:02:03] SYSTEM-ONE", "[whisper] hello-whisper", "[agent] AGENT-ONE", "> async", "ASYNC-TWO", "> tail")
	for number := 0; number < 36; number++ {
		sequence = append(sequence, fmt.Sprintf("TAIL-%02d", number))
	}
	sequence = append(sequence, "> boundary-seed")
	sequence = append(sequence, nativeTerminalBoundarySeeds()...)
	sequence = append(sequence, "> boundary-advance")
	for number := 0; number < 14; number++ {
		sequence = append(sequence, fmt.Sprintf("BOUNDARY-ADVANCE-%02d", number))
	}
	sequence = append(sequence, "> flush")
	for number := 0; number < 36; number++ {
		sequence = append(sequence, fmt.Sprintf("FLUSH-%02d", number))
	}
	return append(sequence, "[system 01:02:03] disconnected")
}

func nativeTerminalBoundarySeeds() []string {
	seeds := make([]string, 8)
	for number := range seeds {
		seeds[number] = fmt.Sprintf("BOUNDARY-SEED-%02d", number)
	}
	return seeds
}

func assertCaptureBoundaryRace(t *testing.T, before, history, pane, after string) {
	t.Helper()
	missing := make([]string, 0, 3)
	assembled := history + pane
	for _, seed := range nativeTerminalBoundarySeeds() {
		if strings.Count(before, seed) != 1 || strings.Count(after, seed) != 1 {
			t.Fatalf("complete capture did not retain %q\nbefore:\n%s\nafter:\n%s", seed, before, after)
		}
		if !strings.Contains(assembled, seed) {
			missing = append(missing, seed)
		}
	}
	if len(missing) != 3 {
		t.Fatalf("split captures omitted %d boundary records, want 3: %v\nhistory:\n%s\npane:\n%s", len(missing), missing, history, pane)
	}
}

func assertNativeHistory(t *testing.T, history string) {
	t.Helper()
	for _, value := range []string{"CLEAR-ME", "[Test Hall]", "L: shield", "R: sword", "o---@", "READY |", "DISCONNECTED", "Help", "Theme", "map navigation"} {
		if strings.Contains(history, value) {
			t.Fatalf("immutable history contains dashboard, map, or modal value %q\n%s", value, history)
		}
	}
}

func assertUnicodeRecord(t *testing.T, text string) {
	t.Helper()
	for _, segment := range []string{"café", "你好", "👋"} {
		if count := strings.Count(text, segment); count != 3 {
			t.Fatalf("unicode segment %q count=%d, want 3\n%s", segment, count, text)
		}
	}
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func assertOrderedOnce(t *testing.T, text string, sequence []string) {
	t.Helper()
	wantCounts := make(map[string]int, len(sequence))
	for _, record := range sequence {
		wantCounts[record]++
	}
	position := 0
	for _, record := range sequence {
		index := strings.Index(text[position:], record)
		if index < 0 || strings.Count(text, record) != wantCounts[record] {
			t.Fatalf("terminal record %q count=%d want %d\n%s", record, strings.Count(text, record), wantCounts[record], text)
		}
		position += index + len(record)
	}
}
