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
