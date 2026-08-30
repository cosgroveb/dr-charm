package terminaltext

import "testing"

func TestSanitize(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"osc52", "a\x1b]52;c;secret\a b", "a b"},
		{"osc52 string terminator", "a\x1b]52;c;secret\x1b\\b", "ab"},
		{"kitty", "a\x1bP@kitty-cmd{\"x\":1}\x1b\\b", "ab"},
		{"csi", "a\x1b[31mred\x1b[0mb", "aredb"},
		{"osc8", "\x1b]8;;https://x\ahello\x1b]8;;\a", "hello"},
		{"c1 osc", "a\x9d52;c;secret\x9cb", "ab"},
		{"c1 osc bel", "a\x9d52;c;secret\ab", "ab"},
		{"c1 dcs", "a\x90@kitty-cmd\x9cb", "ab"},
		{"c1 csi", "a\x9b31mb", "ab"},
		{"c1 st", "a\x9cb", "ab"},
		{"controls", "a\x00\x7f\x9bb", "a"},
		{"literal angle brackets", "a <not markup> b", "a <not markup> b"},
		{"newlines", "a\r\nb\rc", "a\nb\nc"},
		{"allowed", "λ\tline\n", "λ\tline\n"},
		{"invalid", string([]byte{'a', 0xff, 'b'}), "a�b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sanitize(tt.in); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeLeavesNoUnsafeControls(t *testing.T) {
	in := "text\x00\x01\x02\x07\x08\x0b\x0c\x0e\x1f\x7f\x80\x81\x8f\x90\x9b\x9d\x9f\x9ctext"
	for _, r := range Sanitize(in) {
		if r != '\t' && r != '\n' && (r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f) {
			t.Fatalf("unsafe rune U+%04X in %q", r, Sanitize(in))
		}
	}
}
