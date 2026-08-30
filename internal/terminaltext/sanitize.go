// Package terminaltext normalizes untrusted text before terminal presentation.
package terminaltext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Sanitize removes terminal controls, repairs UTF-8, and normalizes newlines.
func Sanitize(input string) string {
	var out strings.Builder
	for i := 0; i < len(input); {
		if input[i] >= 0x80 && input[i] <= 0x9f {
			i = consumeSequence(input, i, rune(input[i]), 1)
			continue
		}
		r, size := utf8.DecodeRuneInString(input[i:])
		if r == utf8.RuneError && size == 1 {
			r = utf8.RuneError
		}
		if r == '\x1b' || (r >= 0x80 && r <= 0x9f) {
			i = consumeSequence(input, i, r, size)
			continue
		}
		i += size
		switch r {
		case '\r':
			if i < len(input) && input[i] == '\n' {
				i++
			}
			out.WriteByte('\n')
		case '\n', '\t':
			out.WriteRune(r)
		default:
			if r == utf8.RuneError || (unicode.IsPrint(r) && !unicode.IsControl(r)) {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}

func consumeSequence(s string, start int, r rune, size int) int {
	i := start + size
	if r == '\x1b' {
		if i >= len(s) {
			return i
		}
		switch s[i] {
		case '\\': // String terminator (ST).
			i++
		case '[': // CSI: consume through its final byte.
			i++
			for i < len(s) {
				b := s[i]
				i++
				if b >= 0x40 && b <= 0x7e {
					break
				}
			}
		case ']', 'P', '^', '_', 'X': // OSC/DCS/PM/APC/SOS.
			i++
			for i < len(s) {
				if s[i] == '\a' {
					return i + 1
				}
				if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
					return i + 2
				}
				i++
			}
		default:
			i++
		}
		return i
	}
	// C1 CSI/OSC/DCS/ST.
	switch r {
	case 0x9b:
		for i < len(s) {
			b := s[i]
			i++
			if b >= 0x40 && b <= 0x7e {
				break
			}
		}
	case 0x9d, 0x90:
		for i < len(s) {
			if s[i] == '\a' || s[i] == 0x9c {
				return i + 1
			}
			i++
		}
	}
	return i
}
