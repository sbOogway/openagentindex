package tui

import "testing"

func TestClean(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain text", in: "Echo Agent", want: "Echo Agent"},
		{name: "unicode", in: "Café 🤖 エージェント", want: "Café 🤖 エージェント"},
		{name: "colour", in: "\x1b[31mred\x1b[0m", want: "red"},
		{name: "cursor move and clear screen", in: "a\x1b[2J\x1b[Hb", want: "ab"},
		{name: "window title", in: "x\x1b]0;pwned\x07y", want: "xy"},
		{name: "hyperlink", in: "\x1b]8;;https://evil.example\x1b\\click\x1b]8;;\x1b\\", want: "click"},
		{name: "bell and null", in: "a\x07b\x00c", want: "abc"},
		{name: "C1 control", in: "a\u009b31mb", want: "a31mb"},
		{name: "bidi override", in: "user\u202egnp.exe", want: "usergnp.exe"},
		{name: "new lines and tabs", in: "line 1\nline 2\r\n\tend", want: "line 1 line 2 end"},
		{name: "spaces", in: "  a   b  ", want: "a b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clean(tt.in); got != tt.want {
				t.Errorf("clean(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanMarkdown(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "keeps lines and indentation", in: "# Title\n\n    code\n- item", want: "# Title\n\n    code\n- item"},
		{name: "tab to spaces", in: "a\tb", want: "a    b"},
		{name: "escape sequences", in: "**x**\x1b]0;pwned\x07\x1b[2J", want: "**x**"},
		{name: "control characters", in: "a\x00b\rc\x07", want: "abc"},
		{name: "bidi override", in: "user\u202egnp.exe", want: "usergnp.exe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanMarkdown(tt.in); got != tt.want {
				t.Errorf("cleanMarkdown(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStripHyperlinks(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{in: "\x1b]8;id=1;https://evil.example\x1b\\click\x1b]8;;\x1b\\", want: "click"},
		{in: "\x1b]8;;https://evil.example\x07click\x1b]8;;\x07", want: "click"},
		{in: "\x1b[1mbold\x1b[0m", want: "\x1b[1mbold\x1b[0m"},
	}
	for _, tt := range tests {
		if got := stripHyperlinks(tt.in); got != tt.want {
			t.Errorf("stripHyperlinks(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
