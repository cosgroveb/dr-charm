package tea

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

type printErrorRenderer struct {
	nilRenderer
	flushErr    error
	insertErr   error
	insertCalls int
}

func (r *printErrorRenderer) flush(bool) error { return r.flushErr }

func (r *printErrorRenderer) insertAbove(string) error {
	r.insertCalls++
	return r.insertErr
}

type printErrorModel struct{ updates int }

func (printErrorModel) Init() Cmd { return nil }

func (m printErrorModel) Update(Msg) (Model, Cmd) {
	m.updates++
	return m, nil
}

func (printErrorModel) View() View { return NewView("") }

func TestPrintLineMessageReturnsRendererErrors(t *testing.T) {
	for _, test := range []struct {
		name        string
		flushErr    error
		insertErr   error
		wantInserts int
	}{
		{name: "flush", flushErr: errors.New("flush failed"), wantInserts: 0},
		{name: "insert", insertErr: errors.New("insert failed"), wantInserts: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			renderer := &printErrorRenderer{flushErr: test.flushErr, insertErr: test.insertErr}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			program := &Program{
				ctx:      ctx,
				msgs:     make(chan Msg, 3),
				errs:     make(chan error),
				renderer: renderer,
			}
			program.msgs <- printLineMessage{messageBody: "record"}
			program.msgs <- "must not update"
			program.msgs <- QuitMsg{}

			model, err := program.eventLoop(printErrorModel{}, make(chan Cmd, 3))
			wantErr := test.flushErr
			if wantErr == nil {
				wantErr = test.insertErr
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("event loop error=%v, want %v", err, wantErr)
			}
			if renderer.insertCalls != test.wantInserts {
				t.Fatalf("insert calls=%d, want %d", renderer.insertCalls, test.wantInserts)
			}
			if updates := model.(printErrorModel).updates; updates != 0 {
				t.Fatalf("model processed %d messages after renderer error", updates)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func newInsertAboveTestRenderer(t *testing.T, output *bytes.Buffer, width int) *cursedRenderer {
	t.Helper()
	renderer := newCursedRenderer(output, []string{"TERM=xterm-256color"}, width, 30)
	renderer.render(NewView(strings.Repeat("\n", 14)))
	if err := renderer.flush(false); err != nil {
		t.Fatalf("flush test frame: %v", err)
	}
	output.Reset()
	return renderer
}

func TestInsertAboveUsesIncomingPhysicalRows(t *testing.T) {
	for _, lineWidth := range []int{61, 120} {
		t.Run(fmt.Sprintf("width-%d", lineWidth), func(t *testing.T) {
			var output bytes.Buffer
			renderer := newInsertAboveTestRenderer(t, &output, 60)

			if err := renderer.insertAbove(strings.Repeat("x", lineWidth)); err != nil {
				t.Fatalf("insert wrapped line: %v", err)
			}
			output.Reset()

			if err := renderer.insertAbove("next"); err != nil {
				t.Fatalf("insert next line: %v", err)
			}
			want := "\r" + ansi.CursorDown(14) + "\n" + ansi.CursorUp(15)
			if !strings.HasPrefix(output.String(), want) {
				t.Fatalf("next insertion prefix %q, want %q", output.String(), want)
			}
		})
	}
}

func TestInsertAboveWriteFailureDoesNotAffectNextInsertion(t *testing.T) {
	var output bytes.Buffer
	renderer := newInsertAboveTestRenderer(t, &output, 60)
	if err := renderer.insertAbove(strings.Repeat("x", 61)); err != nil {
		t.Fatalf("insert initial line: %v", err)
	}

	renderer.w = failingWriter{}
	if err := renderer.insertAbove(strings.Repeat("x", 121)); err == nil {
		t.Fatal("failed writer returned nil")
	}
	output.Reset()
	renderer.w = &output
	if err := renderer.insertAbove("next"); err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
	want := "\r" + ansi.CursorDown(14) + "\n" + ansi.CursorUp(15)
	if !strings.HasPrefix(output.String(), want) {
		t.Fatalf("retry prefix %q, want %q", output.String(), want)
	}
}

func TestInsertAboveUsesResizedWidthBeforeFlush(t *testing.T) {
	var output bytes.Buffer
	renderer := newInsertAboveTestRenderer(t, &output, 100)
	renderer.resize(40, 30)
	renderer.render(NewView(strings.Repeat("\n", 14)))
	if err := renderer.flush(false); err != nil {
		t.Fatalf("flush resized frame: %v", err)
	}
	output.Reset()

	if err := renderer.insertAbove(strings.Repeat("x", 41)); err != nil {
		t.Fatalf("insert after resize: %v", err)
	}
	want := "\n\n" + ansi.CursorUp(16) + ansi.InsertLine(2)
	if !strings.Contains(output.String(), want) {
		t.Fatalf("post-resize insertion %q does not contain %q", output.String(), want)
	}
}

func TestInsertAboveUsesWrappedIncomingRows(t *testing.T) {
	var output bytes.Buffer
	renderer := newInsertAboveTestRenderer(t, &output, 60)
	if err := renderer.insertAbove("short"); err != nil {
		t.Fatalf("insert short line: %v", err)
	}
	output.Reset()

	if err := renderer.insertAbove(strings.Repeat("x", 61)); err != nil {
		t.Fatalf("insert wrapped line: %v", err)
	}
	want := "\r" + ansi.CursorDown(14) + "\n\n" + ansi.CursorUp(16) + ansi.InsertLine(2)
	if !strings.HasPrefix(output.String(), want) {
		t.Fatalf("wrapped insertion prefix %q, want %q", output.String(), want)
	}
}

func TestInsertAbovePreservesTabBytes(t *testing.T) {
	var output bytes.Buffer
	renderer := newInsertAboveTestRenderer(t, &output, 20)
	record := "TAB-ONE 1234567\tABCDEFGHIJKLM TAB-TAIL"

	if err := renderer.insertAbove(record); err != nil {
		t.Fatalf("insert tab record: %v", err)
	}
	if got := output.String(); !strings.Contains(got, record) {
		t.Fatalf("inserted output %q does not contain raw record %q", got, record)
	}
}

func TestTerminalLineWidthUsesEightColumnTabStops(t *testing.T) {
	for _, test := range []struct {
		name  string
		line  string
		width int
		want  int
	}{
		{name: "tab advances one cell", line: "1234567\tABCDEFGHIJKLM", width: 20, want: 21},
		{name: "tab advances eight cells", line: "12345678\tA", width: 20, want: 17},
		{name: "multiple tabs", line: "1\t2\t3", width: 20, want: 17},
		{name: "tab after wrapped text", line: strings.Repeat("x", 23) + "\t界", width: 20, want: 30},
		{name: "tab clamps at margin", line: strings.Repeat("x", 17) + "\t", width: 20, want: 19},
		{name: "print in clamped margin", line: strings.Repeat("x", 17) + "\tA", width: 20, want: 20},
		{name: "tab preserves delayed wrap", line: strings.Repeat("x", 20) + "\tA", width: 20, want: 21},
		{name: "wide print after clamped margin", line: strings.Repeat("x", 17) + "\t界", width: 19, want: 21},
		{name: "ANSI ignored", line: "\x1b[31m1234567\x1b[0m\tA", width: 20, want: 9},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := terminalLineWidth(test.line, test.width); got != test.want {
				t.Fatalf("terminalLineWidth(%q, %d)=%d, want %d", test.line, test.width, got, test.want)
			}
		})
	}
}

func TestShouldStreamAboveUsesRowsOutsideFrame(t *testing.T) {
	const width, terminalHeight, frameHeight = 10, 30, 6
	capacity := terminalHeight - frameHeight
	for _, test := range []struct {
		name string
		line string
		want bool
	}{
		{name: "one row below capacity", line: strings.Repeat("x", (capacity-1)*width), want: false},
		{name: "at capacity", line: strings.Repeat("x", capacity*width), want: false},
		{name: "one row over capacity", line: strings.Repeat("x", capacity*width+1), want: true},
		{name: "several screens", line: strings.Repeat("x", terminalHeight*width*3), want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldStreamAbove([]string{test.line}, width, terminalHeight, frameHeight); got != test.want {
				t.Fatalf("shouldStreamAbove(%d cells)=%v, want %v", len(test.line), got, test.want)
			}
		})
	}
	if !shouldStreamAbove([]string{"short", strings.Repeat("x", capacity*width+1), "", "last"}, width, terminalHeight, frameHeight) {
		t.Fatal("mixed message with oversized logical line did not select streaming")
	}
	if shouldStreamAbove([]string{"x"}, width, terminalHeight, 0) {
		t.Fatal("empty frame selected streaming")
	}
	if !shouldStreamAbove([]string{"x"}, width, terminalHeight, terminalHeight) {
		t.Fatal("full-height frame did not select streaming")
	}
}

func TestInsertAboveStreamsOversizedMessageAndInvalidatesFrame(t *testing.T) {
	var output bytes.Buffer
	renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 10, 8)
	renderer.render(NewView("FRAME-TOP\nFRAME-MID\nFRAME-END"))
	if err := renderer.flush(false); err != nil {
		t.Fatalf("flush frame: %v", err)
	}
	output.Reset()
	message := "short\n" + strings.Repeat("x", 51) + "\n\nlast"
	if err := renderer.insertAbove(message); err != nil {
		t.Fatalf("stream oversized message: %v", err)
	}
	got := output.String()
	if !strings.Contains(got, message[:strings.IndexByte(message, '\n')]+"\r\n") ||
		!strings.Contains(got, strings.Repeat("x", 51)+"\r\n\r\nlast\r\n") {
		t.Fatalf("streamed output changed logical content: %q", got)
	}
	if strings.Count(got, ansi.EraseEntireLine) != 3 {
		t.Fatalf("owned frame erases=%d, want 3: %q", strings.Count(got, ansi.EraseEntireLine), got)
	}
	if !renderer.pendingErase {
		t.Fatal("oversized stream did not invalidate unchanged frame")
	}
	output.Reset()
	if err := renderer.flush(false); err != nil {
		t.Fatalf("redraw unchanged frame: %v", err)
	}
	if renderer.pendingErase || !strings.Contains(output.String(), "FRAME-TOP") {
		t.Fatalf("unchanged frame was not redrawn: pending=%v output=%q", renderer.pendingErase, output.String())
	}
}

