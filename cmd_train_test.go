package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrainFromFile(t *testing.T) {
	// Create a temp training file
	dir := t.TempDir()
	trainFile := filepath.Join(dir, "train.txt")
	content := `The cat sat on the mat and looked around the room.
The dog ran through the park and chased the birds away.
# This is a comment and should be skipped.
Birds fly high over the mountains and rivers below.

The fish swam in the river under the old stone bridge.`

	os.WriteFile(trainFile, []byte(content), 0644)

	model := newModel(5)
	count, err := trainFromFile(model, trainFile)
	if err != nil {
		t.Fatalf("trainFromFile: %v", err)
	}

	// 4 sentences (comment and empty line skipped)
	if count != 4 {
		t.Errorf("trained %d sentences, want 4", count)
	}

	// Dictionary should have learned words
	if len(model.Dictionary) < 10 {
		t.Errorf("dictionary only has %d words, expected more", len(model.Dictionary))
	}
}

func TestTrainFromFileMissing(t *testing.T) {
	model := newModel(5)
	_, err := trainFromFile(model, "/nonexistent/file.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCleanWikipediaText(t *testing.T) {
	input := "Albert Einstein[1] was a physicist.[2] He developed the theory of relativity.[3]"
	cleaned := cleanWikipediaText(input)

	if strings.Contains(cleaned, "[1]") {
		t.Error("references not removed")
	}
	if strings.Contains(cleaned, "[2]") {
		t.Error("references not removed")
	}
	if !strings.Contains(cleaned, "Albert Einstein") {
		t.Error("content lost")
	}
	if !strings.Contains(cleaned, "physicist") {
		t.Error("content lost")
	}
}

func TestCleanWikipediaTextHeaders(t *testing.T) {
	input := "Some text.\n== Early life ==\nMore text.\n=== Education ===\nEven more."
	cleaned := cleanWikipediaText(input)

	if strings.Contains(cleaned, "==") {
		t.Errorf("headers not removed: %q", cleaned)
	}
	if !strings.Contains(cleaned, "Some text") {
		t.Error("content lost")
	}
	if !strings.Contains(cleaned, "More text") {
		t.Error("content lost")
	}
}

func TestSplitSentences(t *testing.T) {
	input := "The cat sat on the mat. The dog ran through the park! Did the bird fly? It did."
	sentences := splitSentences(input)

	if len(sentences) != 4 {
		t.Fatalf("got %d sentences, want 4: %v", len(sentences), sentences)
	}
	if sentences[0] != "The cat sat on the mat." {
		t.Errorf("sentence 0: %q", sentences[0])
	}
	if sentences[1] != "The dog ran through the park!" {
		t.Errorf("sentence 1: %q", sentences[1])
	}
}

func TestExtractWikiArticle(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://en.wikipedia.org/wiki/Albert_Einstein", "Albert_Einstein"},
		{"https://en.wikipedia.org/wiki/Go_(programming_language)", "Go_(programming_language)"},
		{"https://sv.wikipedia.org/wiki/Sverige", "Sverige"},
		{"not-a-url", ""},
	}

	for _, tt := range tests {
		got := extractWikiArticle(tt.url)
		if got != tt.want {
			t.Errorf("extractWikiArticle(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestIsWikipediaURL(t *testing.T) {
	if !isWikipediaURL("https://en.wikipedia.org/wiki/Cat") {
		t.Error("should detect Wikipedia URL")
	}
	if isWikipediaURL("https://google.com") {
		t.Error("should not detect non-Wikipedia URL")
	}
}

func TestTrainFullRoundTrip(t *testing.T) {
	dir := t.TempDir()
	brainPath := filepath.Join(dir, "train-test.brain")
	trainFile := filepath.Join(dir, "corpus.txt")

	// Write a training corpus
	var lines []string
	corpus := []string{
		"The cat sat on the mat and looked around the room.",
		"The dog ran through the park and chased the birds away.",
		"Birds fly high over the mountains and rivers below.",
		"The fish swam in the river under the old stone bridge.",
		"Mountains rise above the clouds every single morning.",
	}
	// Repeat for enough training data
	for i := 0; i < 20; i++ {
		lines = append(lines, corpus...)
	}
	os.WriteFile(trainFile, []byte(strings.Join(lines, "\n")), 0644)

	// Train
	model := newModel(5)
	count, err := trainFromFile(model, trainFile)
	if err != nil {
		t.Fatalf("trainFromFile: %v", err)
	}
	if count != 100 {
		t.Errorf("trained %d sentences, want 100", count)
	}

	// Save
	if err := saveBrain(brainPath, model); err != nil {
		t.Fatalf("saveBrain: %v", err)
	}

	// Load and verify
	model2 := newModel(5)
	if err := loadBrain(brainPath, model2); err != nil {
		t.Fatalf("loadBrain: %v", err)
	}

	if len(model2.Dictionary) != len(model.Dictionary) {
		t.Errorf("dictionary size mismatch: %d vs %d", len(model2.Dictionary), len(model.Dictionary))
	}
}
