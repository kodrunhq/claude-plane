package telegram

import (
	"testing"
)

func TestEscapeMarkdownV2(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"hello.world", "hello\\.world"},
		{"test-123", "test\\-123"},
		{"a_b*c[d](e)~f>g#h+i=j|k{l}m!n", "a\\_b\\*c\\[d\\]\\(e\\)\\~f\\>g\\#h\\+i\\=j\\|k\\{l\\}m\\!n"},
	}
	for _, tt := range tests {
		got := escapeMarkdownV2(tt.input)
		if got != tt.want {
			t.Errorf("escapeMarkdownV2(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