func TestInsertAboveOversizedWriteFailureDoesNotClaimRedraw(t *testing.T) {
	var output bytes.Buffer
	renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 10, 8)
	renderer.render(NewView("top\nmid\nbottom"))
	if err := renderer.flush(false); err != nil {
		t.Fatalf("flush frame: %v", err)
	}
	renderer.w = failingWriter{}
	if err := renderer.insertAbove(strings.Repeat("x", 51)); err == nil {
		t.Fatal("failed oversized writer returned nil")
	}
	if renderer.pendingErase {
		t.Fatal("failed oversized write claimed a pending redraw")
	}
}

func TestWriteFrameEraseSingleRowHasNoMovement(t *testing.T) {
	var output strings.Builder
	writeFrameErase(&output, 1, 0)
	if got, want := output.String(), "\r"+ansi.EraseEntireLine+"\r"; got != want {
		t.Fatalf("single-row frame erase=%q, want %q", got, want)
	}
}

func TestResizeErasesOnlyOwnedRowsBeforeFullRedraw(t *testing.T) {
	var output bytes.Buffer
	renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 100, 30)
	renderer.render(NewView("old top\nold bottom"))
	if err := renderer.flush(false); err != nil {
		t.Fatalf("flush initial view: %v", err)
	}
	output.Reset()
	renderer.scr.SetPosition(0, 0)

	renderer.resize(44, 30)
	renderer.render(NewView("new top\nnew bottom"))
	if err := renderer.flush(false); err != nil {
		t.Fatalf("flush resized view: %v", err)
	}
	got := output.String()
	if strings.Contains(got, ansi.EraseScreenBelow) {
		t.Fatalf("resized redraw used unbounded erase: %q", got)
	}
	wantErase := ansi.SaveCursor + ansi.EraseEntireLine + ansi.CursorDown(1) + ansi.EraseEntireLine + ansi.RestoreCursor
	if !strings.HasPrefix(got, wantErase) || !strings.Contains(got, "new top") {
		t.Fatalf("resized redraw did not erase two owned rows before redraw: %q", got)
	}
}

