package tea

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestInsertAboveScrollsEveryPreviouslyPrintedRow(t *testing.T) {
	for _, lineWidth := range []int{61, 120} {
		t.Run(fmt.Sprintf("width-%d", lineWidth), func(t *testing.T) {
			var output bytes.Buffer
			renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 60, 15)

			if err := renderer.insertAbove(strings.Repeat("x", lineWidth)); err != nil {
				t.Fatalf("insert wrapped line: %v", err)
			}
			output.Reset()

			if err := renderer.insertAbove("next"); err != nil {
				t.Fatalf("insert next line: %v", err)
			}
			want := "\r" + ansi.CursorDown(14) + "\n\n" + ansi.CursorUp(16)
			if !strings.HasPrefix(output.String(), want) {
				t.Fatalf("next insertion prefix %q, want %q", output.String(), want)
			}
		})
	}
}

func TestInsertAboveWriteFailureDoesNotAdvancePrintedRows(t *testing.T) {
	var output bytes.Buffer
	renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 60, 15)
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
	want := "\r" + ansi.CursorDown(14) + "\n\n" + ansi.CursorUp(16)
	if !strings.HasPrefix(output.String(), want) {
		t.Fatalf("retry prefix %q, want %q", output.String(), want)
	}
}

func TestInsertAboveUsesResizedWidthBeforeFlush(t *testing.T) {
	var output bytes.Buffer
	renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 100, 15)
	renderer.resize(40, 15)

	if err := renderer.insertAbove(strings.Repeat("x", 41)); err != nil {
		t.Fatalf("insert after resize: %v", err)
	}
	want := "\n\n" + ansi.CursorUp(16) + ansi.InsertLine(2)
	if !strings.Contains(output.String(), want) {
		t.Fatalf("post-resize insertion %q does not contain %q", output.String(), want)
	}
}

func TestInsertAboveMovesFromPreviousRowsBeforeWrappedLine(t *testing.T) {
	var output bytes.Buffer
	renderer := newCursedRenderer(&output, []string{"TERM=xterm-256color"}, 60, 15)
	if err := renderer.insertAbove("short"); err != nil {
		t.Fatalf("insert short line: %v", err)
	}
	output.Reset()

	if err := renderer.insertAbove(strings.Repeat("x", 61)); err != nil {
		t.Fatalf("insert wrapped line: %v", err)
	}
	want := "\r" + ansi.CursorDown(14) + "\n" + ansi.CursorUp(15) + ansi.InsertLine(2)
	if !strings.HasPrefix(output.String(), want) {
		t.Fatalf("wrapped insertion prefix %q, want %q", output.String(), want)
	}
}

func TestResizeReanchorsFirstRedrawnRow(t *testing.T) {
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
	if !strings.HasPrefix(output.String(), "\r"+ansi.EraseScreenBelow+"new top") {
		t.Fatalf("resized redraw did not reanchor first row: %q", output.String())
	}
}
