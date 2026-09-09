package ui

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"dr-charm/internal/agent"
	"dr-charm/internal/presentation"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

// TestNativeTerminalHelper is launched by TestNativeTerminalScrollback inside
// an isolated tmux server. It deliberately drives the real program rather than
// a viewport or a byte-stream substitute.
func TestNativeTerminalHelper(t *testing.T) {
	if os.Getenv("DR_CHARM_NATIVE_TERMINAL_HELPER") != "1" {
		return
	}
	if os.Getenv("DR_CHARM_NATIVE_MIXED_HEIGHT") == "1" {
		runNativeMixedHeightHelper(t)
		return
	}
	if path := os.Getenv("DR_CHARM_NATIVE_PID_FILE"); path != "" {
		if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	previousProfile := lipgloss.Writer.Profile
	lipgloss.Writer.Profile = colorprofile.ANSI256
	t.Cleanup(func() { lipgloss.Writer.Profile = previousProfile })
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
	if os.Getenv("DR_CHARM_NATIVE_EXACT_WIDTH") == "60" {
		first.Entries = append(first.Entries,
			presentation.Entry{Pane: presentation.Game, Text: nativeTerminalExactWidthRecord(60), Operation: presentation.Append},
			presentation.Entry{Pane: presentation.Game, Text: nativeTerminalExactWidthRecord(120), Operation: presentation.Append},
			presentation.Entry{Pane: presentation.Game, Text: "AFTER-EXACT-WIDTH", Operation: presentation.Append},
		)
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
			advance := make([]presentation.Entry, 0, 11)
			for number := 0; number < 11; number++ {
				advance = append(advance, presentation.Entry{Pane: presentation.Game, Text: fmt.Sprintf("BOUNDARY-ADVANCE-%02d", number), Operation: presentation.Append})
			}
			updates <- readyUpdate(advance)
		case "flush":
			flush := make([]presentation.Entry, 0, 36)
			for number := 0; number < 36; number++ {
				flush = append(flush, presentation.Entry{Pane: presentation.Game, Text: fmt.Sprintf("FLUSH-%02d", number), Operation: presentation.Append})
			}
			updates <- readyUpdate(flush)
		case "post-resize":
			updates <- readyUpdate([]presentation.Entry{{Pane: presentation.Game, Text: nativeTerminalPostResizeRecord(), Operation: presentation.Append}})
		case "post-repaint":
			updates <- readyUpdate([]presentation.Entry{{Pane: presentation.Game, Text: "POST-REPAINT-ONE", Operation: presentation.Append}})
		case "post-editor":
			updates <- readyUpdate([]presentation.Entry{{Pane: presentation.Game, Text: "POST-EDITOR-ONE", Operation: presentation.Append}})
		case "close":
			close(updates)
		}
	}
	model := InitialEnhancedModel(session, Options{Context: context.Background(), ThemeDir: os.Getenv("DR_CHARM_NATIVE_THEME_DIR")})
	model.agent.client = nativeTerminalAgent{}
	model.now = func() time.Time { return time.Date(2026, 1, 1, 1, 2, 3, 0, time.UTC) }
	program := tea.NewProgram(model, tea.WithColorProfile(colorprofile.ANSI256))
	if _, err := program.Run(); err != nil {
		t.Fatal(err)
	}
}

func runNativeMixedHeightHelper(t *testing.T) {
	updates := make(chan presentation.Update, 1)
	base := presentation.Update{
		Connection: presentation.Ready,
		Prompt:     ">",
		Location:   presentation.Location{Title: "[Mixed Hall]", Exits: []string{"north"}},
		Hands:      presentation.Hands{Left: "shield", Right: "sword", PreparedSpell: "Fire"},
		Status:     []presentation.StatusField{{Label: "H", Value: "100%"}},
		Map:        presentation.Map{Lines: []string{"o---@", "    |", "    o", "    |", "    o"}},
	}
	updates <- base
	go func() {
		time.Sleep(200 * time.Millisecond)
		records := nativeMixedHeightRecords()
		if os.Getenv("DR_CHARM_NATIVE_TAB") == "1" {
			records = nativeTabRecords()
		}
		for number, record := range records {
			update := base
			update.Location.Title = fmt.Sprintf("[Mixed Hall %02d]", number)
			update.Status = []presentation.StatusField{{Label: "Burst", Value: strconv.Itoa(number)}}
			update.Map.Lines = []string{fmt.Sprintf("%02d--@", number), "    |", "    o", "    |", "    o"}
			update.Entries = []presentation.Entry{{Pane: presentation.Game, Text: record, Operation: presentation.Append}}
			updates <- update
			time.Sleep(3 * time.Millisecond)
		}
	}()
	session := &nativeTerminalSession{updates: updates, send: func(string) {}}
	model := InitialEnhancedModel(session, Options{Context: context.Background()})
	model.agent.client = nativeTerminalAgent{}
	if _, err := tea.NewProgram(model).Run(); err != nil {
		t.Fatal(err)
	}
}

