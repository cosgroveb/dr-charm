# Bubble Tea local correction

Source: `charm.land/bubbletea/v2` v2.0.9.

The root Go package is copied unchanged except for `cursed_renderer.go`.
The local patch parks Bubble Tea's hidden inline hardware cursor at the
frame origin after each flush. It preserves the next repaint's origin after
terminal reflow on resize. `LICENSE` is Bubble Tea's upstream MIT license.

Remove this module replacement when an upstream release contains the same
inline-resize correction and the native terminal proof passes against it.
