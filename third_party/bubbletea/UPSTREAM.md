# Bubble Tea local correction

Source: `charm.land/bubbletea/v2` v2.0.9.

The root Go package is copied unchanged except for `cursed_renderer.go` and
`tea.go`. The local changes keep the inline frame origin consistent after
terminal reflow and transcript insertion. `tea.go` flushes the pending frame
before insertion so the renderer uses its realized height.

`cursed_renderer.go` counts each incoming logical line's physical rows,
including default eight-column TAB advancement and the unused margin cell when
a wide grapheme wraps before drawing. Lines that fit above the frame use
terminal line insertion. A larger message clears only the owned frame, streams
the original text through native wrapping, reserves the frame rows, and
invalidates the cached frame for redraw. The renderer does not change terminal
tab stops or clear scrollback. `LICENSE` is Bubble Tea's upstream MIT license.

Remove this module replacement when an upstream release contains the same
inline-resize correction and the native terminal proof passes against it.