func TestNativeTerminalTabInsertion(t *testing.T) {
	if os.Getenv("DR_CHARM_NATIVE_TERMINAL_HELPER") == "1" {
		return
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatalf("tmux is required for terminal-state proof: %v", err)
	}
	helper := filepath.Join(t.TempDir(), "native-ui.test")
	if output, err := exec.Command("go", "test", "-c", "-o", helper).CombinedOutput(); err != nil {
		t.Fatalf("prebuild tab helper: %v\n%s", err, output)
	}
	server := fmt.Sprintf("dr-charm-native-tab-%d", os.Getpid())
	session := "tab"
	tmux := func(args ...string) ([]byte, error) {
		return exec.Command("tmux", append([]string{"-L", server}, args...)...).CombinedOutput()
	}
	t.Cleanup(func() { _, _ = tmux("kill-server") })
	command := fmt.Sprintf("DR_CHARM_NATIVE_TERMINAL_HELPER=1 DR_CHARM_NATIVE_MIXED_HEIGHT=1 DR_CHARM_NATIVE_TAB=1 %s -test.run '^TestNativeTerminalHelper$' -test.count=1", shellQuote(helper))
	if output, err := tmux("new-session", "-d", "-x", "20", "-y", "15", "-s", session, command); err != nil {
		t.Fatalf("start tab helper: %v\n%s", err, output)
	}
	capture := func(arguments ...string) string {
		t.Helper()
		output, err := tmux(append([]string{"capture-pane", "-p", "-t", session}, arguments...)...)
		if err != nil {
			t.Fatalf("capture tab terminal: %v\n%s", err, output)
		}
		return string(output)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(capture("-J", "-S", "-1000"), "TAB-AFTER") {
		if time.Now().After(deadline) {
			t.Fatalf("tab stream did not finish\n%s", capture("-S", "-1000"))
		}
		time.Sleep(25 * time.Millisecond)
	}
	complete := normalizeTmuxPhysicalCapture(capture("-S", "-1000"), 20)
	wantRows := []string{
		"TAB-CONTROL 1234567A",
		"BCDEFGHIJKLM CONTROL",
		"-TAIL",
		"TAB-ONE 1234567 ABCD",
		"EFGHIJKLM TAB-TAIL",
		"TAB-MULTIPLE 1  2  3",
		" MULTIPLE-TAIL",
		"TAB-WIDE 1234567",
		"界界界界界界 WIDE-TA",
		"IL",
		strings.Repeat("m", 17),
		strings.Repeat("n", 17) + "  A",
		strings.Repeat("p", 20),
		strings.Repeat("q", 20),
		"A",
		strings.Repeat("a", 19),
		"界" + strings.Repeat("b", 18),
		"b",
		"TAB-AFTER",
	}
	rows := strings.Split(complete, "\n")
	start := slices.Index(rows, wantRows[0])
	if start < 0 || start+len(wantRows) > len(rows) || !slices.Equal(rows[start:start+len(wantRows)], wantRows) {
		t.Fatalf("tab fixture physical rows differ\ngot:\n%s\nwant:\n%s", complete, strings.Join(wantRows, "\n"))
	}
	wantCounts := make(map[string]int, len(wantRows))
	gotCounts := make(map[string]int, len(wantRows))
	for _, row := range wantRows {
		wantCounts[row]++
	}
	for _, row := range rows {
		if _, ok := wantCounts[row]; ok {
			gotCounts[row]++
		}
	}
	if !maps.Equal(gotCounts, wantCounts) {
		t.Fatalf("tab fixture physical row counts=%v, want %v\n%s", gotCounts, wantCounts, complete)
	}
	pane := capture()
	lastTitle := fmt.Sprintf("[Mixed Hall %02d]", len(nativeTabRecords())-1)
	if !strings.Contains(pane, "Command >") || !strings.Contains(pane, lastTitle) {
		t.Fatalf("tab insertion corrupted compact frame\n%s", pane)
	}
	assertNativeHistory(t, capture("-S", "-1000", "-E", "-1"))
}

func TestNativeTerminalMixedHeightInsertion(t *testing.T) {
	if os.Getenv("DR_CHARM_NATIVE_TERMINAL_HELPER") == "1" {
		return
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatalf("tmux is required for terminal-state proof: %v", err)
	}
	helper := filepath.Join(t.TempDir(), "native-ui.test")
	if output, err := exec.Command("go", "test", "-c", "-o", helper).CombinedOutput(); err != nil {
		t.Fatalf("prebuild mixed-height helper: %v\n%s", err, output)
	}
	server := fmt.Sprintf("dr-charm-native-mixed-%d", os.Getpid())
	session := "mixed-height"
	tmux := func(args ...string) ([]byte, error) {
		return exec.Command("tmux", append([]string{"-L", server}, args...)...).CombinedOutput()
	}
	t.Cleanup(func() { _, _ = tmux("kill-server") })
	command := fmt.Sprintf("DR_CHARM_NATIVE_TERMINAL_HELPER=1 DR_CHARM_NATIVE_MIXED_HEIGHT=1 %s -test.run '^TestNativeTerminalHelper$' -test.count=1", shellQuote(helper))
	if output, err := tmux("new-session", "-d", "-x", "168", "-y", "24", "-s", session, command); err != nil {
		t.Fatalf("start mixed-height helper: %v\n%s", err, output)
	}
	capture := func(arguments ...string) string {
		t.Helper()
		output, err := tmux(append([]string{"capture-pane", "-p", "-t", session}, arguments...)...)
		if err != nil {
			t.Fatalf("capture mixed-height terminal: %v\n%s", err, output)
		}
		return string(output)
	}
	last := "MIXED-AFTER"
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(capture("-J", "-S", "-1000"), last) {
		if time.Now().After(deadline) {
			t.Fatalf("mixed-height stream did not finish\n%s", capture("-S", "-1000"))
		}
		time.Sleep(25 * time.Millisecond)
	}
	if output, err := tmux("send-keys", "-t", session, "F6", "draft-proof"); err != nil {
		t.Fatalf("switch mixed-height helper to Whisper: %v\n%s", err, output)
	}
	deadline = time.Now().Add(5 * time.Second)
	var pane string
	for {
		pane = capture()
		if strings.Contains(pane, "Whisper > draft-proof") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Whisper input did not render after mixed-height stream\n%s", pane)
		}
		time.Sleep(25 * time.Millisecond)
	}
	complete := capture("-J", "-S", "-1000")
	assertOrderedOnce(t, complete, nativeMixedHeightRecords())
	if strings.Count(pane, "Whisper >") != 1 || strings.Contains(pane, "Command >") {
		t.Fatalf("input mode labels corrupted after mixed-height stream\n%s", pane)
	}
	if !strings.Contains(pane, "READY | LOG off | AGENT idle | Burst:42") {
		t.Fatalf("final status missing after mixed-height stream\n%s", pane)
	}
	rows := strings.Split(strings.TrimSuffix(pane, "\n"), "\n")
	if len(rows) < 11 {
		t.Fatalf("visible terminal rows=%d, want at least 11 dashboard rows\n%s", len(rows), pane)
	}
	rows = rows[len(rows)-11:]
	if top, bottom := nativeCompleteDashboardBounds(pane, 168); top != 0 || bottom != 1 {
		// The changing title intentionally differs from nativeCompleteDashboardBounds' Test Hall fixture.
		if bottom != 1 || !strings.HasPrefix(rows[0], "╭─ [Mixed Hall 42]") ||
			!strings.HasSuffix(rows[0], "╮") || ansi.StringWidth(rows[0]) != 168 {
			t.Fatalf("mixed-height dashboard bounds top=%d bottom=%d\n%s", top, bottom, pane)
		}
	}
	assertNativeHistory(t, capture("-S", "-1000", "-E", "-1"))
	if output, err := tmux("resize-window", "-t", session, "-x", "100", "-y", "24"); err != nil {
		t.Fatalf("narrow mixed-height helper after oversized output: %v\n%s", err, output)
	}
	time.Sleep(100 * time.Millisecond)
	if resized := capture("-J", "-S", "-1000"); strings.Count(resized, nativeMixedHeightRecords()[41]) != 1 {
		t.Fatalf("resized terminal changed oversized logical record\n%s", resized)
	}
	if output, err := tmux("resize-window", "-t", session, "-x", "168", "-y", "24"); err != nil {
		t.Fatalf("restore mixed-height helper width: %v\n%s", err, output)
	}
}

func nativeMixedHeightRecords() []string {
	records := make([]string, 0, 43)
	for number := 0; number < 40; number++ {
		if number%2 == 0 {
			records = append(records, fmt.Sprintf("MIXED-%02d short", number))
			continue
		}
		records = append(records, fmt.Sprintf("MIXED-%02d %s TAIL-%02d", number, strings.Repeat("long ", 38), number))
	}
	oversized := "MIXED-OVERSIZED " + strings.Repeat("oversized ", 240) + "OVERSIZED-TAIL"
	records = append(records,
		"MIXED-MULTILINE-SHORT\nMIXED-MULTILINE-LONG "+strings.Repeat("wide ", 40)+"MULTILINE-TAIL",
		"MIXED-STREAM-PRE\n"+oversized+"\n\n"+oversized+"-SECOND\nMIXED-STREAM-POST",
		"MIXED-AFTER",
	)
	return records
}

