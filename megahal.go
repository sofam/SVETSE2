package main

import (
	"bufio"
	"math"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// GenerationConfig controls reply generation behavior.
type GenerationConfig struct {
	Temperature  float64
	SurpriseBias float64
	ReplyTimeout time.Duration
}

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
// Sentences with fewer than 3 tokens (word + separator + punctuation) are skipped.
func (m *Model) learn(input string) {
	tokens := makeWords(input)
	if len(tokens) < 3 {
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

// loadWordList loads a file with one word per line into a set.
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

// makeKeywords extracts interesting keywords from tokens for use in reply generation.
// Applies swaps, skips unknown/banned/aux/non-alphanumeric words.
// If primary keywords were found, aux words that passed all other checks are appended.
func (m *Model) makeKeywords(tokens []string, ban, aux map[string]bool, swaps map[string]string) []string {
	seen := make(map[string]bool)
	var primary []string
	var secondary []string

	for _, tok := range tokens {
		// Apply swap if present.
		word := strings.ToUpper(tok)
		if sw, ok := swaps[word]; ok {
			word = sw
		}

		// Skip if not in model dictionary (symbol 0 means unknown).
		if m.findWord(word) == 0 {
			continue
		}

		// First rune must be letter or digit.
		r, _ := utf8.DecodeRuneInString(word)
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}

		// Skip if banned.
		if ban[word] {
			continue
		}

		if seen[word] {
			continue
		}
		seen[word] = true

		if aux[word] {
			secondary = append(secondary, word)
		} else {
			primary = append(primary, word)
		}
	}

	// Only include aux words when there are primary keywords.
	if len(primary) > 0 {
		return append(primary, secondary...)
	}
	return primary
}

// seed picks an initial symbol for reply generation.
// Tries keywords in order; falls back to a random child of Forward root.
func (m *Model) seed(keys []string, aux map[string]bool) uint32 {
	// Try to find a non-aux keyword present in the model.
	for _, k := range keys {
		if aux[k] {
			continue
		}
		sym := m.findWord(k)
		if sym != 0 {
			return sym
		}
	}

	// Fall back to a random child of the Forward root.
	if len(m.Forward.Children) == 0 {
		return 0
	}
	return m.Forward.Children[rand.Intn(len(m.Forward.Children))].Symbol
}

// babble selects the next symbol from the deepest available context node.
// It uses weighted random selection with temperature, preferring keywords not yet in reply.
func (m *Model) babble(keys []string, replyWords []string, aux map[string]bool, usedKey bool, temperature float64) (uint32, bool) {
	// Find deepest non-nil context node that has children.
	var node *Node
	for i := m.Order + 1; i >= 0; i-- {
		if m.Context[i] != nil && len(m.Context[i].Children) > 0 {
			node = m.Context[i]
			break
		}
	}
	if node == nil || node.Usage == 0 {
		return 0, usedKey
	}

	// Build a set of words already in the reply for quick lookup.
	inReply := make(map[string]bool, len(replyWords))
	for _, w := range replyWords {
		inReply[w] = true
	}

	// Build a set of keywords for quick lookup.
	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		keySet[k] = true
	}

	children := node.Children
	count := len(children)

	// Pick a random starting index.
	start := rand.Intn(count)

	// Compute total effective usage using temperature-adjusted counts.
	effectiveTotal := 0.0
	for _, child := range children {
		effectiveTotal += math.Pow(float64(child.Count), 1.0/temperature)
	}

	// Pick a random threshold in [0, effectiveTotal).
	threshold := rand.Float64() * effectiveTotal

	// Walk circularly. If we encounter a keyword not yet in the reply, prefer it.
	for i := 0; i < count; i++ {
		child := children[(start+i)%count]
		word := ""
		if int(child.Symbol) < len(m.Dictionary) {
			word = m.Dictionary[child.Symbol]
		}

		// If this is a keyword not yet used and not already in reply, use it.
		if !usedKey && keySet[word] && !inReply[word] && !aux[word] {
			return child.Symbol, true
		}

		eff := math.Pow(float64(child.Count), 1.0/temperature)
		threshold -= eff
		if threshold < 0 {
			return child.Symbol, usedKey
		}
	}

	// Fallback: return last child.
	return children[(start+count-1)%count].Symbol, usedKey
}

// replyOnce generates a single candidate reply word list.
func (m *Model) replyOnce(keys []string, aux map[string]bool, temperature float64) []string {
	var reply []string

	// Seed with a keyword.
	seedSym := m.seed(keys, aux)
	if seedSym == 0 {
		return nil
	}

	// Forward pass: initialize context and walk forward.
	m.initializeContext()
	m.Context[0] = m.Forward
	m.updateContext(seedSym)

	word := m.Dictionary[seedSym]
	reply = append(reply, word)

	var usedKey bool
	for i := 0; i < 1024; i++ {
		sym, uk := m.babble(keys, reply, aux, usedKey, temperature)
		usedKey = uk
		if sym == 0 {
			break
		}
		if int(sym) >= len(m.Dictionary) {
			break
		}
		reply = append(reply, m.Dictionary[sym])
		m.updateContext(sym)
	}

	// Backward pass: re-init backward context walking up to order words from reply start.
	m.initializeContext()
	m.Context[0] = m.Backward

	seedIdx := 0
	end := seedIdx + m.Order
	if end > len(reply) {
		end = len(reply)
	}
	for _, w := range reply[seedIdx:end] {
		sym := m.findWord(w)
		if sym == 0 {
			break
		}
		m.updateContext(sym)
	}

	usedKey = false
	for i := 0; i < 1024; i++ {
		sym, uk := m.babble(keys, reply, aux, usedKey, temperature)
		usedKey = uk
		if sym == 0 {
			break
		}
		if int(sym) >= len(m.Dictionary) {
			break
		}
		// Prepend the new word.
		reply = append([]string{m.Dictionary[sym]}, reply...)
		m.updateContext(sym)
	}

	return reply
}

// evaluateReply computes a surprise score for the given word list relative to keywords.
func (m *Model) evaluateReply(keys []string, words []string, surpriseBias float64) float64 {
	if len(keys) == 0 || len(words) == 0 {
		return 0
	}

	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		keySet[k] = true
	}

	num := len(words)
	entropy := 0.0

	// Forward pass.
	m.initializeContext()
	m.Context[0] = m.Forward
	for _, w := range words {
		sym := m.findWord(w)
		if sym != 0 {
			m.updateContext(sym)
		}
		if !keySet[w] {
			continue
		}
		// Compute probability across context levels.
		prob := 0.0
		count := 0
		for i := 1; i <= m.Order+1; i++ {
			ctx := m.Context[i]
			if ctx == nil {
				continue
			}
			child := findSymbol(ctx, sym)
			if child == nil || ctx.Usage == 0 {
				continue
			}
			prob += float64(child.Count) / float64(ctx.Usage)
			count++
		}
		if count > 0 && prob > 0 {
			entropy -= math.Log(prob / float64(count))
		}
	}

	// Backward pass.
	m.initializeContext()
	m.Context[0] = m.Backward
	for i := len(words) - 1; i >= 0; i-- {
		w := words[i]
		sym := m.findWord(w)
		if sym != 0 {
			m.updateContext(sym)
		}
		if !keySet[w] {
			continue
		}
		prob := 0.0
		count := 0
		for j := 1; j <= m.Order+1; j++ {
			ctx := m.Context[j]
			if ctx == nil {
				continue
			}
			child := findSymbol(ctx, sym)
			if child == nil || ctx.Usage == 0 {
				continue
			}
			prob += float64(child.Count) / float64(ctx.Usage)
			count++
		}
		if count > 0 && prob > 0 {
			entropy -= math.Log(prob / float64(count))
		}
	}

	// Dampen for long replies.
	if num >= 8 {
		entropy /= math.Sqrt(float64(num - 1))
	}
	if num >= 16 {
		entropy /= float64(num)
	}

	return math.Pow(math.Abs(entropy), surpriseBias)
}

