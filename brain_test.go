package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadBrain(t *testing.T) {
	m := newModel(5)
	sentences := []string{
		"The cat sat on the mat and looked at the birds",
		"The dog ran through the park chasing the ball",
		"Birds fly over the mountains and rivers below",
		"Hello world this is a test of the brain save system",
	}
	for _, s := range sentences {
		m.learn(s)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.brain")
	err := saveBrain(path, m)
	if err != nil {
		t.Fatalf("saveBrain: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("brain file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("brain file is empty")
	}

	m2 := newModel(5)
	err = loadBrain(path, m2)
	if err != nil {
		t.Fatalf("loadBrain: %v", err)
	}

	if len(m2.Dictionary) != len(m.Dictionary) {
		t.Errorf("Dictionary size: got %d, want %d", len(m2.Dictionary), len(m.Dictionary))
	}
	for i, w := range m.Dictionary {
		if i < len(m2.Dictionary) && m2.Dictionary[i] != w {
			t.Errorf("Dictionary[%d]: got %q, want %q", i, m2.Dictionary[i], w)
		}
	}
	for word, id := range m.DictMap {
		if m2.DictMap[word] != id {
			t.Errorf("DictMap[%q]: got %d, want %d", word, m2.DictMap[word], id)
		}
	}
	if m2.Order != m.Order {
		t.Errorf("Order: got %d, want %d", m2.Order, m.Order)
	}
	if len(m2.Forward.Children) == 0 {
		t.Error("loaded Forward tree has no children")
	}
	if len(m2.Backward.Children) == 0 {
		t.Error("loaded Backward tree has no children")
	}
	if m2.Forward.Usage != m.Forward.Usage {
		t.Errorf("Forward.Usage: got %d, want %d", m2.Forward.Usage, m.Forward.Usage)
	}
}

func TestSaveAtomicity(t *testing.T) {
	m := newModel(5)
	m.learn("The cat sat on the mat and purred loudly")

	dir := t.TempDir()
	path := filepath.Join(dir, "test.brain")
	if err := saveBrain(path, m); err != nil {
		t.Fatalf("first save: %v", err)
	}
	info1, _ := os.Stat(path)

	m.learn("The dog barked at the mailman every single morning")
	if err := saveBrain(path, m); err != nil {
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