func nativeTabRecords() []string {
	return []string{
		"TAB-CONTROL 1234567ABCDEFGHIJKLM CONTROL-TAIL",
		"TAB-ONE 1234567\tABCDEFGHIJKLM TAB-TAIL",
		"TAB-MULTIPLE 1\t2\t3 MULTIPLE-TAIL",
		"TAB-WIDE 1234567\t界界界界界界 WIDE-TAIL",
		strings.Repeat("m", 17) + "\t",
		strings.Repeat("n", 17) + "\tA",
		strings.Repeat("p", 20) + "\t",
		strings.Repeat("q", 20) + "\tA",
		strings.Repeat("a", 19) + "界" + strings.Repeat("b", 19),
		"TAB-AFTER",
	}
}

func normalizeTmuxPhysicalCapture(capture string, width int) string {
	rows := strings.Split(capture, "\n")
	for index, row := range rows {
		var normalized strings.Builder
		column := 0
		for len(row) > 0 {
			if row[0] == '\t' {
				row = row[1:]
				if column >= width {
					continue
				}
				nextStop := column + 8 - column%8
				if nextStop >= width {
					nextStop = width - 1
				}
				normalized.WriteString(strings.Repeat(" ", nextStop-column))
				column = nextStop
				continue
			}

			cluster, clusterWidth := ansi.FirstGraphemeCluster(row, ansi.GraphemeWidth)
			if cluster == "" {
				break
			}
			normalized.WriteString(cluster)
			column += clusterWidth
			row = row[len(cluster):]
		}
		rows[index] = strings.TrimRight(normalized.String(), " ")
	}
	return strings.Join(rows, "\n")
}

