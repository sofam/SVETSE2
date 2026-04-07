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
			// Determine the token class from the first rune.
			startClass := wordClass(runes[i])
			i++ // consume the first rune
			for i < len(runes) && isWordRuneContinue(runes, i, startClass) {
				i++
			}
		} else {
			for i < len(runes) && !isWordRune(runes[i]) {
				i++
			}
		}
		tokens = append(tokens, string(runes[start:i]))
	}

	if len(tokens) == 0 {
		return nil
	}

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

// isWordRuneContinue returns true if runes[i] can continue the current word token.
// tokenClass is 1 for letter-run, 2 for digit-run.
// A word token is either a letter-run or a digit-run — they don't mix.
// Apostrophes are allowed within letter-runs (e.g. "don't").
func isWordRuneContinue(runes []rune, i int, tokenClass int) bool {
	r := runes[i]
	switch {
	case unicode.IsLetter(r):
		return tokenClass == 1 // letter continues letter-run only
	case unicode.IsDigit(r):
		return tokenClass == 2 // digit continues digit-run only
	case r == '\'' && i < len(runes)-1:
		// Apostrophe allowed within a letter-run only, and only if next rune is a letter
		return tokenClass == 1 && unicode.IsLetter(runes[i+1])
	}
	return false
}

// wordClass returns 1 for letter, 2 for digit, 0 for other (apostrophe etc.).
func wordClass(r rune) int {
	if unicode.IsLetter(r) {
		return 1
	}
	if unicode.IsDigit(r) {
		return 2
	}
	return 0
}

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
	// Context has order+2 slots
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
