package main

import (
	"os"
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

func TestNewModel(t *testing.T) {
	m := newModel(5)
	if m.Order != 5 {
		t.Errorf("Order = %d, want 5", m.Order)
	}
	if len(m.Dictionary) != 1 {
		t.Errorf("Dictionary should have 1 entry (boundary), got %d", len(m.Dictionary))
	}
	if m.Dictionary[0] != "" {
		t.Errorf("Dictionary[0] = %q, want %q", m.Dictionary[0], "")
	}
}

func TestAddAndFindWord(t *testing.T) {
	m := newModel(5)
	id1 := m.addWord("HELLO")
	if id1 != 1 {
		t.Errorf("first addWord = %d, want 1", id1)
	}
	id2 := m.addWord("HELLO")
	if id2 != id1 {
		t.Errorf("duplicate addWord = %d, want %d", id2, id1)
	}
	found := m.findWord("HELLO")
	if found != id1 {
		t.Errorf("findWord(HELLO) = %d, want %d", found, id1)
	}
	found = m.findWord("NOPE")
	if found != 0 {
		t.Errorf("findWord(NOPE) = %d, want 0", found)
	}
	id3 := m.addWord("WORLD")
	if id3 != 2 {
		t.Errorf("second addWord = %d, want 2", id3)
	}
}

func TestAddSymbol(t *testing.T) {
	node := newNode()
	child := addSymbol(node, 5)
	if child.Symbol != 5 {
		t.Errorf("child.Symbol = %d, want 5", child.Symbol)
	}
	if child.Count != 1 {
		t.Errorf("child.Count = %d, want 1", child.Count)
	}
	if node.Usage != 1 {
		t.Errorf("node.Usage = %d, want 1", node.Usage)
	}
	child2 := addSymbol(node, 5)
	if child2 != child {
		t.Error("should return the same child node")
	}
	if child.Count != 2 {
		t.Errorf("child.Count = %d, want 2", child.Count)
	}
	child3 := addSymbol(node, 3)
	if child3.Symbol != 3 {
		t.Errorf("child3.Symbol = %d, want 3", child3.Symbol)
	}
	if len(node.Children) != 2 {
		t.Errorf("len(Children) = %d, want 2", len(node.Children))
	}
	if node.Children[0].Symbol != 3 || node.Children[1].Symbol != 5 {
		t.Errorf("Children not sorted: [%d, %d]", node.Children[0].Symbol, node.Children[1].Symbol)
	}
}

func TestLearn(t *testing.T) {
	m := newModel(5)
	m.learn("Hello world")
	// "Hello world" -> tokens: HELLO, " ", WORLD, "." -> 4 tokens, dictionary: "", HELLO, " ", WORLD, "."
	if len(m.Dictionary) != 5 {
		t.Fatalf("Dictionary size = %d, want 5, got %v", len(m.Dictionary), m.Dictionary)
	}
	if len(m.Forward.Children) == 0 {
		t.Error("Forward tree has no children after learning")
	}
	if len(m.Backward.Children) == 0 {
		t.Error("Backward tree has no children after learning")
	}
	m.learn("Hello world")
	if len(m.Dictionary) != 5 {
		t.Errorf("Dictionary size after re-learn = %d, want 5", len(m.Dictionary))
	}
	if m.Forward.Usage < 2 {
		t.Errorf("Forward.Usage = %d, want >= 2", m.Forward.Usage)
	}
}

func TestLearnShortInput(t *testing.T) {
	m := newModel(5)
	m.learn("Hi")
	// "Hi" -> tokens: HI, "." -> 2 tokens, which is <= order 5, so skipped
	if len(m.Forward.Children) != 0 {
		t.Error("Short input should not be learned")
	}
}

func TestLoadWordList(t *testing.T) {
	f, err := os.CreateTemp("", "ban-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("THE\nA\nIS\n")
	f.Close()
	words := loadWordList(f.Name())
	if len(words) != 3 {
		t.Fatalf("loadWordList got %d words, want 3", len(words))
	}
	if !words["THE"] || !words["A"] || !words["IS"] {
		t.Errorf("unexpected words: %v", words)
	}
}

func TestLoadWordListMissing(t *testing.T) {
	words := loadWordList("/nonexistent/file.txt")
	if words == nil {
		t.Error("missing file should return empty map, not nil")
	}
	if len(words) != 0 {
		t.Errorf("missing file should return empty map, got %d entries", len(words))
	}
}

func TestLoadSwapList(t *testing.T) {
	f, err := os.CreateTemp("", "swp-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("MY\nYOUR\nI'M\nYOU'RE\n")
	f.Close()
	swaps := loadSwapList(f.Name())
	if len(swaps) != 2 {
		t.Fatalf("loadSwapList got %d pairs, want 2", len(swaps))
	}
	if swaps["MY"] != "YOUR" {
		t.Errorf("swaps[MY] = %q, want YOUR", swaps["MY"])
	}
	if swaps["I'M"] != "YOU'RE" {
		t.Errorf("swaps[I'M] = %q, want YOU'RE", swaps["I'M"])
	}
}