func TestNormalizeTmuxPhysicalCapture(t *testing.T) {
	macOS := strings.Join([]string{
		"TAB-ONE 1234567\tABCD",
		"TAB-MULTIPLE 1\t2\t3",
		"TAB-WIDE 1234567\t",
		"界\tA COUNTER-TA",
		strings.Repeat("m", 17) + "\t",
		strings.Repeat("n", 17) + "\tA",
	}, "\n")
	linux := strings.Join([]string{
		"TAB-ONE 1234567 ABCD",
		"TAB-MULTIPLE 1  2  3",
		"TAB-WIDE 1234567",
		"界      A COUNTER-TA",
		strings.Repeat("m", 17),
		strings.Repeat("n", 17) + "  A",
	}, "\n")
	if got := normalizeTmuxPhysicalCapture(macOS, 20); got != linux {
		t.Fatalf("normalized macOS capture=%q, want Linux cell text %q", got, linux)
	}
	if got := normalizeTmuxPhysicalCapture(linux, 20); got != linux {
		t.Fatalf("normalized Linux capture=%q, want unchanged cell text %q", got, linux)
	}
	for _, changed := range []string{
		strings.Replace(linux, "  2", " 2", 1),
		strings.Replace(linux, "界", "好", 1),
	} {
		if normalizeTmuxPhysicalCapture(changed, 20) == linux {
			t.Fatalf("normalization hid changed capture %q", changed)
		}
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

func TestNativeTerminalHostResizeReflow(t *testing.T) {
	if os.Getenv("DR_CHARM_NATIVE_TERMINAL_HELPER") == "1" {
		return
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatalf("tmux is required for terminal-state proof: %v", err)
	}
	helper := filepath.Join(t.TempDir(), "native-ui.test")
	if output, err := exec.Command("go", "test", "-c", "-o", helper).CombinedOutput(); err != nil {
		t.Fatalf("prebuild host-reflow helper: %v\n%s", err, output)
	}
	server := fmt.Sprintf("dr-charm-native-reflow-%d", os.Getpid())
	session := "host-reflow"
	helperPIDFile := filepath.Join(t.TempDir(), "helper.pid")
	// The top always enters history at width 10; tmux may keep lower frame rows visible.
	const reflowedNarrowTop = "╭─ [Test H"
	tmux := func(args ...string) ([]byte, error) {
		return exec.Command("tmux", append([]string{"-L", server}, args...)...).CombinedOutput()
	}
	stopped := false
	var panePID int
	readHelperPID := func(path string) int {
		t.Helper()
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read host-reflow helper PID: %v", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
		if err != nil {
			t.Fatalf("parse host-reflow helper PID %q: %v", contents, err)
		}
		return pid
	}
	waitForStopped := func(pid int) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			output, err := exec.Command("ps", "-o", "state=", "-p", strconv.Itoa(pid)).CombinedOutput()
			if err != nil {
				t.Fatalf("read stopped helper state: %v\n%s", err, output)
			}
			if strings.HasPrefix(strings.TrimSpace(string(output)), "T") {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("host-reflow helper PID %d never stopped; state=%q", pid, output)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	t.Cleanup(func() {
		if stopped {
			_ = syscall.Kill(panePID, syscall.SIGCONT)
		}
		_, _ = tmux("kill-server")
	})
	helperCommand := fmt.Sprintf("DR_CHARM_NATIVE_TERMINAL_HELPER=1 DR_CHARM_NATIVE_PID_FILE=%s %s -test.run '^TestNativeTerminalHelper$' -test.count=1 </dev/tty & wait", shellQuote(helperPIDFile), shellQuote(helper))
	if output, err := tmux("new-session", "-d", "-x", "100", "-y", "30", "-s", session, "sh", "-c", helperCommand); err != nil {
		t.Fatalf("start host-reflow helper: %v\n%s", err, output)
	}
	capture := func(args ...string) string {
		t.Helper()
		output, err := tmux(append([]string{"capture-pane", "-p", "-t", session}, args...)...)
		if err != nil {
			t.Fatalf("capture host-reflow pane: %v\n%s", err, output)
		}
		return string(output)
	}
	waitVisible := func(predicate func(string) bool, description string) string {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			visible := capture()
			if predicate(visible) {
				return visible
			}
			if time.Now().After(deadline) {
				t.Fatalf("host-reflow pane never rendered %s\n%s", description, visible)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	waitInitialTranscript := func(description string) string {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			complete := capture("-J", "-S", "-1000")
			if strings.Contains(complete, "[system 01:02:03] SYSTEM-ONE") {
				return complete
			}
			if time.Now().After(deadline) {
				t.Fatalf("host-reflow %s did not finish\n%s", description, complete)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	waitVisible(func(visible string) bool { return strings.Contains(visible, "Command >") }, "initial dashboard")
	beforeComplete := waitInitialTranscript("initial transcript")
	beforeHistory := capture("-S", "-1000", "-E", "-1")
	assertInitialNativeTranscript(t, beforeComplete)
	assertNativeHistory(t, beforeHistory)

	panePID = readHelperPID(helperPIDFile)
	if err := syscall.Kill(panePID, syscall.SIGSTOP); err != nil {
		t.Fatalf("stop host-reflow helper: %v", err)
	}
	stopped = true
	waitForStopped(panePID)
	if output, err := tmux("resize-window", "-t", session, "-x", "44", "-y", "30"); err != nil {
		t.Fatalf("resize stopped helper to 44 columns: %v\n%s", err, output)
	}
	waitForStopped(panePID)
	reflow44History := capture("-S", "-1000", "-E", "-1")
	if !strings.Contains(reflow44History, "[Test Hall]") || lineCount(reflow44History) <= lineCount(beforeHistory) {
		t.Fatalf("44-column host reflow did not retain dashboard fragments in history\nbefore:\n%s\nafter:\n%s", beforeHistory, reflow44History)
	}
	if output, err := tmux("resize-window", "-t", session, "-x", "10", "-y", "30"); err != nil {
		t.Fatalf("resize stopped helper to 10 columns: %v\n%s", err, output)
	}
	waitForStopped(panePID)
	reflow10History := capture("-S", "-1000", "-E", "-1")
	if lineCount(reflow10History) <= lineCount(reflow44History) || !slices.Contains(strings.Split(reflow10History, "\n"), reflowedNarrowTop) {
		t.Fatalf("10-column host reflow did not expand retained dashboard fragments\n44 columns:\n%s\n10 columns:\n%s", reflow44History, reflow10History)
	}
	if output, err := tmux("resize-window", "-t", session, "-x", "44", "-y", "30"); err != nil {
		t.Fatalf("restore stopped helper to 44 columns: %v\n%s", err, output)
	}
	waitForStopped(panePID)
	if err := syscall.Kill(panePID, syscall.SIGCONT); err != nil {
		t.Fatalf("resume host-reflow helper: %v", err)
	}
	stopped = false
	resumedPane := waitVisible(func(visible string) bool {
		tops, bottoms := nativeCompleteDashboardBounds(visible, 44)
		return tops == 1 && bottoms == 1 && nativeDashboardBottomComplete(visible, 44)
	}, "bounded 44-column repaint")
	boundedTopRows, boundedBottomRows := nativeCompleteDashboardBounds(resumedPane, 44)
	if boundedTopRows != 1 || boundedBottomRows != 1 {
		t.Fatalf("resumed repaint contained bounded tops=%d bottoms=%d, want 1 each\n%s", boundedTopRows, boundedBottomRows, resumedPane)
	}
	resumedComplete := capture("-J", "-S", "-1000")
	assertInitialNativeTranscript(t, resumedComplete)
	if output, err := tmux("send-keys", "-t", session, "F1"); err != nil {
		t.Fatalf("settle Help after host reflow: %v\n%s", err, output)
	}
	waitVisible(func(visible string) bool { return strings.Contains(visible, "Help") }, "settling Help after host reflow")
	if output, err := tmux("send-keys", "-t", session, "Escape"); err != nil {
		t.Fatalf("settle dashboard after host reflow: %v\n%s", err, output)
	}
	waitVisible(func(visible string) bool { return strings.Contains(visible, "Command >") }, "settling dashboard after Help")
	stableHistory := capture("-S", "-1000", "-E", "-1")
	if output, err := tmux("send-keys", "-t", session, "F1"); err != nil {
		t.Fatalf("open settled Help after host reflow: %v\n%s", err, output)
	}
	waitVisible(func(visible string) bool { return strings.Contains(visible, "Help") }, "settled Help after host reflow")
	if output, err := tmux("send-keys", "-t", session, "Escape"); err != nil {
		t.Fatalf("close settled Help after host reflow: %v\n%s", err, output)
	}
	waitVisible(func(visible string) bool { return strings.Contains(visible, "Command >") }, "settled dashboard after Help")
	if afterStableModes := capture("-S", "-1000", "-E", "-1"); afterStableModes != stableHistory {
		t.Fatalf("stable-geometry mode repaint changed history\nbefore:\n%s\nafter:\n%s", stableHistory, afterStableModes)
	}
	if output, err := tmux("kill-session", "-t", session); err != nil {
		t.Fatalf("stop first host-reflow session: %v\n%s", err, output)
	}
	session = "direct-narrow"
	directPIDFile := filepath.Join(t.TempDir(), "direct-helper.pid")
	directCommand := fmt.Sprintf("DR_CHARM_NATIVE_TERMINAL_HELPER=1 DR_CHARM_NATIVE_PID_FILE=%s %s -test.run '^TestNativeTerminalHelper$' -test.count=1 </dev/tty & wait", shellQuote(directPIDFile), shellQuote(helper))
	if output, err := tmux("new-session", "-d", "-x", "100", "-y", "30", "-s", session, "sh", "-c", directCommand); err != nil {
		t.Fatalf("start direct-narrow helper: %v\n%s", err, output)
	}
	waitVisible(func(visible string) bool { return strings.Contains(visible, "Command >") }, "direct-narrow initial dashboard")
	directComplete := waitInitialTranscript("direct-narrow initial transcript")
	directBeforeHistory := capture("-S", "-1000", "-E", "-1")
	assertNativeHistory(t, directBeforeHistory)
	assertInitialNativeTranscript(t, directComplete)
	panePID = readHelperPID(directPIDFile)
	if err := syscall.Kill(panePID, syscall.SIGSTOP); err != nil {
		t.Fatalf("stop direct-narrow helper: %v", err)
	}
	stopped = true
	waitForStopped(panePID)
	if output, err := tmux("resize-window", "-t", session, "-x", "10", "-y", "30"); err != nil {
		t.Fatalf("resize stopped helper directly to 10 columns: %v\n%s", err, output)
	}
	waitForStopped(panePID)
	direct10History := capture("-S", "-1000", "-E", "-1")
	if lineCount(direct10History) <= lineCount(directBeforeHistory) || !slices.Contains(strings.Split(direct10History, "\n"), reflowedNarrowTop) {
		t.Fatalf("direct 100-to-10 host reflow did not retain expanded dashboard fragments\nbefore:\n%s\nafter:\n%s", directBeforeHistory, direct10History)
	}
	if err := syscall.Kill(panePID, syscall.SIGCONT); err != nil {
		t.Fatalf("resume direct-narrow helper: %v", err)
	}
	stopped = false
	waitVisible(func(visible string) bool {
		for _, line := range strings.Split(visible, "\n") {
			if line == "Command…" {
				return true
			}
		}
		return false
	}, "bounded direct 10-column repaint")
	assertInitialNativeTranscript(t, capture("-J", "-S", "-1000"))
	if output, err := tmux("send-keys", "-t", session, "F1"); err != nil {
		t.Fatalf("settle narrow Help after direct resize: %v\n%s", err, output)
	}
	waitVisible(func(visible string) bool { return strings.Contains(visible, "Help") }, "settling narrow Help after direct resize")
	if output, err := tmux("send-keys", "-t", session, "Escape"); err != nil {
		t.Fatalf("settle narrow dashboard after direct resize: %v\n%s", err, output)
	}
	waitVisible(func(visible string) bool { return strings.Contains(visible, "Command…") }, "settling narrow dashboard after Help")
	directStableHistory := capture("-S", "-1000", "-E", "-1")
	if output, err := tmux("send-keys", "-t", session, "F1"); err != nil {
		t.Fatalf("open settled narrow Help after direct resize: %v\n%s", err, output)
	}
	waitVisible(func(visible string) bool { return strings.Contains(visible, "Help") }, "settled narrow Help after direct resize")
	if output, err := tmux("send-keys", "-t", session, "Escape"); err != nil {
		t.Fatalf("close settled narrow Help after direct resize: %v\n%s", err, output)
	}
	waitVisible(func(visible string) bool { return strings.Contains(visible, "Command…") }, "settled narrow dashboard after Help")
	if afterDirectModes := capture("-S", "-1000", "-E", "-1"); afterDirectModes != directStableHistory {
		t.Fatalf("direct narrow stable repaint changed history\nbefore:\n%s\nafter:\n%s", directStableHistory, afterDirectModes)
	}

	if directory := os.Getenv("DR_CHARM_TERMINAL_ARTIFACT_DIR"); directory != "" {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		for name, contents := range map[string]string{
			"host-reflow-100x30-before-history.txt":        beforeHistory,
			"host-reflow-44x30-stopped-history.txt":        reflow44History,
			"host-reflow-10x30-stopped-history.txt":        reflow10History,
			"host-reflow-44x30-resumed-history.txt":        stableHistory,
			"host-reflow-direct-10x30-stopped-history.txt": direct10History,
			"host-reflow-direct-10x30-resumed-history.txt": directStableHistory,
		} {
			if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestNativeTerminalFixedGeometry(t *testing.T) {
	if os.Getenv("DR_CHARM_NATIVE_TERMINAL_HELPER") == "1" {
		return
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatalf("tmux is required for terminal-state proof: %v", err)
	}
	helper := filepath.Join(t.TempDir(), "native-ui.test")
	if output, err := exec.Command("go", "test", "-c", "-o", helper).CombinedOutput(); err != nil {
		t.Fatalf("prebuild fixed-geometry helper: %v\n%s", err, output)
	}
	server := fmt.Sprintf("dr-charm-native-fixed-%d", os.Getpid())
	tmux := func(args ...string) ([]byte, error) {
		return exec.Command("tmux", append([]string{"-L", server}, args...)...).CombinedOutput()
	}
	t.Cleanup(func() { _, _ = tmux("kill-server") })
	artifactDirectory := os.Getenv("DR_CHARM_TERMINAL_ARTIFACT_DIR")
	if artifactDirectory != "" {
		if err := os.MkdirAll(artifactDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name, width, height, input, artifact string
		contains                             []string
		absent                               []string
		exactWidth                           bool
	}{
		{name: "full", width: "100", height: "30", input: "Command >", contains: []string{"╭", "38;5;62", "38;5;170", "38;5;252", "48;5;235"}},
		{name: "compact-map", width: "100", height: "19", input: "Command >", artifact: "100x19-compact-map", contains: []string{"o---@", "38;5;252", "48;5;235"}, absent: []string{"╭"}},
		{name: "short", width: "60", height: "15", input: "Command >", artifact: "60x15-short", contains: []string{"38;5;252", "48;5;235"}, absent: []string{"╭", "o---@"}, exactWidth: true},
		{name: "narrow", width: "10", height: "24", input: "Command…", artifact: "10x24-narrow", contains: []string{"38;5;252", "48;5;235"}, absent: []string{"╭", "o---@"}},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := fmt.Sprintf("fixed-%d", index)
			command := []string{"new-session", "-d", "-x", test.width, "-y", test.height, "-s", session, "env", "DR_CHARM_NATIVE_TERMINAL_HELPER=1"}
			if test.exactWidth {
				command = append(command, "DR_CHARM_NATIVE_EXACT_WIDTH=60")
			}
			command = append(command, helper, "-test.run", "^TestNativeTerminalHelper$", "-test.count=1")
			if output, err := tmux(command...); err != nil {
				t.Fatalf("start %s helper: %v\n%s", test.name, err, output)
			}
			t.Cleanup(func() { _, _ = tmux("kill-session", "-t", session) })
			capture := func(arguments ...string) string {
				t.Helper()
				output, err := tmux(append([]string{"capture-pane", "-p", "-t", session}, arguments...)...)
				if err != nil {
					t.Fatalf("capture %s pane: %v\n%s", test.name, err, output)
				}
				return string(output)
			}
			waitCapture := func(arguments []string, predicate func(string) bool, description string) string {
				t.Helper()
				deadline := time.Now().Add(10 * time.Second)
				for {
					captured := capture(arguments...)
					if predicate(captured) {
						return captured
					}
					if time.Now().After(deadline) {
						t.Fatalf("%s pane never rendered %s\n%q", test.name, description, captured)
					}
					time.Sleep(25 * time.Millisecond)
				}
			}
			waitCapture([]string{"-J", "-S", "-1000"}, func(captured string) bool {
				return strings.Contains(captured, "[system 01:02:03] SYSTEM-ONE")
			}, "complete initial transcript")
			ansiCapture := waitCapture([]string{"-e"}, func(captured string) bool {
				if !strings.Contains(captured, test.input) {
					return false
				}
				for _, value := range test.contains {
					if !strings.Contains(captured, value) {
						return false
					}
				}
				for _, value := range test.absent {
					if strings.Contains(captured, value) {
						return false
					}
				}
				return true
			}, "expected geometry and ANSI256 colors")
			complete := capture("-J", "-S", "-1000")
			assertInitialNativeTranscript(t, complete)
			if test.exactWidth {
				assertOrderedOnce(t, complete, []string{nativeTerminalExactWidthRecord(60), nativeTerminalExactWidthRecord(120), "AFTER-EXACT-WIDTH"})
			}
			stableHistory := capture("-S", "-1000", "-E", "-1")
			assertNativeHistory(t, stableHistory)
			if output, err := tmux("send-keys", "-t", session, "F1"); err != nil {
				t.Fatalf("open %s Help: %v\n%s", test.name, err, output)
			}
			waitCapture(nil, func(captured string) bool { return strings.Contains(captured, "Help") }, "Help")
			if output, err := tmux("send-keys", "-t", session, "Escape", "F3"); err != nil {
				t.Fatalf("open %s Theme: %v\n%s", test.name, err, output)
			}
			waitCapture(nil, func(captured string) bool { return strings.Contains(captured, "Theme") }, "Theme")
			if output, err := tmux("send-keys", "-t", session, "Escape"); err != nil {
				t.Fatalf("close %s Theme: %v\n%s", test.name, err, output)
			}
			waitCapture(nil, func(captured string) bool { return strings.Contains(captured, test.input) }, "restored dashboard")
			if afterModes := capture("-S", "-1000", "-E", "-1"); afterModes != stableHistory {
				t.Fatalf("%s stable mode repaint changed history\nbefore:\n%s\nafter:\n%s", test.name, stableHistory, afterModes)
			}
			if artifactDirectory != "" && test.artifact != "" {
				if err := os.WriteFile(filepath.Join(artifactDirectory, test.artifact+".ansi"), []byte(ansiCapture), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
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
	artifactDirectory := os.Getenv("DR_CHARM_TERMINAL_ARTIFACT_DIR")
	if artifactDirectory != "" {
		if err := os.MkdirAll(artifactDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	themeDirectory := filepath.Join(t.TempDir(), "themes")
	if err := os.MkdirAll(themeDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	proofTheme := `{"name":"proof-chrome","foreground":"231","border":"39","title_bar":"214","status_bar":"16","status_bar_bg":"220","border_type":"double","padding":2}`
	if err := os.WriteFile(filepath.Join(themeDirectory, "proof-chrome.json"), []byte(proofTheme), 0o600); err != nil {
		t.Fatal(err)
	}
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
	command := fmt.Sprintf("DR_CHARM_NATIVE_TERMINAL_HELPER=1 DR_CHARM_NATIVE_THEME_DIR=%s DR_CHARM_EDITOR_READY=%s EDITOR=%s %s -test.run '^TestNativeTerminalHelper$' -test.count=1; exec sh", shellQuote(themeDirectory), shellQuote(editorReady), shellQuote(editor), shellQuote(helper))
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
	waitForVisibleWidth := func(record string, width int) string {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			capture, err := tmux("capture-pane", "-p", "-t", session)
			if err != nil {
				t.Fatalf("poll width-bounded terminal: %v\n%s", err, capture)
			}
			for _, line := range strings.Split(string(capture), "\n") {
				if strings.Contains(line, record) && ansi.StringWidth(line) == width {
					return string(capture)
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("visible terminal never rendered %q at width %d\n%s", record, width, capture)
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
	waitForClearedDashboardInput := func(submitted string, width int) string {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			capture, err := tmux("capture-pane", "-p", "-t", session)
			if err != nil {
				t.Fatalf("poll cleared dashboard input: %v\n%s", err, capture)
			}
			pane := string(capture)
			if nativeDashboardInputCleared(pane, submitted, width) {
				return pane
			}
			if time.Now().After(deadline) {
				t.Fatalf("dashboard input did not clear submitted command %q\n%s", submitted, pane)
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
	captureANSI := func(name string, cues ...string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		var capture []byte
		for {
			var err error
			capture, err = tmux("capture-pane", "-e", "-p", "-t", session)
			if err != nil {
				t.Fatalf("capture %s ANSI terminal pane: %v\n%s", name, err, capture)
			}
			ready := true
			for _, cue := range cues {
				ready = ready && strings.Contains(string(capture), cue)
			}
			if ready {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("ANSI terminal pane %s never contained cues %q\n%q", name, cues, capture)
			}
			time.Sleep(25 * time.Millisecond)
		}
		if artifactDirectory == "" {
			return
		}
		if err := os.WriteFile(filepath.Join(artifactDirectory, name+".ansi"), capture, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sequence := nativeTerminalSequence()
	assertCheckpoint := func(name, visible, absent string, requireCleanHistory bool) {
		t.Helper()
		complete := captureCompleteJoined()
		assertOrderedOnce(t, complete, sequence)
		assertUnicodeRecord(t, complete)
		if requireCleanHistory {
			assertNativeHistory(t, captureHistory())
		}
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
	send("draft-proof")
	waitForVisible("draft-proof")
	captureANSI("100x30-default-command", "╭", "Command >", "38;5;62", "38;5;170", "38;5;252", "48;5;235")
	send("Escape")
	waitForVisible("Map navigation")
	captureANSI("100x30-map-navigation", "╭", "Map navigation", "38;5;170", "38;5;252", "48;5;235")
	send("Tab")
	waitForVisible("Command >")
	send("F6")
	waitForVisible("Whisper >")
	captureANSI("100x30-whisper", "╭", "Whisper >", "38;5;62", "38;5;252", "48;5;235")
	send("F6")
	waitForVisible("Command >")
	send("F1")
	waitForVisible("Help")
	captureANSI("100x30-help", "╭", "Help", "38;5;62", "38;5;170", "38;5;252", "48;5;235")
	send("Escape")
	waitForVisible("READY | LOG off")
	send("F3")
	waitForVisible("Theme")
	send("Down")
	waitForVisible("> dark")
	captureANSI("100x30-theme-dark-selected", "╭", "Theme", "> dark", "38;5;237", "38;5;33", "38;5;252", "48;5;237")
	send("Enter")
	waitForVisible("Command >")
	captureANSI("100x30-dark-dashboard", "╭", "Command >", "38;5;237", "38;5;33", "38;5;252", "48;5;237")
	send("F3")
	waitForVisible("Theme")
	send("Down")
	waitForVisible("> high-contrast")
	send("Enter")
	captureANSI("100x30-high-contrast-dashboard", "┏", "Command >", "38;5;226", "\x1b[30m", "\x1b[107m")
	send("F3")
	waitForVisible("Theme")
	send("G")
	waitForVisible("> proof-chrome")
	send("Enter")
	captureANSI("100x30-proof-chrome-dashboard", "╔", "Command >", "38;5;39", "38;5;214", "38;5;16", "48;5;220")
	send("F3")
	waitForVisible("Theme")
	send("g")
	waitForVisible("> default")
	send("Enter")
	captureANSI("100x30-default-dashboard", "╭", "Command >", "38;5;62", "38;5;170", "38;5;252", "48;5;235")
	if pane := capturePane(); !strings.Contains(pane, "draft-proof") {
		t.Fatalf("modes changed command draft\n%s", pane)
	}
	send("C-u")
	waitForVisibleWithout("Command >", "draft-proof")
	waitForVisible("READY | LOG off")
	send("F6")
	waitForVisible("Whisper >")
	send("hello-whisper", "Enter")
	waitFor("AGENT-ONE")
	send("F6")
	deadline := time.Now().Add(10 * time.Second)
	for strings.Contains(waitForVisible("READY | LOG off"), "Whisper >") {
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
	waitFor("BOUNDARY-ADVANCE-10")
	boundaryPane := capturePane()
	boundaryAfter := captureComplete()
	assertCaptureBoundaryRace(t, boundaryBefore, boundaryHistory, boundaryPane, boundaryAfter)
	send("flush", "Enter")
	waitFor("FLUSH-35")
	waitForVisible("READY |")
	assertCheckpoint("before resize", "o---@", "", true)
	if out, err := tmux("resize-window", "-t", session, "-x", "44", "-y", "30"); err != nil {
		t.Fatalf("width-only resize: %v\n%s", err, out)
	}
	waitForVisibleWithout("Command >", "o---@")
	assertCheckpoint("width-only resize", "Command >", "o---@", false)
	if out, err := tmux("resize-window", "-t", session, "-x", "44", "-y", "18"); err != nil {
		t.Fatalf("below-map resize: %v\n%s", err, out)
	}
	send("F1")
	waitForVisibleWidth("Help", 44)
	send("Escape")
	waitForVisibleWithout("Command >", "Help")
	waitForVisibleWithout("L: shield", "o---@")
	assertCheckpoint("below-map resize", "L: shield", "o---@", false)
	send("post-resize", "Enter")
	postResizePrefix := strings.TrimSuffix(nativeTerminalPostResizeRecord(), "Ω")
	waitFor(postResizePrefix)
	waitForClearedDashboardInput("post-resize", 44)
	sequence = append(sequence, "> post-resize", postResizePrefix, "Ω")
	assertCheckpoint("post-resize output at 44x18", "Command >", "o---@", false)
	if complete := captureCompleteJoined(); strings.Count(complete, nativeTerminalPostResizeRecord()) != 1 {
		t.Fatalf("44x18 terminal did not contain the complete post-resize record exactly once\n%s", complete)
	} else if !strings.Contains(complete, nativeTerminalPostResizeRecord()+"\n") {
		t.Fatalf("44x18 terminal joined the post-resize record to following frame content\n%s", complete)
	}
	if out, err := tmux("resize-window", "-t", session, "-x", "100", "-y", "19"); err != nil {
		t.Fatalf("at-map resize: %v\n%s", err, out)
	}
	waitForVisible("o---@")
	assertCheckpoint("at-map resize", "o---@", "", false)
	if out, err := tmux("resize-window", "-t", session, "-x", "60", "-y", "15"); err != nil {
		t.Fatalf("short resize: %v\n%s", err, out)
	}
	waitForVisibleWithout("Command >", "o---@")
	if out, err := tmux("resize-window", "-t", session, "-x", "10", "-y", "24"); err != nil {
		t.Fatalf("narrow resize: %v\n%s", err, out)
	}
	waitForVisible("Comman")
	for _, size := range [][2]string{{"100", "30"}, {"44", "20"}, {"100", "30"}} {
		if out, err := tmux("resize-window", "-t", session, "-x", size[0], "-y", size[1]); err != nil {
			t.Fatalf("rapid resize %s×%s: %v\n%s", size[0], size[1], err, out)
		}
	}
	send("F1")
	waitForVisibleWidth("Help", 100)
	send("Escape")
	widePane := waitForVisibleAll("o---@", "L: shield", "R: sword", "READY |")
	assertCheckpoint("rapid resize", "o---@", "", false)
	if !strings.Contains(widePane, "L: shield") || !strings.Contains(widePane, "R: sword") || !strings.Contains(widePane, "READY |") {
		t.Fatalf("wide dashboard omitted map-adjacent hands or status\n%s", widePane)
	}
	send("post-repaint", "Enter")
	waitFor("POST-REPAINT-ONE")
	waitForClearedDashboardInput("post-repaint", 100)
	sequence = append(sequence, "> post-repaint", "POST-REPAINT-ONE")
	assertCheckpoint("post-repaint output", "o---@", "", false)
	settledHistory := captureHistory()
	send("C-g")
	waitForFile(editorReady)
	waitForVisible("EDITOR-ONE")
	if editorHistory := captureHistory(); editorHistory != settledHistory {
		t.Fatalf("stable-geometry editor repaint changed history\nbefore:\n%s\nafter:\n%s", settledHistory, editorHistory)
	}
	send("C-u")
	waitForVisibleWithout("Command >", "EDITOR-ONE")
	send("post-editor", "Enter")
	waitFor("POST-EDITOR-ONE")
	waitForClearedDashboardInput("post-editor", 100)
	sequence = append(sequence, "> post-editor", "POST-EDITOR-ONE")
	assertCheckpoint("post-editor output", "o---@", "", false)
	historyAfterEditorOutput := captureHistory()
	assertNoAdditionalNativeChrome(t, settledHistory, historyAfterEditorOutput)
	send("close", "Enter")
	waitForVisible("DISCONNECTED")
	sequence = append(sequence, "> close", "[system 01:02:03] disconnected")
	disconnectedPane := capturePane()
	if !strings.Contains(disconnectedPane, "[Test Hall]") {
		t.Fatalf("source close did not retain disconnected dashboard\n%s", disconnectedPane)
	}
	assertCheckpoint("source close", "DISCONNECTED", "", false)
	activeComplete := captureComplete()
	activeHistory := captureHistory()
	assertNoAdditionalNativeChrome(t, historyAfterEditorOutput, activeHistory)
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
	if artifactDirectory != "" {
		for name, contents := range map[string]string{"terminal-active-history.txt": activeHistory} {
			if err := os.WriteFile(filepath.Join(artifactDirectory, name), []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	assertOrderedOnce(t, activeComplete, sequence)
	activeJoined := captureCompleteJoined()
	assertUnicodeRecord(t, activeJoined)
	if count := strings.Count(activeJoined, nativeTerminalPostResizeRecord()); count != 1 {
		t.Fatalf("joined terminal record %q count=%d, want 1\n%s", nativeTerminalPostResizeRecord(), count, activeJoined)
	}
	for _, command := range []string{"> post-resize", "> post-repaint", "> post-editor", "> close"} {
		if count := strings.Count(activeComplete, command); count != 1 {
			t.Fatalf("terminal command %q count=%d, want 1\n%s", command, count, activeComplete)
		}
	}
	for _, record := range []string{"[familiar] FAMILIAR-ONE", "REPLACE-ONE", "ASYNC-TWO", "[whisper] hello-whisper", "AGENT-ONE"} {
		if strings.Count(activeComplete, record) != 1 {
			t.Fatalf("terminal record %q count=%d\n%s", record, strings.Count(activeHistory, record), activeHistory)
		}
	}
	if strings.Count(activeComplete, "GAME-ONE") != 2 {
		t.Fatalf("duplicate game record not retained twice\n%s", activeHistory)
	}
	if !strings.Contains(activeComplete, "GAME-ONE\nGAME-ONE\n\n[familiar] FAMILIAR-ONE") {
		t.Fatalf("empty append did not create a visually blank terminal line\n%s", activeHistory)
	}
	if !strings.Contains(widePane, "o---@") {
		t.Fatalf("visible pane lost map after wide resize\n%s", widePane)
	}
	assertOrderedOnce(t, postQuitComplete, sequence)
	assertUnicodeRecord(t, captureCompleteJoined())
	assertNoAdditionalNativeChrome(t, activeHistory, postQuitHistory)
	postQuitCursor, err := tmux("display-message", "-p", "-t", session, "#{cursor_flag}:#{cursor_x}:#{cursor_y}")
	if err != nil {
		t.Fatalf("read post-quit cursor state: %v\n%s", err, postQuitCursor)
	}
	if !strings.HasPrefix(string(postQuitCursor), "1:") {
		t.Fatalf("post-quit cursor was not restored: %q", postQuitCursor)
	}
}

func TestNativeDashboardInputClearedUsesBottommostFrame(t *testing.T) {
	const width = 30
	frame := func(value string) string {
		input := "│ Command > " + value
		return input + strings.Repeat(" ", width-ansi.StringWidth(input)-1) + "│\n╰" + strings.Repeat("─", width-2) + "╯"
	}
	staleCleared := frame("")
	currentDraft := frame("post-repaint")
	if nativeDashboardInputCleared(staleCleared+"\n"+currentDraft, "post-repaint", width) {
		t.Fatal("stale cleared input masked occupied current input")
	}
	partialCurrent := "│ Command > " + strings.Repeat(" ", width-len("│ Command > ")-1) + "│\n╰────"
	if nativeDashboardInputCleared(staleCleared+"\n"+partialCurrent, "post-repaint", width) {
		t.Fatal("stale cleared input masked partial current frame")
	}
	currentCleared := frame("")
	if !nativeDashboardInputCleared(currentDraft+"\n"+currentCleared, "post-repaint", width) {
		t.Fatal("cleared current input was not ready")
	}
	sharedBody := "│ Command >       │L: shield│o---@│\n" +
		"│                 │R: sword │  |  │\n" +
		"╰────────────────────────────╯"
	if !nativeDashboardInputCleared(sharedBody, "post-repaint", width) {
		t.Fatal("cleared shared-body input was not ready")
	}
}

func nativeDashboardInputCleared(pane, submitted string, width int) bool {
	lines := nativeNonblankRows(pane)
	if len(lines) == 0 {
		return false
	}
	last := lines[len(lines)-1]
	if strings.Contains(last, "Command >") && !strings.Contains(last, submitted) && ansi.StringWidth(last) <= width {
		return true
	}
	if !strings.HasPrefix(last, "╰") || !strings.HasSuffix(last, "╯") || ansi.StringWidth(last) != width {
		return false
	}
	for index := len(lines) - 2; index >= 0; index-- {
		line := lines[index]
		if strings.HasPrefix(line, "╰") && strings.HasSuffix(line, "╯") {
			break
		}
		if strings.Contains(line, "│ Command >") && !strings.Contains(line, submitted) &&
			strings.HasPrefix(line, "│") && strings.HasSuffix(line, "│") {
			return true
		}
	}
	return false
}

func nativeCompleteDashboardBounds(pane string, width int) (int, int) {
	var tops, bottoms int
	for _, line := range strings.Split(pane, "\n") {
		if ansi.StringWidth(line) != width {
			continue
		}
		if strings.HasPrefix(line, "╭─ [Test Hall]") && strings.HasSuffix(line, "╮") {
			tops++
		}
		if strings.HasPrefix(line, "╰") && strings.HasSuffix(line, "╯") {
			bottoms++
		}
	}
	return tops, bottoms
}

func nativeDashboardBottomComplete(pane string, width int) bool {
	lines := nativeNonblankRows(pane)
	if len(lines) == 0 {
		return false
	}
	bottom := lines[len(lines)-1]
	return strings.HasPrefix(bottom, "╰") && strings.HasSuffix(bottom, "╯") && ansi.StringWidth(bottom) == width
}

func nativeNonblankRows(pane string) []string {
	var lines []string
	for _, line := range strings.Split(pane, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func nativeTerminalMultilineRecord() string {
	lines := make([]string, 36)
	for number := range lines {
		lines[number] = fmt.Sprintf("MULTILINE-%02d", number)
	}
	return strings.Join(lines, "\n")
}

func nativeTerminalExactWidthRecord(width int) string {
	prefix := fmt.Sprintf("EXACT-WIDTH-%03d-", width)
	return prefix + strings.Repeat("x", width-len(prefix))
}

func nativeTerminalPostResizeRecord() string {
	prefix := "POST-RESIZE-"
	return prefix + strings.Repeat("x", 44-len(prefix)) + "Ω"
}

func assertInitialNativeTranscript(t *testing.T, text string) {
	t.Helper()
	assertOrderedOnce(t, text, nativeTerminalInitialSequence())
	assertUnicodeRecord(t, text)
	if !containsNativeBlankAppend(text) {
		t.Fatalf("empty append did not create a visually blank terminal line\n%s", text)
	}
}

func containsNativeBlankAppend(text string) bool {
	lines := strings.Split(text, "\n")
	for index := 0; index+3 < len(lines); index++ {
		if lines[index] == "GAME-ONE" && lines[index+1] == "GAME-ONE" && strings.TrimSpace(lines[index+2]) == "" && lines[index+3] == "[familiar] FAMILIAR-ONE" {
			return true
		}
	}
	return false
}

func nativeTerminalInitialSequence() []string {
	sequence := []string{"GAME-ONE", "GAME-ONE", "[familiar] FAMILIAR-ONE", "REPLACE-ONE"}
	for number := 0; number < 36; number++ {
		sequence = append(sequence, fmt.Sprintf("TALL-%02d", number))
	}
	sequence = append(sequence, "UNICODE-WRAP-BEGIN", "UNICODE-WRAP-END")
	for number := 0; number < 36; number++ {
		sequence = append(sequence, fmt.Sprintf("MULTILINE-%02d", number))
	}
	return append(sequence, "[system 01:02:03] SYSTEM-ONE")
}

func lineCount(text string) int {
	return strings.Count(text, "\n")
}

func nativeTerminalSequence() []string {
	sequence := nativeTerminalInitialSequence()
	sequence = append(sequence, "[whisper] hello-whisper", "[agent] AGENT-ONE", "> async", "ASYNC-TWO", "> tail")
	for number := 0; number < 36; number++ {
		sequence = append(sequence, fmt.Sprintf("TAIL-%02d", number))
	}
	sequence = append(sequence, "> boundary-seed")
	sequence = append(sequence, nativeTerminalBoundarySeeds()...)
	sequence = append(sequence, "> boundary-advance")
	for number := 0; number < 11; number++ {
		sequence = append(sequence, fmt.Sprintf("BOUNDARY-ADVANCE-%02d", number))
	}
	sequence = append(sequence, "> flush")
	for number := 0; number < 36; number++ {
		sequence = append(sequence, fmt.Sprintf("FLUSH-%02d", number))
	}
	return sequence
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
	if len(missing) != 1 {
		t.Fatalf("split captures omitted %d boundary records, want 1: %v\nhistory:\n%s\npane:\n%s", len(missing), missing, history, pane)
	}
}

func assertNativeHistory(t *testing.T, history string) {
	t.Helper()
	for _, value := range nativeChromeValues() {
		if strings.Contains(history, value) {
			t.Fatalf("immutable history contains dashboard, map, or modal value %q\n%s", value, history)
		}
	}
}

func assertNoAdditionalNativeChrome(t *testing.T, before, after string) {
	t.Helper()
	for _, value := range nativeChromeValues() {
		if strings.Count(after, value) > strings.Count(before, value) {
			t.Fatalf("stable terminal operation added dashboard, map, or modal value %q to history\nbefore:\n%s\nafter:\n%s", value, before, after)
		}
	}
}

func nativeChromeValues() []string {
	return []string{"CLEAR-ME", "[Test Hall]", "L: shield", "R: sword", "o---@", "READY |", "DISCONNECTED", "Help", "Theme", "Map navigation", "Command", "Whisper", "default", "dark", "high-contrast", "proof-chrome"}
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
