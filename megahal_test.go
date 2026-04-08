package main

import (
	"os"
	"strings"
	"testing"
	"time"
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

func TestMakeKeywords(t *testing.T) {
	m := newModel(5)
	ban := map[string]bool{"THE": true, "A": true, "IS": true}
	aux := map[string]bool{"IT": true, "THIS": true}
	swaps := map[string]string{"MY": "YOUR"}

	m.addWord("HELLO")
	m.addWord("WORLD")
	m.addWord("THE")
	m.addWord("IT")
	m.addWord("MY")
	m.addWord("YOUR")

	tokens := makeWords("the hello world")
	keys := m.makeKeywords(tokens, ban, aux, swaps)

	found := make(map[string]bool)
	for _, k := range keys {
		found[k] = true
	}
	if found["THE"] {
		t.Error("THE should be banned from keywords")
	}
	if !found["HELLO"] {
		t.Error("HELLO should be a keyword")
	}
	if !found["WORLD"] {
		t.Error("WORLD should be a keyword")
	}
}

func TestMakeKeywordsSwap(t *testing.T) {
	m := newModel(5)
	ban := map[string]bool{}
	aux := map[string]bool{}
	swaps := map[string]string{"MY": "YOUR"}

	m.addWord("MY")
	m.addWord("YOUR")

	tokens := makeWords("my cat")
	keys := m.makeKeywords(tokens, ban, aux, swaps)

	found := make(map[string]bool)
	for _, k := range keys {
		found[k] = true
	}
	if found["MY"] {
		t.Error("MY should be swapped, not directly in keywords")
	}
	if !found["YOUR"] {
		t.Error("YOUR should appear as keyword (swapped from MY)")
	}
}

func TestGenerateReplyBasic(t *testing.T) {
	m := newModel(5)
	ban := map[string]bool{}
	aux := map[string]bool{}
	swaps := map[string]string{}

	sentences := []string{
		"The cat sat on the mat and looked at the birds",
		"The dog ran through the park chasing the ball",
		"Birds fly over the mountains and rivers below",
		"The fish swam in the river under the bridge",
		"Mountains rise above the clouds in the morning",
		"The cat chased the dog around the park today",
		"Rivers flow from the mountains to the sea below",
		"The ball bounced over the fence into the garden",
		"Gardens grow with flowers and trees in spring",
		"The bridge crosses over the river near the park",
	}
	for _, s := range sentences {
		m.learn(s)
	}

	cfg := GenerationConfig{
		Temperature:  1.0,
		SurpriseBias: 1.0,
		ReplyTimeout: 1 * time.Second,
	}

	reply := m.generateReply("cat", ban, aux, swaps, cfg)
	if reply == "" {
		t.Error("generateReply returned empty string")
	}
	t.Logf("Reply: %s", reply)
}

func TestGenerateReplyEmptyBrain(t *testing.T) {
	m := newModel(5)
	ban := map[string]bool{}
	aux := map[string]bool{}
	swaps := map[string]string{}
	cfg := GenerationConfig{
		Temperature:  1.0,
		SurpriseBias: 1.0,
		ReplyTimeout: 500 * time.Millisecond,
	}

	reply := m.generateReply("hello", ban, aux, swaps, cfg)
	if reply == "" {
		t.Error("generateReply should return a fallback message for empty brain")
	}
}

func TestParseOverrides(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
		wantKV   map[string]string
		wantHelp bool
	}{
		{"no overrides", "hello world", "hello world", map[string]string{}, false},
		{"chaos override", "hello !CHAOS=1.5 world", "hello world", map[string]string{"CHAOS": "1.5"}, false},
		{"multiple overrides", "test !TEMPERATURE=2.0 !TIMEOUT=5s message", "test message", map[string]string{"TEMPERATURE": "2.0", "TIMEOUT": "5s"}, false},
		{"help flag", "!HELP", "", map[string]string{}, true},
		{"help with other text", "hello !HELP world", "hello world", map[string]string{}, true},
		{"case insensitive keys", "test !chaos=2.0", "test", map[string]string{"CHAOS": "2.0"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, kv, help := parseOverrides(tt.input)
			text = strings.TrimSpace(text)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if help != tt.wantHelp {
				t.Errorf("help = %v, want %v", help, tt.wantHelp)
			}
			for k, v := range tt.wantKV {
				if kv[k] != v {
					t.Errorf("kv[%q] = %q, want %q", k, kv[k], v)
				}
			}
		})
	}
}

