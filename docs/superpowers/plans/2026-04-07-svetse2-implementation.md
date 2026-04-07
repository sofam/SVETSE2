# SVETSE2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a pure Go MegaHAL chatbot with Slack and Discord adapters, using surprise-maximizing reply generation with configurable chaos.

**Architecture:** Single binary, flat `package main`. A model goroutine owns all state, platform adapters send learn/reply requests via channels. Brain persists as custom binary format with uint32 fields and atomic saves.

**Tech Stack:** Go 1.23+, `github.com/slack-go/slack` (Socket Mode), `github.com/bwmarrin/discordgo`, Docker multi-stage build.

**Spec:** `docs/superpowers/specs/2026-04-07-svetse2-megahal-design.md`

**Reference C source:** `/opt/UnitySrc/slackseNET/slackseNET/SVETSE/megahal.c`

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `main.go`

- [ ] **Step 1: Initialize Go module**

Run:
```bash
cd /opt/UnitySrc/SVETSE2
go mod init github.com/oscelf/svetse2
```

- [ ] **Step 2: Create main.go with minimal structure**

Create `main.go`:

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "svetse2 starting...")
}
```

- [ ] **Step 3: Verify it compiles and runs**

Run: `go build -o svetse2 . && ./svetse2`
Expected: prints "svetse2 starting..." to stderr

- [ ] **Step 4: Commit**

```bash
git add go.mod main.go
git commit -m "feat: initialize Go module and main entry point"
```

---

### Task 2: Tokenization (make_words + boundary)

**Files:**
- Create: `megahal.go`
- Create: `megahal_test.go`

This implements the tokenizer that splits input into alternating word/separator tokens using unicode-aware boundary detection. Port of `make_words()` and `boundary()` from `megahal.c:1678-1800`.

The original C code uses `isalpha()` (ASCII-only). We replace with `unicode.IsLetter()` and `unicode.IsDigit()`. We also handle apostrophes within words (e.g., "don't" stays as one token), matching the original behavior.

Key behavior: if the last token is alphanumeric, append a "." token. If the last token is punctuation but doesn't end with `.`, `!`, or `?`, replace it with ".". This ensures sentences always end with terminal punctuation.

- [ ] **Step 1: Write tokenization tests**

Create `megahal_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestMakeWords -v ./...`
Expected: FAIL — `makeWords` not defined

- [ ] **Step 3: Implement tokenizer**

Create `megahal.go`:

```go
package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// makeWords splits input into alternating word/separator tokens.
// Words are sequences of letters (including apostrophes between letters)
// or sequences of digits. Everything else is separator.
// Input is uppercased. If the last token is alphanumeric, "." is appended.
// If the last token is punctuation not ending in .!?, it's replaced with ".".
func makeWords(input string) []string {
	input = strings.ToUpper(input)
	if len(input) == 0 {
		return nil
	}

	var tokens []string
	runes := []rune(input)
	i := 0

	for i < len(runes) {
		start := i
		if isWordRune(runes[i]) {
			// Consume a word: letters (with embedded apostrophes) or digits
			for i < len(runes) && isWordRuneContinue(runes, i) {
				i++
			}
		} else {
			// Consume a separator: everything that isn't a word char
			for i < len(runes) && !isWordRune(runes[i]) {
				i++
			}
		}
		tokens = append(tokens, string(runes[start:i]))
	}

	if len(tokens) == 0 {
		return nil
	}

	// Ensure sentence ends with terminal punctuation
	last := tokens[len(tokens)-1]
	firstRune, _ := utf8.DecodeRuneInString(last)
	if unicode.IsLetter(firstRune) || unicode.IsDigit(firstRune) {
		tokens = append(tokens, ".")
	} else {
		lastRune, _ := utf8.DecodeLastRuneInString(last)
		if lastRune != '.' && lastRune != '!' && lastRune != '?' {
			tokens[len(tokens)-1] = "."
		}
	}

	return tokens
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// isWordRuneContinue checks if runes[i] should continue the current word.
// Letters and digits continue. An apostrophe continues if it's between letters.
func isWordRuneContinue(runes []rune, i int) bool {
	r := runes[i]
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	if r == '\'' && i > 0 && i < len(runes)-1 {
		return unicode.IsLetter(runes[i-1]) && unicode.IsLetter(runes[i+1])
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestMakeWords -v ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add megahal.go megahal_test.go
git commit -m "feat: implement unicode-aware tokenizer (makeWords)"
```

---

### Task 3: Dictionary and Tree Data Structures

**Files:**
- Modify: `megahal.go`
- Modify: `megahal_test.go`

Implements the core data structures: Node (Markov tree node), Model (holds forward/backward trees + dictionary), and the dictionary operations (addWord, findWord) plus tree operations (searchNode, findSymbol, findSymbolAdd, addSymbol). Port of the C functions at `megahal.c:703-1221`.

- [ ] **Step 1: Write tests for dictionary and tree operations**

Append to `megahal_test.go`:

```go
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
	if m.Forward == nil || m.Backward == nil {
		t.Error("Forward/Backward trees should not be nil")
	}
}

func TestAddAndFindWord(t *testing.T) {
	m := newModel(5)

	// Adding a new word returns its symbol ID
	id1 := m.addWord("HELLO")
	if id1 != 1 {
		t.Errorf("first addWord = %d, want 1", id1)
	}

	// Adding the same word returns the same ID
	id2 := m.addWord("HELLO")
	if id2 != id1 {
		t.Errorf("duplicate addWord = %d, want %d", id2, id1)
	}

	// Finding an existing word
	found := m.findWord("HELLO")
	if found != id1 {
		t.Errorf("findWord(HELLO) = %d, want %d", found, id1)
	}

	// Finding a non-existent word returns 0
	found = m.findWord("NOPE")
	if found != 0 {
		t.Errorf("findWord(NOPE) = %d, want 0", found)
	}

	// Adding another word gets the next ID
	id3 := m.addWord("WORLD")
	if id3 != 2 {
		t.Errorf("second addWord = %d, want 2", id3)
	}
}

func TestAddSymbol(t *testing.T) {
	node := newNode()

	// Adding a symbol creates a child
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
	if len(node.Children) != 1 {
		t.Errorf("len(Children) = %d, want 1", len(node.Children))
	}

	// Adding the same symbol increments count
	child2 := addSymbol(node, 5)
	if child2 != child {
		t.Error("should return the same child node")
	}
	if child.Count != 2 {
		t.Errorf("child.Count = %d, want 2", child.Count)
	}
	if node.Usage != 2 {
		t.Errorf("node.Usage = %d, want 2", node.Usage)
	}

	// Adding a different symbol
	child3 := addSymbol(node, 3)
	if child3.Symbol != 3 {
		t.Errorf("child3.Symbol = %d, want 3", child3.Symbol)
	}
	if len(node.Children) != 2 {
		t.Errorf("len(Children) = %d, want 2", len(node.Children))
	}

	// Children should be sorted by symbol
	if node.Children[0].Symbol != 3 || node.Children[1].Symbol != 5 {
		t.Errorf("Children not sorted: [%d, %d]",
			node.Children[0].Symbol, node.Children[1].Symbol)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestNewModel|TestAddAndFindWord|TestAddSymbol" -v ./...`
Expected: FAIL — types/functions not defined

- [ ] **Step 3: Implement data structures and operations**

Add to `megahal.go` (after the tokenizer code):

```go
// Node is a node in the Markov tree.
type Node struct {
	Symbol   uint32
	Usage    uint32
	Count    uint32
	Children []*Node // sorted by Symbol
}

// Model holds the complete MegaHAL state.
type Model struct {
	Order      int
	Forward    *Node
	Backward   *Node
	Dictionary []string
	DictMap    map[string]uint32
	Context    []*Node
}

func newNode() *Node {
	return &Node{}
}

func newModel(order int) *Model {
	m := &Model{
		Order:    order,
		Forward:  newNode(),
		Backward: newNode(),
		DictMap:  make(map[string]uint32),
	}
	// Index 0 is the boundary/end-of-sentence symbol (empty string).
	// findWord returns 0 for unknown words, which also serves as boundary.
	m.Dictionary = append(m.Dictionary, "")
	// Context has order+2 slots (indices 0..order+1)
	m.Context = make([]*Node, order+2)
	return m
}

// addWord adds a word to the dictionary if not present, returns its symbol ID.
func (m *Model) addWord(word string) uint32 {
	if id, ok := m.DictMap[word]; ok {
		return id
	}
	id := uint32(len(m.Dictionary))
	m.Dictionary = append(m.Dictionary, word)
	m.DictMap[word] = id
	return id
}

// findWord returns the symbol ID for a word, or 0 if not found.
func (m *Model) findWord(word string) uint32 {
	if id, ok := m.DictMap[word]; ok {
		return id
	}
	return 0
}

// searchNode performs a binary search for symbol in node.Children.
// Returns the index where the symbol is or should be inserted, and whether it was found.
func searchNode(node *Node, symbol uint32) (int, bool) {
	lo, hi := 0, len(node.Children)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if node.Children[mid].Symbol == symbol {
			return mid, true
		} else if node.Children[mid].Symbol < symbol {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, false
}

// findSymbol returns the child node with the given symbol, or nil.
func findSymbol(node *Node, symbol uint32) *Node {
	if node == nil {
		return nil
	}
	i, found := searchNode(node, symbol)
	if found {
		return node.Children[i]
	}
	return nil
}

// findSymbolAdd returns the child node with the given symbol, creating it if absent.
func findSymbolAdd(node *Node, symbol uint32) *Node {
	i, found := searchNode(node, symbol)
	if found {
		return node.Children[i]
	}
	child := newNode()
	child.Symbol = symbol
	// Insert at position i to maintain sorted order
	node.Children = append(node.Children, nil)
	copy(node.Children[i+1:], node.Children[i:])
	node.Children[i] = child
	return child
}

// addSymbol finds or creates the child for the symbol, increments counts.
func addSymbol(node *Node, symbol uint32) *Node {
	child := findSymbolAdd(node, symbol)
	child.Count++
	node.Usage++
	return child
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestNewModel|TestAddAndFindWord|TestAddSymbol" -v ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add megahal.go megahal_test.go
git commit -m "feat: implement dictionary and Markov tree data structures"
```

---

### Task 4: Learning (learn + updateModel + initializeContext)

**Files:**
- Modify: `megahal.go`
- Modify: `megahal_test.go`

Implements the learning algorithm: tokenize input, walk forward tree adding symbols, walk backward tree adding symbols. Port of `learn()` at `megahal.c:1402-1451` and `update_model()` at `megahal.c:1163-1176`.

- [ ] **Step 1: Write learning tests**

Append to `megahal_test.go`:

```go
func TestLearn(t *testing.T) {
	m := newModel(5)

	m.learn("Hello world")

	// Dictionary should now contain: <FIN>, HELLO, " ", WORLD, "."
	if len(m.Dictionary) != 5 {
		t.Fatalf("Dictionary size = %d, want 5, got %v", len(m.Dictionary), m.Dictionary)
	}

	// Forward tree should have children (the first symbols seen)
	if len(m.Forward.Children) == 0 {
		t.Error("Forward tree has no children after learning")
	}

	// Backward tree should have children too
	if len(m.Backward.Children) == 0 {
		t.Error("Backward tree has no children after learning")
	}

	// Learning the same sentence again should increase counts, not add new dictionary entries
	m.learn("Hello world")
	if len(m.Dictionary) != 5 {
		t.Errorf("Dictionary size after re-learn = %d, want 5", len(m.Dictionary))
	}

	// Forward root usage should have increased
	if m.Forward.Usage < 2 {
		t.Errorf("Forward.Usage = %d, want >= 2", m.Forward.Usage)
	}
}

func TestLearnShortInput(t *testing.T) {
	m := newModel(5)

	// Input shorter than order should still be learned
	// (the C code skips if words->size <= order, but we tokenize "Hi" into ["HI", "."]
	// which is 2 tokens — less than order 5, so it would be skipped)
	m.learn("Hi")

	// With only 2 tokens, learning is skipped per original algorithm
	if len(m.Forward.Children) != 0 {
		t.Error("Short input should not be learned")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestLearn" -v ./...`
Expected: FAIL — `learn` method not defined

- [ ] **Step 3: Implement learning**

Add to `megahal.go`:

```go
// initializeContext resets all context slots to nil.
func (m *Model) initializeContext() {
	for i := range m.Context {
		m.Context[i] = nil
	}
}

// updateModel updates the context tree with a symbol (adding nodes).
func (m *Model) updateModel(symbol uint32) {
	for i := m.Order + 1; i > 0; i-- {
		if m.Context[i-1] != nil {
			m.Context[i] = addSymbol(m.Context[i-1], symbol)
		}
	}
}

// updateContext updates the context tree with a symbol (read-only traversal).
func (m *Model) updateContext(symbol uint32) {
	for i := m.Order + 1; i > 0; i-- {
		if m.Context[i-1] != nil {
			m.Context[i] = findSymbol(m.Context[i-1], symbol)
		}
	}
}

// learn tokenizes the input and trains both forward and backward trees.
func (m *Model) learn(input string) {
	tokens := makeWords(input)
	if len(tokens) <= m.Order {
		return
	}

	symbols := make([]uint32, len(tokens))
	for i, tok := range tokens {
		symbols[i] = m.addWord(tok)
	}

	// Train forward
	m.initializeContext()
	m.Context[0] = m.Forward
	for _, sym := range symbols {
		m.updateModel(sym)
	}
	m.updateModel(0) // sentence boundary

	// Train backward
	m.initializeContext()
	m.Context[0] = m.Backward
	for i := len(symbols) - 1; i >= 0; i-- {
		m.updateModel(symbols[i])
	}
	m.updateModel(0) // sentence boundary
}
```

Note on boundary symbol: Dictionary[0] = "" is the boundary/end-of-sentence symbol. `findWord` returns 0 for unknown words, which also serves as the boundary marker. The original C code uses both 0 and 1 as terminators in `reply()` — we simplify to just 0. `learn` calls `updateModel(0)` to mark sentence boundaries in both directions. `replyOnce` stops generating when it encounters symbol 0.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestLearn|TestNewModel" -v ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add megahal.go megahal_test.go
git commit -m "feat: implement learning algorithm (forward + backward tree training)"
```

---

### Task 5: Support File Loading (ban, aux, swp)

**Files:**
- Modify: `megahal.go`
- Modify: `megahal_test.go`

Load keyword filtering lists and word swap pairs from text files. These are used during reply generation to select interesting keywords.

- `megahal.ban`: one word per line, words to exclude from keywords
- `megahal.aux`: one word per line, auxiliary keywords (used only as fallback)
- `megahal.swp`: pairs of words per line (one pair per two lines: from, then to), used to swap perspective (e.g., MY → YOUR)

- [ ] **Step 1: Write tests for file loading**

Append to `megahal_test.go`:

```go
import "os"

func TestLoadWordList(t *testing.T) {
	// Create a temp file with some words
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
	if words["THE"] != true || words["A"] != true || words["IS"] != true {
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestLoadWordList|TestLoadSwapList" -v ./...`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement file loaders**

Add to `megahal.go`:

```go
import (
	"bufio"
	"os"
)

// loadWordList loads a file with one word per line into a set.
// Returns an empty map if the file doesn't exist.
func loadWordList(path string) map[string]bool {
	result := make(map[string]bool)
	f, err := os.Open(path)
	if err != nil {
		return result
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			result[strings.ToUpper(line)] = true
		}
	}
	return result
}

// loadSwapList loads a swap file (pairs of lines: from, to).
// Returns an empty map if the file doesn't exist.
func loadSwapList(path string) map[string]string {
	result := make(map[string]string)
	f, err := os.Open(path)
	if err != nil {
		return result
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		from := strings.TrimSpace(strings.ToUpper(scanner.Text()))
		if !scanner.Scan() {
			break
		}
		to := strings.TrimSpace(strings.ToUpper(scanner.Text()))
		if from != "" && to != "" {
			result[from] = to
		}
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestLoadWordList|TestLoadSwapList" -v ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add megahal.go megahal_test.go
git commit -m "feat: implement support file loading (ban, aux, swap lists)"
```

---

### Task 6: Reply Generation (seed, babble, reply, generateReply, evaluateReply)

**Files:**
- Modify: `megahal.go`
- Modify: `megahal_test.go`

This is the core reply algorithm. Port of `seed()`, `babble()`, `reply()`, `generate_reply()`, `evaluate_reply()`, `make_keywords()`, `add_key()`, `add_aux()` from `megahal.c:1826-2348`.

The new additions vs the original:
- Temperature parameter modifies probability distribution in `babble()`
- Surprise bias exponent applied in `evaluateReply()`
- `GenerationConfig` struct holds per-reply overrides

- [ ] **Step 1: Write reply generation tests**

Append to `megahal_test.go`:

```go
import "time"

func TestMakeKeywords(t *testing.T) {
	m := newModel(5)
	ban := map[string]bool{"THE": true, "A": true, "IS": true}
	aux := map[string]bool{"IT": true, "THIS": true}
	swaps := map[string]string{"MY": "YOUR"}

	// Add some words to dictionary so they're "known"
	m.addWord("HELLO")
	m.addWord("WORLD")
	m.addWord("THE")
	m.addWord("IT")
	m.addWord("MY")
	m.addWord("YOUR")

	tokens := makeWords("the hello world")
	keys := m.makeKeywords(tokens, ban, aux, swaps)

	// THE is banned, so only HELLO and WORLD should be keywords
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
	// MY should be swapped to YOUR in keywords
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

	// Train with enough data to generate replies
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestMakeKeywords|TestGenerateReply" -v ./...`
Expected: FAIL — functions/types not defined

- [ ] **Step 3: Implement reply generation**

Add to `megahal.go`:

```go
import (
	"math"
	"math/rand"
	"time"
)

// GenerationConfig holds per-reply generation parameters.
type GenerationConfig struct {
	Temperature  float64
	SurpriseBias float64
	ReplyTimeout time.Duration
}

// makeKeywords extracts interesting keywords from tokens.
// Applies swap mapping, excludes banned words, falls back to aux words.
func (m *Model) makeKeywords(tokens []string, ban, aux map[string]bool, swaps map[string]string) []string {
	var keys []string
	seen := make(map[string]bool)

	// First pass: add primary keywords (not banned, not aux)
	for _, tok := range tokens {
		word := tok
		if sw, ok := swaps[word]; ok {
			word = sw
		}
		if m.findWord(word) == 0 {
			continue
		}
		r := firstRune(word)
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		if ban[word] {
			continue
		}
		if aux[word] {
			continue
		}
		if !seen[word] {
			keys = append(keys, word)
			seen[word] = true
		}
	}

	// Second pass: if we have primary keywords, also add aux keywords
	if len(keys) > 0 {
		for _, tok := range tokens {
			word := tok
			if sw, ok := swaps[word]; ok {
				word = sw
			}
			if m.findWord(word) == 0 {
				continue
			}
			r := firstRune(word)
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				continue
			}
			if !aux[word] {
				continue
			}
			if !seen[word] {
				keys = append(keys, word)
				seen[word] = true
			}
		}
	}

	return keys
}

func firstRune(s string) rune {
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

// seed picks an initial symbol for reply generation.
// Tries to find a non-aux keyword in the model, falls back to random.
func (m *Model) seed(keys []string, aux map[string]bool) uint32 {
	if len(m.Forward.Children) == 0 {
		return 0
	}
	symbol := m.Forward.Children[rand.Intn(len(m.Forward.Children))].Symbol

	if len(keys) > 0 {
		start := rand.Intn(len(keys))
		i := start
		for {
			id := m.findWord(keys[i])
			if id != 0 && !aux[keys[i]] {
				return id
			}
			i = (i + 1) % len(keys)
			if i == start {
				break
			}
		}
	}

	return symbol
}

// babble selects the next symbol from the current context.
// Uses temperature to modify the probability distribution.
// Prefers keywords that haven't been used in the reply yet.
func (m *Model) babble(keys []string, replyWords []string, aux map[string]bool, usedKey bool, temperature float64) (uint32, bool) {
	// Find the deepest available context
	var node *Node
	for i := 0; i <= m.Order; i++ {
		if m.Context[i] != nil {
			node = m.Context[i]
		}
	}

	if node == nil || len(node.Children) == 0 {
		return 0, usedKey
	}

	// Build reply word set for deduplication
	replySet := make(map[string]bool, len(replyWords))
	for _, w := range replyWords {
		replySet[w] = true
	}

	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		keySet[k] = true
	}

	// Pick a random starting position and walk through children
	// weighted by count, looking for keywords
	i := rand.Intn(len(node.Children))
	count := rand.Intn(int(node.Usage))

	for count >= 0 {
		sym := node.Children[i].Symbol
		word := ""
		if int(sym) < len(m.Dictionary) {
			word = m.Dictionary[sym]
		}

		if keySet[word] && (usedKey || !aux[word]) && !replySet[word] {
			return sym, true
		}

		childCount := int(node.Children[i].Count)
		if temperature != 1.0 && temperature > 0 {
			// Apply temperature: modify effective count
			childCount = int(math.Pow(float64(childCount), 1.0/temperature))
			if childCount < 1 {
				childCount = 1
			}
		}
		count -= childCount
		i++
		if i >= len(node.Children) {
			i = 0
		}
	}

	// Return whatever symbol we landed on
	return node.Children[i].Symbol, usedKey
}

// replyOnce generates a single candidate reply.
func (m *Model) replyOnce(keys []string, aux map[string]bool, temperature float64) []string {
	var words []string

	// Forward direction
	m.initializeContext()
	m.Context[0] = m.Forward
	usedKey := false

	// Seed with a keyword
	symbol := m.seed(keys, aux)
	if symbol == 0 {
		return nil
	}

	words = append(words, m.Dictionary[symbol])
	m.updateContext(symbol)

	// Generate forward
	for {
		var sym uint32
		sym, usedKey = m.babble(keys, words, aux, usedKey, temperature)
		if sym == 0 {
			break
		}
		words = append(words, m.Dictionary[sym])
		m.updateContext(sym)
	}

	// Backward direction
	m.initializeContext()
	m.Context[0] = m.Backward

	// Re-establish context from current reply
	limit := len(words) - 1
	if limit > m.Order {
		limit = m.Order
	}
	for i := limit; i >= 0; i-- {
		sym := m.findWord(words[i])
		m.updateContext(sym)
	}

	// Generate backward
	for {
		sym, uk := m.babble(keys, words, aux, usedKey, temperature)
		usedKey = uk
		if sym == 0 {
			break
		}
		// Prepend
		words = append([]string{m.Dictionary[sym]}, words...)
		m.updateContext(sym)
	}

	return words
}

// dissimilar returns true if two word lists are different.
func dissimilar(a, b []string) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

// evaluateReply computes the surprise score of a reply given keywords.
func (m *Model) evaluateReply(keys []string, words []string, surpriseBias float64) float64 {
	if len(words) == 0 {
		return 0
	}

	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		keySet[k] = true
	}

	entropy := 0.0
	num := 0

	// Forward pass
	m.initializeContext()
	m.Context[0] = m.Forward
	for _, w := range words {
		sym := m.findWord(w)
		if keySet[w] {
			prob := 0.0
			count := 0
			num++
			for j := 0; j < m.Order; j++ {
				if m.Context[j] != nil {
					node := findSymbol(m.Context[j], sym)
					if node != nil {
						prob += float64(node.Count) / float64(m.Context[j].Usage)
						count++
					}
				}
			}
			if count > 0 {
				entropy -= math.Log(prob / float64(count))
			}
		}
		m.updateContext(sym)
	}

	// Backward pass
	m.initializeContext()
	m.Context[0] = m.Backward
	for i := len(words) - 1; i >= 0; i-- {
		sym := m.findWord(words[i])
		if keySet[words[i]] {
			prob := 0.0
			count := 0
			num++
			for j := 0; j < m.Order; j++ {
				if m.Context[j] != nil {
					node := findSymbol(m.Context[j], sym)
					if node != nil {
						prob += float64(node.Count) / float64(m.Context[j].Usage)
						count++
					}
				}
			}
			if count > 0 {
				entropy -= math.Log(prob / float64(count))
			}
		}
		m.updateContext(sym)
	}

	// Dampen for long replies (matches original)
	if num >= 8 {
		entropy /= math.Sqrt(float64(num - 1))
	}
	if num >= 16 {
		entropy /= float64(num)
	}

	// Apply surprise bias
	if surpriseBias != 1.0 {
		entropy = math.Pow(math.Abs(entropy), surpriseBias)
	}

	return entropy
}

// makeOutput joins reply words into a string with capitalization.
func makeOutput(words []string) string {
	if len(words) == 0 {
		return ""
	}
	result := strings.Join(words, "")
	// Capitalize first letter
	runes := []rune(result)
	capsNext := true
	for i, r := range runes {
		if capsNext && unicode.IsLetter(r) {
			runes[i] = unicode.ToUpper(r)
			capsNext = false
		} else {
			runes[i] = unicode.ToLower(r)
		}
		if r == '.' || r == '!' || r == '?' {
			capsNext = true
		}
	}
	return string(runes)
}

// generateReply generates the best reply for the given input.
func (m *Model) generateReply(input string, ban, aux map[string]bool, swaps map[string]string, cfg GenerationConfig) string {
	tokens := makeWords(input)
	keys := m.makeKeywords(tokens, ban, aux, swaps)

	// Generate a baseline reply with no keywords
	baseWords := m.replyOnce(nil, aux, cfg.Temperature)
	output := "I don't know enough to answer you yet!"
	if baseWords != nil && dissimilar(tokens, baseWords) {
		output = makeOutput(baseWords)
	}

	// Timed loop: generate and evaluate replies
	maxSurprise := -1.0
	deadline := time.Now().Add(cfg.ReplyTimeout)
	for time.Now().Before(deadline) {
		candidate := m.replyOnce(keys, aux, cfg.Temperature)
		if candidate == nil {
			continue
		}
		surprise := m.evaluateReply(keys, candidate, cfg.SurpriseBias)
		if surprise > maxSurprise && dissimilar(tokens, candidate) {
			maxSurprise = surprise
			output = makeOutput(candidate)
		}
	}

	return output
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestMakeKeywords|TestGenerateReply" -v ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add megahal.go megahal_test.go
git commit -m "feat: implement reply generation with surprise-maximizing selection and temperature"
```

---

### Task 7: Brain Persistence (save/load)

**Files:**
- Create: `brain.go`
- Create: `brain_test.go`

Implements save/load of the model to the custom `SVETSE2v1` binary format with atomic writes.

- [ ] **Step 1: Write brain persistence tests**

Create `brain_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadBrain(t *testing.T) {
	m := newModel(5)

	// Train with some data
	sentences := []string{
		"The cat sat on the mat and looked at the birds",
		"The dog ran through the park chasing the ball",
		"Birds fly over the mountains and rivers below",
		"Hello world this is a test of the brain save system",
	}
	for _, s := range sentences {
		m.learn(s)
	}

	// Save
	dir := t.TempDir()
	path := filepath.Join(dir, "test.brain")
	err := saveBrain(path, m)
	if err != nil {
		t.Fatalf("saveBrain: %v", err)
	}

	// Verify file exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("brain file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("brain file is empty")
	}

	// Load into a new model
	m2 := newModel(5)
	err = loadBrain(path, m2)
	if err != nil {
		t.Fatalf("loadBrain: %v", err)
	}

	// Verify dictionary matches
	if len(m2.Dictionary) != len(m.Dictionary) {
		t.Errorf("Dictionary size: got %d, want %d", len(m2.Dictionary), len(m.Dictionary))
	}
	for i, w := range m.Dictionary {
		if i < len(m2.Dictionary) && m2.Dictionary[i] != w {
			t.Errorf("Dictionary[%d]: got %q, want %q", i, m2.Dictionary[i], w)
		}
	}

	// Verify DictMap is rebuilt
	for word, id := range m.DictMap {
		if m2.DictMap[word] != id {
			t.Errorf("DictMap[%q]: got %d, want %d", word, m2.DictMap[word], id)
		}
	}

	// Verify order
	if m2.Order != m.Order {
		t.Errorf("Order: got %d, want %d", m2.Order, m.Order)
	}

	// Verify trees have content
	if len(m2.Forward.Children) == 0 {
		t.Error("loaded Forward tree has no children")
	}
	if len(m2.Backward.Children) == 0 {
		t.Error("loaded Backward tree has no children")
	}

	// Verify forward tree structure matches
	if m2.Forward.Usage != m.Forward.Usage {
		t.Errorf("Forward.Usage: got %d, want %d", m2.Forward.Usage, m.Forward.Usage)
	}
}

func TestSaveAtomicity(t *testing.T) {
	m := newModel(5)
	m.learn("The cat sat on the mat and purred loudly")

	dir := t.TempDir()
	path := filepath.Join(dir, "test.brain")

	// Save twice — second should atomically replace first
	err := saveBrain(path, m)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	info1, _ := os.Stat(path)

	m.learn("The dog barked at the mailman every single morning")
	err = saveBrain(path, m)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	info2, _ := os.Stat(path)

	if info2.Size() <= info1.Size() {
		t.Errorf("second save should be larger: %d <= %d", info2.Size(), info1.Size())
	}
}

func TestLoadBrainMissing(t *testing.T) {
	m := newModel(5)
	err := loadBrain("/nonexistent/path/brain.bin", m)
	if err == nil {
		t.Error("loadBrain should return error for missing file")
	}
}

func TestLoadBrainCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.brain")
	os.WriteFile(path, []byte("not a brain file"), 0644)

	m := newModel(5)
	err := loadBrain(path, m)
	if err == nil {
		t.Error("loadBrain should return error for corrupt file")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestSave|TestLoad" -v ./...`
Expected: FAIL — `saveBrain`/`loadBrain` not defined

- [ ] **Step 3: Implement brain save/load**

Create `brain.go`:

```go
package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const brainCookie = "SVETSE2v1"

// saveBrain writes the model to a file atomically (write temp, then rename).
func saveBrain(path string, m *Model) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".brain-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	success := false
	defer func() {
		tmp.Close()
		if !success {
			os.Remove(tmpPath)
		}
	}()

	w := tmp

	// Header
	if _, err := w.Write([]byte(brainCookie)); err != nil {
		return fmt.Errorf("write cookie: %w", err)
	}

	// Order
	if err := binary.Write(w, binary.LittleEndian, uint8(m.Order)); err != nil {
		return fmt.Errorf("write order: %w", err)
	}

	// Trees
	if err := saveTree(w, m.Forward); err != nil {
		return fmt.Errorf("write forward tree: %w", err)
	}
	if err := saveTree(w, m.Backward); err != nil {
		return fmt.Errorf("write backward tree: %w", err)
	}

	// Dictionary
	if err := saveDictionary(w, m.Dictionary); err != nil {
		return fmt.Errorf("write dictionary: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	tmp.Close()

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	success = true
	return nil
}

func saveTree(w io.Writer, node *Node) error {
	if err := binary.Write(w, binary.LittleEndian, node.Symbol); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, node.Usage); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, node.Count); err != nil {
		return err
	}
	numChildren := uint32(len(node.Children))
	if err := binary.Write(w, binary.LittleEndian, numChildren); err != nil {
		return err
	}
	for _, child := range node.Children {
		if err := saveTree(w, child); err != nil {
			return err
		}
	}
	return nil
}

func saveDictionary(w io.Writer, dict []string) error {
	size := uint32(len(dict))
	if err := binary.Write(w, binary.LittleEndian, size); err != nil {
		return err
	}
	for _, word := range dict {
		b := []byte(word)
		length := uint32(len(b))
		if err := binary.Write(w, binary.LittleEndian, length); err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}

// loadBrain reads a model from a brain file.
func loadBrain(path string, m *Model) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Verify cookie
	cookie := make([]byte, len(brainCookie))
	if _, err := io.ReadFull(f, cookie); err != nil {
		return fmt.Errorf("read cookie: %w", err)
	}
	if string(cookie) != brainCookie {
		return fmt.Errorf("invalid brain file: bad cookie %q", string(cookie))
	}

	// Order
	var order uint8
	if err := binary.Read(f, binary.LittleEndian, &order); err != nil {
		return fmt.Errorf("read order: %w", err)
	}
	m.Order = int(order)
	m.Context = make([]*Node, m.Order+2)

	// Trees
	m.Forward = newNode()
	if err := loadTree(f, m.Forward); err != nil {
		return fmt.Errorf("read forward tree: %w", err)
	}
	m.Backward = newNode()
	if err := loadTree(f, m.Backward); err != nil {
		return fmt.Errorf("read backward tree: %w", err)
	}

	// Dictionary
	dict, err := loadDictionary(f)
	if err != nil {
		return fmt.Errorf("read dictionary: %w", err)
	}
	m.Dictionary = dict
	m.DictMap = make(map[string]uint32, len(dict))
	for i, word := range dict {
		m.DictMap[word] = uint32(i)
	}

	return nil
}

func loadTree(r io.Reader, node *Node) error {
	if err := binary.Read(r, binary.LittleEndian, &node.Symbol); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &node.Usage); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &node.Count); err != nil {
		return err
	}
	var numChildren uint32
	if err := binary.Read(r, binary.LittleEndian, &numChildren); err != nil {
		return err
	}
	if numChildren == 0 {
		return nil
	}
	node.Children = make([]*Node, numChildren)
	for i := uint32(0); i < numChildren; i++ {
		node.Children[i] = newNode()
		if err := loadTree(r, node.Children[i]); err != nil {
			return err
		}
	}
	return nil
}

func loadDictionary(r io.Reader) ([]string, error) {
	var size uint32
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return nil, err
	}
	dict := make([]string, size)
	for i := uint32(0); i < size; i++ {
		var length uint32
		if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
			return nil, err
		}
		b := make([]byte, length)
		if _, err := io.ReadFull(r, b); err != nil {
			return nil, err
		}
		dict[i] = string(b)
	}
	return dict, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestSave|TestLoad" -v ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add brain.go brain_test.go
git commit -m "feat: implement brain persistence with atomic saves (SVETSE2v1 format)"
```

---

### Task 8: Configuration and Model Goroutine

**Files:**
- Modify: `main.go`
- Modify: `megahal.go`

Wire up environment variable parsing, the model goroutine, and the per-message override parser.

- [ ] **Step 1: Write config and override parser tests**

Append to `megahal_test.go`:

```go
func TestParseOverrides(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantText  string
		wantKV    map[string]string
		wantHelp  bool
	}{
		{
			name:     "no overrides",
			input:    "hello world",
			wantText: "hello world",
			wantKV:   map[string]string{},
		},
		{
			name:     "chaos override",
			input:    "hello !CHAOS=1.5 world",
			wantText: "hello world",
			wantKV:   map[string]string{"CHAOS": "1.5"},
		},
		{
			name:     "multiple overrides",
			input:    "test !TEMPERATURE=2.0 !TIMEOUT=5s message",
			wantText: "test message",
			wantKV:   map[string]string{"TEMPERATURE": "2.0", "TIMEOUT": "5s"},
		},
		{
			name:     "help flag",
			input:    "!HELP",
			wantText: "",
			wantHelp: true,
		},
		{
			name:     "help with other text",
			input:    "hello !HELP world",
			wantText: "hello world",
			wantHelp: true,
		},
		{
			name:     "case insensitive keys",
			input:    "test !chaos=2.0",
			wantText: "test",
			wantKV:   map[string]string{"CHAOS": "2.0"},
		},
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
			if tt.wantKV != nil {
				for k, v := range tt.wantKV {
					if kv[k] != v {
						t.Errorf("kv[%q] = %q, want %q", k, kv[k], v)
					}
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestParseOverrides -v ./...`
Expected: FAIL — `parseOverrides` not defined

- [ ] **Step 3: Implement config, override parser, and model goroutine**

Add to `megahal.go`:

```go
import (
	"regexp"
	"strconv"
)

var overrideRe = regexp.MustCompile(`(?i)!(CHAOS|TEMPERATURE|SURPRISE_BIAS|TIMEOUT|HELP)(?:=(\S+))?`)

// parseOverrides extracts !KEY=VALUE pairs from a message.
// Returns cleaned text, key-value map, and whether !HELP was found.
func parseOverrides(input string) (string, map[string]string, bool) {
	kv := make(map[string]string)
	help := false

	cleaned := overrideRe.ReplaceAllStringFunc(input, func(match string) string {
		sub := overrideRe.FindStringSubmatch(match)
		key := strings.ToUpper(sub[1])
		if key == "HELP" {
			help = true
		} else if sub[2] != "" {
			kv[key] = sub[2]
		}
		return ""
	})

	// Collapse multiple spaces left by removals
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return cleaned, kv, help
}

// applyOverrides merges !KEY=VALUE overrides into a GenerationConfig.
func applyOverrides(base GenerationConfig, kv map[string]string) GenerationConfig {
	cfg := base

	// Apply CHAOS first (sets both temperature and surprise bias)
	if v, ok := kv["CHAOS"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			cfg.Temperature = f
			cfg.SurpriseBias = f
		}
	}

	// Individual knobs override CHAOS
	if v, ok := kv["TEMPERATURE"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			cfg.Temperature = f
		}
	}
	if v, ok := kv["SURPRISE_BIAS"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			cfg.SurpriseBias = f
		}
	}
	if v, ok := kv["TIMEOUT"]; ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			if d > 30*time.Second {
				d = 30 * time.Second
			}
			cfg.ReplyTimeout = d
		}
	}

	return cfg
}
```

Add the model goroutine and config to `main.go`:

```go
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Config holds all environment-based configuration.
type Config struct {
	SlackToken    string
	SlackAppToken string
	DiscordToken  string

	SlackChannels   []string
	DiscordChannels []string

	BrainPath    string
	SaveInterval time.Duration

	BanFile string
	AuxFile string
	SwpFile string

	DefaultConfig GenerationConfig
}

func loadConfig() Config {
	cfg := Config{
		SlackToken:    os.Getenv("SVETSE2_SLACK_TOKEN"),
		SlackAppToken: os.Getenv("SVETSE2_SLACK_APP_TOKEN"),
		DiscordToken:  os.Getenv("SVETSE2_DISCORD_TOKEN"),
		BrainPath:     envOrDefault("SVETSE2_BRAIN_PATH", "./brain.bin"),
		BanFile:       envOrDefault("SVETSE2_BAN_FILE", "./megahal.ban"),
		AuxFile:       envOrDefault("SVETSE2_AUX_FILE", "./megahal.aux"),
		SwpFile:       envOrDefault("SVETSE2_SWP_FILE", "./megahal.swp"),
	}

	if ch := os.Getenv("SVETSE2_SLACK_CHANNELS"); ch != "" {
		cfg.SlackChannels = strings.Split(ch, ",")
		for i := range cfg.SlackChannels {
			cfg.SlackChannels[i] = strings.TrimSpace(cfg.SlackChannels[i])
		}
	}
	if ch := os.Getenv("SVETSE2_DISCORD_CHANNELS"); ch != "" {
		cfg.DiscordChannels = strings.Split(ch, ",")
		for i := range cfg.DiscordChannels {
			cfg.DiscordChannels[i] = strings.TrimSpace(cfg.DiscordChannels[i])
		}
	}

	cfg.SaveInterval = parseDuration("SVETSE2_SAVE_INTERVAL", 5*time.Minute)
	chaos := parseFloat("SVETSE2_CHAOS", 1.0)
	cfg.DefaultConfig = GenerationConfig{
		Temperature:  parseFloat("SVETSE2_TEMPERATURE", chaos),
		SurpriseBias: parseFloat("SVETSE2_SURPRISE_BIAS", chaos),
		ReplyTimeout: parseDuration("SVETSE2_REPLY_TIMEOUT", 2*time.Second),
	}

	return cfg
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var f float64
	if _, err := fmt.Sscanf(v, "%f", &f); err != nil {
		return def
	}
	return f
}

func parseDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// LearnRequest asks the model to learn from text.
type LearnRequest struct {
	Text string
}

// ReplyRequest asks the model to generate a reply.
type ReplyRequest struct {
	Text      string
	Overrides map[string]string
	ReplyCh   chan string
}

// HelpRequest asks for the help text.
type HelpRequest struct {
	ReplyCh chan string
}

func helpText(cfg GenerationConfig) string {
	return fmt.Sprintf(`SVETSE2 — MegaHAL Markov chain bot

Usage: @bot <message> [!KEY=VALUE...]

Per-message overrides:
  !CHAOS=X          Combined chaos dial (default: %.1f)
  !TEMPERATURE=X    Random walk temperature (default: %.1f)
  !SURPRISE_BIAS=X  Surprise scoring exponent (default: %.1f)
  !TIMEOUT=Xs       Reply generation time (default: %s, max: 30s)
  !HELP             Show this message

Higher CHAOS = wilder, more unhinged replies.`,
		cfg.Temperature, cfg.Temperature, cfg.SurpriseBias, cfg.ReplyTimeout)
}

func runModelGoroutine(cfg Config, learnCh <-chan LearnRequest, replyCh <-chan ReplyRequest, helpCh <-chan HelpRequest, quit <-chan struct{}) {
	model := newModel(5)
	ban := loadWordList(cfg.BanFile)
	aux := loadWordList(cfg.AuxFile)
	swaps := loadSwapList(cfg.SwpFile)

	// Try to load existing brain
	if err := loadBrain(cfg.BrainPath, model); err != nil {
		log.Printf("No existing brain loaded: %v", err)
	} else {
		log.Printf("Brain loaded: %d words in dictionary", len(model.Dictionary))
	}

	saveTicker := time.NewTicker(cfg.SaveInterval)
	defer saveTicker.Stop()

	save := func() {
		if err := saveBrain(cfg.BrainPath, model); err != nil {
			log.Printf("Error saving brain: %v", err)
		} else {
			log.Printf("Brain saved: %d words in dictionary", len(model.Dictionary))
		}
	}

	for {
		select {
		case req := <-learnCh:
			model.learn(req.Text)
		case req := <-replyCh:
			genCfg := applyOverrides(cfg.DefaultConfig, req.Overrides)
			reply := model.generateReply(req.Text, ban, aux, swaps, genCfg)
			req.ReplyCh <- reply
		case req := <-helpCh:
			req.ReplyCh <- helpText(cfg.DefaultConfig)
		case <-saveTicker.C:
			save()
		case <-quit:
			save()
			return
		}
	}
}

func main() {
	cfg := loadConfig()

	if cfg.SlackToken == "" && cfg.DiscordToken == "" {
		log.Fatal("At least one of SVETSE2_SLACK_TOKEN or SVETSE2_DISCORD_TOKEN must be set")
	}

	learnCh := make(chan LearnRequest, 100)
	replyCh := make(chan ReplyRequest, 10)
	helpCh := make(chan HelpRequest, 10)
	quit := make(chan struct{})

	go runModelGoroutine(cfg, learnCh, replyCh, helpCh, quit)

	if cfg.SlackToken != "" {
		go runSlack(cfg, learnCh, replyCh, helpCh)
		log.Println("Slack adapter started")
	}

	if cfg.DiscordToken != "" {
		go runDiscord(cfg, learnCh, replyCh, helpCh)
		log.Println("Discord adapter started")
	}

	log.Println("SVETSE2 running. Press Ctrl+C to stop.")

	// Wait for signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down...")
	close(quit)
	// Give the model goroutine time to save
	time.Sleep(2 * time.Second)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestParseOverrides -v ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add main.go megahal.go megahal_test.go
git commit -m "feat: implement config, model goroutine, and per-message override parser"
```

---

### Task 9: Slack Adapter

**Files:**
- Create: `platform_slack.go`

Implements the Slack Socket Mode adapter. Listens to all messages, learns from non-mention messages, generates replies for mentions, sends top-level channel messages (not threads).

- [ ] **Step 1: Add slack dependency**

Run:
```bash
go get github.com/slack-go/slack
```

- [ ] **Step 2: Implement Slack adapter**

Create `platform_slack.go`:

```go
package main

import (
	"log"
	"regexp"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
	"github.com/slack-go/slack/slackevents"
)

var slackMentionRe = regexp.MustCompile(`<@[A-Z0-9]+>`)
var slackFormatRe = regexp.MustCompile(`<[^>]+>`)

// cleanSlackText strips Slack formatting tokens from message text.
// Removes user mentions (<@USERID>), channel refs (<#CID|name>), URLs (<http://...|label>).
func cleanSlackText(text string) string {
	// Replace channel refs with their display name
	text = regexp.MustCompile(`<#[A-Z0-9]+\|([^>]+)>`).ReplaceAllString(text, "$1")
	// Replace URL wrappers with the URL or label
	text = regexp.MustCompile(`<(https?://[^|>]+)\|([^>]+)>`).ReplaceAllString(text, "$2")
	text = regexp.MustCompile(`<(https?://[^>]+)>`).ReplaceAllString(text, "$1")
	// Remove user mentions
	text = slackMentionRe.ReplaceAllString(text, "")
	// Collapse whitespace
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}

func runSlack(cfg Config, learnCh chan<- LearnRequest, replyCh chan<- ReplyRequest, helpCh chan<- HelpRequest) {
	api := slack.New(
		cfg.SlackToken,
		slack.OptionAppLevelToken(cfg.SlackAppToken),
	)

	client := socketmode.New(api)

	// Get our own user ID to detect mentions
	authResp, err := api.AuthTest()
	if err != nil {
		log.Fatalf("Slack auth failed: %v", err)
	}
	botUserID := authResp.UserID
	mentionTag := "<@" + botUserID + ">"
	log.Printf("Slack bot user ID: %s", botUserID)

	// Build channel allowlist set
	allowedChannels := make(map[string]bool)
	for _, ch := range cfg.SlackChannels {
		allowedChannels[strings.TrimPrefix(ch, "#")] = true
	}

	go func() {
		for evt := range client.Events {
			switch evt.Type {
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				client.Ack(*evt.Request)

				switch ev := eventsAPIEvent.InnerEvent.Data.(type) {
				case *slackevents.MessageEvent:
					// Ignore bot messages and message edits/deletes
					if ev.BotID != "" || ev.SubType != "" {
						continue
					}

					isMention := strings.Contains(ev.Text, mentionTag)

					if !isMention {
						// Learn from non-mention messages
						cleaned := cleanSlackText(ev.Text)
						if cleaned != "" {
							learnCh <- LearnRequest{Text: cleaned}
						}
						continue
					}

					// Check channel allowlist
					if len(allowedChannels) > 0 && !allowedChannels[ev.Channel] {
						// Resolve channel name for name-based allowlists
						info, err := api.GetConversationInfo(&slack.GetConversationInfoInput{
							ChannelID: ev.Channel,
						})
						if err != nil || !allowedChannels[info.Name] {
							continue
						}
					}

					// Parse overrides and strip mention
					text := cleanSlackText(ev.Text)
					text, overrides, isHelp := parseOverrides(text)

					if isHelp {
						rc := make(chan string, 1)
						helpCh <- HelpRequest{ReplyCh: rc}
						reply := <-rc
						api.PostMessage(ev.Channel, slack.MsgOptionText(reply, false))
						continue
					}

					if strings.TrimSpace(text) == "" {
						continue
					}

					// Generate reply
					rc := make(chan string, 1)
					replyCh <- ReplyRequest{
						Text:      text,
						Overrides: overrides,
						ReplyCh:   rc,
					}
					reply := <-rc
					api.PostMessage(ev.Channel, slack.MsgOptionText(reply, false))
				}

			case socketmode.EventTypeConnecting:
				log.Println("Slack: connecting...")
			case socketmode.EventTypeConnected:
				log.Println("Slack: connected")
			case socketmode.EventTypeConnectionError:
				log.Println("Slack: connection error")
			}
		}
	}()

	if err := client.Run(); err != nil {
		log.Fatalf("Slack client error: %v", err)
	}
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: compiles without errors

- [ ] **Step 4: Commit**

```bash
git add platform_slack.go go.mod go.sum
git commit -m "feat: implement Slack Socket Mode adapter"
```

---

### Task 10: Discord Adapter

**Files:**
- Create: `platform_discord.go`

Implements the Discord gateway adapter. Same logic as Slack: learn from non-mention messages, reply to mentions as top-level channel messages.

- [ ] **Step 1: Add discordgo dependency**

Run:
```bash
go get github.com/bwmarrin/discordgo
```

- [ ] **Step 2: Implement Discord adapter**

Create `platform_discord.go`:

```go
package main

import (
	"log"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var discordMentionRe = regexp.MustCompile(`<@!?\d+>`)

// cleanDiscordText strips Discord formatting from message text.
func cleanDiscordText(text string) string {
	text = discordMentionRe.ReplaceAllString(text, "")
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}

func runDiscord(cfg Config, learnCh chan<- LearnRequest, replyCh chan<- ReplyRequest, helpCh chan<- HelpRequest) {
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		log.Fatalf("Discord session error: %v", err)
	}

	// We need message content intent
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	// Build channel allowlist
	allowedChannels := make(map[string]bool)
	for _, ch := range cfg.DiscordChannels {
		allowedChannels[strings.TrimPrefix(ch, "#")] = true
	}

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore our own messages
		if m.Author.ID == s.State.User.ID {
			return
		}

		// Ignore bot messages
		if m.Author.Bot {
			return
		}

		// Check if we're mentioned
		isMention := false
		for _, mention := range m.Mentions {
			if mention.ID == s.State.User.ID {
				isMention = true
				break
			}
		}

		if !isMention {
			// Learn from non-mention messages
			cleaned := cleanDiscordText(m.Content)
			if cleaned != "" {
				learnCh <- LearnRequest{Text: cleaned}
			}
			return
		}

		// Check channel allowlist
		if len(allowedChannels) > 0 {
			ch, err := s.Channel(m.ChannelID)
			if err != nil || (!allowedChannels[m.ChannelID] && !allowedChannels[ch.Name]) {
				return
			}
		}

		// Parse overrides and strip mention
		text := cleanDiscordText(m.Content)
		text, overrides, isHelp := parseOverrides(text)

		if isHelp {
			rc := make(chan string, 1)
			helpCh <- HelpRequest{ReplyCh: rc}
			reply := <-rc
			s.ChannelMessageSend(m.ChannelID, reply)
			return
		}

		if strings.TrimSpace(text) == "" {
			return
		}

		// Generate reply
		rc := make(chan string, 1)
		replyCh <- ReplyRequest{
			Text:      text,
			Overrides: overrides,
			ReplyCh:   rc,
		}
		reply := <-rc
		s.ChannelMessageSend(m.ChannelID, reply)
	})

	if err := dg.Open(); err != nil {
		log.Fatalf("Discord connection error: %v", err)
	}
	log.Println("Discord adapter connected")

	// Block forever (or until the session closes)
	select {}
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: compiles without errors

- [ ] **Step 4: Commit**

```bash
git add platform_discord.go go.mod go.sum
git commit -m "feat: implement Discord gateway adapter"
```

---

### Task 11: Default Ban List and Dockerfile

**Files:**
- Create: `megahal.ban`
- Create: `Dockerfile`

- [ ] **Step 1: Create default ban list**

Create `megahal.ban` with common English stop words:

```
A
ABOUT
ABOVE
AFTER
AGAIN
ALL
AM
AN
AND
ANY
ARE
AS
AT
BE
BECAUSE
BEEN
BEFORE
BEING
BELOW
BETWEEN
BOTH
BUT
BY
CAN
COULD
DID
DO
DOES
DOING
DOWN
DURING
EACH
FEW
FOR
FROM
GET
GOT
HAD
HAS
HAVE
HAVING
HE
HER
HERE
HERS
HERSELF
HIM
HIMSELF
HIS
HOW
I
IF
IN
INTO
IS
IT
ITS
ITSELF
JUST
ME
MIGHT
MORE
MOST
MUST
MY
MYSELF
NO
NOR
NOT
NOW
OF
OFF
ON
ONCE
ONLY
OR
OTHER
OUR
OURS
OURSELVES
OUT
OVER
OWN
SAME
SHE
SHOULD
SO
SOME
SUCH
THAN
THAT
THE
THEIR
THEIRS
THEM
THEMSELVES
THEN
THERE
THESE
THEY
THIS
THOSE
THROUGH
TO
TOO
UNDER
UNTIL
UP
VERY
WAS
WE
WERE
WHAT
WHEN
WHERE
WHICH
WHILE
WHO
WHOM
WHY
WILL
WITH
WOULD
YOU
YOUR
YOURS
YOURSELF
YOURSELVES
```

- [ ] **Step 2: Create Dockerfile**

Create `Dockerfile`:

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o svetse2 .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/svetse2 .
COPY megahal.ban ./
ENTRYPOINT ["./svetse2"]
```

- [ ] **Step 3: Verify Docker build**

Run: `docker build -t svetse2 .`
Expected: builds successfully

- [ ] **Step 4: Commit**

```bash
git add megahal.ban Dockerfile
git commit -m "feat: add default ban list and Dockerfile"
```

---

### Task 12: Integration Test — Full Round Trip

**Files:**
- Modify: `megahal_test.go`

End-to-end test: learn sentences, save brain, load brain into fresh model, generate replies.

- [ ] **Step 1: Write integration test**

Append to `megahal_test.go`:

```go
func TestFullRoundTrip(t *testing.T) {
	m := newModel(5)

	// Train with enough data
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

	// Save brain
	dir := t.TempDir()
	path := dir + "/test.brain"
	if err := saveBrain(path, m); err != nil {
		t.Fatalf("saveBrain: %v", err)
	}

	// Load into fresh model
	m2 := newModel(5)
	if err := loadBrain(path, m2); err != nil {
		t.Fatalf("loadBrain: %v", err)
	}

	// Generate replies from loaded model
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

	// Test with high chaos
	cfg.Temperature = 3.0
	cfg.SurpriseBias = 2.0
	chaosReply := m2.generateReply("cat park", ban, aux, swaps, cfg)
	if chaosReply == "" {
		t.Error("no reply generated with high chaos")
	}
	t.Logf("High chaos reply: %s", chaosReply)
}

func TestOverrideApplication(t *testing.T) {
	base := GenerationConfig{
		Temperature:  1.0,
		SurpriseBias: 1.0,
		ReplyTimeout: 2 * time.Second,
	}

	// CHAOS sets both
	cfg := applyOverrides(base, map[string]string{"CHAOS": "2.5"})
	if cfg.Temperature != 2.5 {
		t.Errorf("Temperature = %f, want 2.5", cfg.Temperature)
	}
	if cfg.SurpriseBias != 2.5 {
		t.Errorf("SurpriseBias = %f, want 2.5", cfg.SurpriseBias)
	}

	// Individual knobs override CHAOS
	cfg = applyOverrides(base, map[string]string{
		"CHAOS":       "2.0",
		"TEMPERATURE": "3.0",
	})
	if cfg.Temperature != 3.0 {
		t.Errorf("Temperature = %f, want 3.0 (individual override)", cfg.Temperature)
	}
	if cfg.SurpriseBias != 2.0 {
		t.Errorf("SurpriseBias = %f, want 2.0 (from CHAOS)", cfg.SurpriseBias)
	}

	// Timeout capped at 30s
	cfg = applyOverrides(base, map[string]string{"TIMEOUT": "60s"})
	if cfg.ReplyTimeout != 30*time.Second {
		t.Errorf("ReplyTimeout = %v, want 30s (capped)", cfg.ReplyTimeout)
	}
}
```

- [ ] **Step 2: Run all tests**

Run: `go test -v ./...`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add megahal_test.go
git commit -m "test: add integration tests for full round trip and override application"
```

---

### Task 13: Final Cleanup and README

**Files:**
- Create: `README.md`
- Modify: `main.go` (if any compilation issues found)

- [ ] **Step 1: Run full build and test suite**

Run: `go build -o svetse2 . && go test -v -count=1 ./...`
Expected: builds and all tests pass

- [ ] **Step 2: Run go vet and check for issues**

Run: `go vet ./...`
Expected: no issues

- [ ] **Step 3: Create README**

Create `README.md`:

```markdown
# SVETSE2

A MegaHAL Markov-chain chatbot reimplemented in Go, with Slack and Discord support.

Learns from everything said in channels it can see, responds with surprise-maximizing
nonsense when @mentioned.

## Quick Start

```bash
# Build
go build -o svetse2 .

# Run with Slack
export SVETSE2_SLACK_TOKEN=xoxb-...
export SVETSE2_SLACK_APP_TOKEN=xapp-...
./svetse2

# Run with Discord
export SVETSE2_DISCORD_TOKEN=...
./svetse2

# Docker
docker build -t svetse2 .
docker run -e SVETSE2_SLACK_TOKEN=... -e SVETSE2_SLACK_APP_TOKEN=... svetse2
```

## Configuration

All via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `SVETSE2_SLACK_TOKEN` | Slack bot token | - |
| `SVETSE2_SLACK_APP_TOKEN` | Slack app-level token (Socket Mode) | - |
| `SVETSE2_DISCORD_TOKEN` | Discord bot token | - |
| `SVETSE2_SLACK_CHANNELS` | Comma-separated channel allowlist | (all) |
| `SVETSE2_DISCORD_CHANNELS` | Comma-separated channel allowlist | (all) |
| `SVETSE2_BRAIN_PATH` | Brain file path | `./brain.bin` |
| `SVETSE2_SAVE_INTERVAL` | Auto-save interval | `5m` |
| `SVETSE2_CHAOS` | Combined chaos dial | `1.0` |
| `SVETSE2_TEMPERATURE` | Random walk temperature | `1.0` |
| `SVETSE2_SURPRISE_BIAS` | Surprise scoring exponent | `1.0` |
| `SVETSE2_REPLY_TIMEOUT` | Reply generation duration | `2s` |
| `SVETSE2_BAN_FILE` | Banned keywords file | `./megahal.ban` |
| `SVETSE2_AUX_FILE` | Auxiliary keywords file | `./megahal.aux` |
| `SVETSE2_SWP_FILE` | Word swap pairs file | `./megahal.swp` |

## Per-Message Overrides

When @mentioning the bot, add `!KEY=VALUE` to override settings for that reply:

```
@bot tell me about cats !CHAOS=2.0 !TIMEOUT=5s
```

| Override | Description |
|----------|-------------|
| `!CHAOS=X` | Combined chaos dial |
| `!TEMPERATURE=X` | Random walk temperature |
| `!SURPRISE_BIAS=X` | Surprise scoring exponent |
| `!TIMEOUT=Xs` | Reply generation time (max 30s) |
| `!HELP` | Show usage guide |
```

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add README with configuration and usage guide"
```
