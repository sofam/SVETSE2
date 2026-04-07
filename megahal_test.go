package main

import (
	"testing"
)

func TestMakeWords(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{
			name:   "simple sentence",
			input:  "Hello world",
			expect: []string{"HELLO", " ", "WORLD", "."},
		},
		{
			name:   "with punctuation",
			input:  "Hello, world!",
			expect: []string{"HELLO", ", ", "WORLD", "!"},
		},
		{
			name:   "apostrophe stays in word",
			input:  "don't stop",
			expect: []string{"DON'T", " ", "STOP", "."},
		},
		{
			name:   "emoji as token",
			input:  "hello 🎉 world",
			expect: []string{"HELLO", " 🎉 ", "WORLD", "."},
		},
		{
			name:   "unicode letters",
			input:  "café résumé",
			expect: []string{"CAFÉ", " ", "RÉSUMÉ", "."},
		},
		{
			name:   "mixed digits and letters",
			input:  "abc123 def",
			expect: []string{"ABC", "123", " ", "DEF", "."},
		},
		{
			name:   "empty string",
			input:  "",
			expect: nil,
		},
		{
			name:   "trailing punctuation preserved",
			input:  "hello world?",
			expect: []string{"HELLO", " ", "WORLD", "?"},
		},
		{
			name:   "CJK characters",
			input:  "hello 你好 world",
			expect: []string{"HELLO", " ", "你好", " ", "WORLD", "."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeWords(tt.input)
			if len(got) != len(tt.expect) {
				t.Fatalf("makeWords(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.expect, len(tt.expect))
			}
			for i := range got {
				if got[i] != tt.expect[i] {
					t.Errorf("makeWords(%q)[%d] = %q, want %q",
						tt.input, i, got[i], tt.expect[i])
				}
			}
		})
	}
}