func TestApplyOverrides(t *testing.T) {
	base := GenerationConfig{
		Temperature:  1.0,
		SurpriseBias: 1.0,
		ReplyTimeout: 2 * time.Second,
	}
	cfg := applyOverrides(base, map[string]string{"CHAOS": "2.5"})
	if cfg.Temperature != 2.5 {
		t.Errorf("Temperature = %f, want 2.5", cfg.Temperature)
	}
	if cfg.SurpriseBias != 2.5 {
		t.Errorf("SurpriseBias = %f, want 2.5", cfg.SurpriseBias)
	}

	cfg = applyOverrides(base, map[string]string{"CHAOS": "2.0", "TEMPERATURE": "3.0"})
	if cfg.Temperature != 3.0 {
		t.Errorf("Temperature = %f, want 3.0", cfg.Temperature)
	}
	if cfg.SurpriseBias != 2.0 {
		t.Errorf("SurpriseBias = %f, want 2.0", cfg.SurpriseBias)
	}

	cfg = applyOverrides(base, map[string]string{"TIMEOUT": "60s"})
	if cfg.ReplyTimeout != 30*time.Second {
		t.Errorf("ReplyTimeout = %v, want 30s (capped)", cfg.ReplyTimeout)
	}
}

func TestFullRoundTrip(t *testing.T) {
	m := newModel(5)
	corpus := []string{
		"The cat sat on the mat and looked around the room",
		"The dog ran through the park and chased the birds away",
		"Birds fly high over the mountains and rivers below",
		"The fish swam in the river under the old stone bridge",
		"Mountains rise above the clouds every single morning",
		"The cat chased the dog around the park all afternoon",
		"Rivers flow from the mountains down to the big sea",
		"The ball bounced over the fence and into the garden",
		"Gardens grow with beautiful flowers and tall trees",
		"The bridge crosses over the wide river near the park",
		"Every morning the sun rises over the distant mountains",
		"The old stone bridge has been standing for many years",
		"Cats and dogs are the most popular pets in the world",
		"The park is full of trees and flowers in the spring",
		"Fish swim upstream in the river during spawning season",
	}
	for i := 0; i < 10; i++ {
		for _, s := range corpus {
			m.learn(s)
		}
	}

	dir := t.TempDir()
	path := dir + "/test.brain"
	if err := saveBrain(path, m); err != nil {
		t.Fatalf("saveBrain: %v", err)
	}

	m2 := newModel(5)
	if err := loadBrain(path, m2); err != nil {
		t.Fatalf("loadBrain: %v", err)
	}

	ban := map[string]bool{"THE": true, "A": true, "AND": true}
	aux := map[string]bool{}
	swaps := map[string]string{}
	cfg := GenerationConfig{
		Temperature:  1.0,
		SurpriseBias: 1.0,
		ReplyTimeout: 1 * time.Second,
	}

	reply := m2.generateReply("cat park", ban, aux, swaps, cfg)
	if reply == "" {
		t.Error("no reply generated from loaded brain")
	}
	t.Logf("Reply from loaded brain: %s", reply)

	cfg.Temperature = 3.0
	cfg.SurpriseBias = 2.0
	chaosReply := m2.generateReply("cat park", ban, aux, swaps, cfg)
	if chaosReply == "" {
		t.Error("no reply generated with high chaos")
	}
	t.Logf("High chaos reply: %s", chaosReply)
}