func TestResizePreservesLegacyPathForFlushedVisibleCursor(t *testing.T) {
	for _, height := range []int{10, 1} {
		t.Run(fmt.Sprintf("height-%d", height), func(t *testing.T) {
			var output bytes.Buffer
			renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 20, 10)
			renderer.render(View{Content: "first\nsecond\nthird", Cursor: NewCursor(3, 1)})
			if err := renderer.flush(false); err != nil {
				t.Fatalf("flush visible cursor: %v", err)
			}
			output.Reset()

			renderer.resize(21, height)
			if renderer.resizeErased || strings.Contains(renderer.buf.String(), ansi.SaveCursor) {
				t.Fatalf("visible cursor used bounded inline erase: %q", renderer.buf.String())
			}
			renderer.render(View{Content: "first\nsecond\nthird", Cursor: NewCursor(3, 1)})
			if err := renderer.flush(false); err != nil {
				t.Fatalf("flush resized visible cursor: %v", err)
			}
		})
	}
}

func TestResizeUsesFlushedCursorOwnership(t *testing.T) {
	var output bytes.Buffer
	renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 20, 10)
	renderer.render(NewView("first\nsecond"))
	if err := renderer.flush(false); err != nil {
		t.Fatalf("flush hidden cursor: %v", err)
	}
	renderer.render(View{Content: "first\nsecond", Cursor: NewCursor(1, 1)})
	renderer.resize(21, 10)
	if !renderer.resizeErased {
		t.Fatal("pending visible cursor changed ownership of flushed hidden-cursor frame")
	}
}