// dissimilar returns true if word lists a and b differ.
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

// makeOutput joins words and capitalizes the first letter after sentence-ending punctuation.
func makeOutput(words []string) string {
	if len(words) == 0 {
		return ""
	}
	result := strings.Join(words, "")

	// Capitalize: find first letter and capitalize it, then capitalize after .!?
	runes := []rune(result)
	capitalize := true
	for i, r := range runes {
		if capitalize && unicode.IsLetter(r) {
			runes[i] = unicode.ToUpper(r)
			capitalize = false
		}
		if r == '.' || r == '!' || r == '?' {
			capitalize = true
		}
	}
	return string(runes)
}

// generateReply is the main entry point for reply generation.
// It extracts keywords, runs a timed loop generating candidates, and returns the highest-scoring one.
func (m *Model) generateReply(input string, ban, aux map[string]bool, swaps map[string]string, cfg GenerationConfig) string {
	const fallback = "I don't know enough to answer you yet!"

	tokens := makeWords(input)
	keys := m.makeKeywords(tokens, ban, aux, swaps)

	// Fail fast if the model is essentially empty.
	if len(m.Forward.Children) == 0 {
		return fallback
	}

	deadline := time.Now().Add(cfg.ReplyTimeout)

	var bestWords []string
	bestScore := -1.0
	var lastWords []string

	for time.Now().Before(deadline) {
		candidate := m.replyOnce(keys, aux, cfg.Temperature)
		if len(candidate) == 0 {
			continue
		}
		// Skip if identical to the last generated candidate.
		if lastWords != nil && !dissimilar(lastWords, candidate) {
			continue
		}
		lastWords = candidate

		score := m.evaluateReply(keys, candidate, cfg.SurpriseBias)
		if bestWords == nil || score > bestScore {
			bestScore = score
			bestWords = candidate
		}
	}

	if len(bestWords) == 0 {
		return fallback
	}
	return makeOutput(bestWords)
}

var overrideRe = regexp.MustCompile(`(?i)!(CHAOS|TEMPERATURE|SURPRISE_BIAS|TIMEOUT|HELP|TRAIN)(?:=(\S+))?`)

// ParsedMessage holds the result of parsing overrides from a message.
type ParsedMessage struct {
	Text      string
	Overrides map[string]string
	Help      bool
	TrainURL  string // non-empty if !TRAIN=URL was found
}

func parseOverrides(input string) ParsedMessage {
	result := ParsedMessage{Overrides: make(map[string]string)}
	cleaned := overrideRe.ReplaceAllStringFunc(input, func(match string) string {
		sub := overrideRe.FindStringSubmatch(match)
		key := strings.ToUpper(sub[1])
		switch key {
		case "HELP":
			result.Help = true
		case "TRAIN":
			if sub[2] != "" {
				result.TrainURL = sub[2]
			}
		default:
			if sub[2] != "" {
				result.Overrides[key] = sub[2]
			}
		}
		return ""
	})
	result.Text = strings.Join(strings.Fields(cleaned), " ")
	return result
}

func applyOverrides(base GenerationConfig, kv map[string]string) GenerationConfig {
	cfg := base
	if v, ok := kv["CHAOS"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			cfg.Temperature = f
			cfg.SurpriseBias = f
		}
	}
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