func TestResizeErasesFlushedHiddenCursorFrameRows(t *testing.T) {
	for _, test := range []struct {
		name    string
		flushed string
		pending string
		want    int
	}{
		{name: "pending taller", flushed: "one\ntwo", pending: "one\ntwo\nthree\nfour", want: 2},
		{name: "pending shorter", flushed: "one\ntwo\nthree\nfour", pending: "one\ntwo", want: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 20, 10)
			renderer.render(NewView(test.flushed))
			if err := renderer.flush(false); err != nil {
				t.Fatalf("flush physical frame: %v", err)
			}
			output.Reset()
			renderer.render(NewView(test.pending))

			renderer.resize(21, 10)
			if got := strings.Count(renderer.buf.String(), ansi.EraseEntireLine); got != test.want {
				t.Fatalf("bounded erase rows=%d, want flushed frame rows %d: %q", got, test.want, renderer.buf.String())
			}
		})
	}
}

func TestResizeWithOnlyPendingHiddenCursorViewOwnsNoRows(t *testing.T) {
	var output bytes.Buffer
	renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 20, 10)
	renderer.render(NewView("one\ntwo"))

	renderer.resize(21, 10)
	if got := strings.Count(renderer.buf.String(), ansi.EraseEntireLine); got != 0 {
		t.Fatalf("unflushed pending view erased %d rows: %q", got, renderer.buf.String())
	}
}

func TestResizePreservesAlternateScreenPath(t *testing.T) {
	var output bytes.Buffer
	renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 20, 10)
	view := NewView("first\nsecond")
	view.AltScreen = true
	renderer.render(view)
	if err := renderer.flush(false); err != nil {
		t.Fatalf("flush alternate screen: %v", err)
	}
	output.Reset()

	renderer.resize(21, 5)
	if renderer.resizeErased || strings.Contains(renderer.buf.String(), ansi.SaveCursor) {
		t.Fatalf("alternate screen used bounded inline erase: %q", renderer.buf.String())
	}
	renderer.render(view)
	if err := renderer.flush(false); err != nil {
		t.Fatalf("flush resized alternate screen: %v", err)
	}
}

func TestWideGlyphAtRightMarginCountsPhysicalRows(t *testing.T) {
	line := strings.Repeat("a", 19) + "界" + strings.Repeat("b", 19)
	if got := terminalLineWidth(line, 20); got != 41 {
		t.Fatalf("effective width=%d, want 41", got)
	}
	if !shouldStreamAbove([]string{line}, 20, 5, 3) {
		t.Fatal("three-row wide-glyph line did not stream above two-row capacity")
	}
	if got := resizedFrameRows(line, 20, 10); got != 3 {
		t.Fatalf("resized frame rows=%d, want 3", got)
	}
}

func TestInsertAboveUsesWideGlyphPhysicalRows(t *testing.T) {
	var output bytes.Buffer
	renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 20, 10)
	renderer.render(NewView("top\nmiddle\nbottom"))
	if err := renderer.flush(false); err != nil {
		t.Fatalf("flush frame: %v", err)
	}
	output.Reset()
	line := strings.Repeat("a", 19) + "界" + strings.Repeat("b", 19)
	if err := renderer.insertAbove(line); err != nil {
		t.Fatalf("insert right-margin wide glyph: %v", err)
	}
	if got := output.String(); !strings.Contains(got, strings.Repeat("\n", 3)+ansi.CursorUp(5)+ansi.InsertLine(3)) {
		t.Fatalf("wide-glyph insertion did not reserve three rows: %q", got)
	}
}

func TestResizedFrameRowsClampsToVisibleTerminal(t *testing.T) {
	content := strings.Repeat("x", 100) + "\n" + strings.Repeat("y", 100)
	if got := resizedFrameRows(content, 40, 5); got != 5 {
		t.Fatalf("resized frame rows=%d, want visible height 5", got)
	}
	if got := resizedFrameRows(content, 0, 5); got != 0 {
		t.Fatalf("zero-width resized frame rows=%d, want 0", got)
	}
}
