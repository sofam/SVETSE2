package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const brainCookie = "SVETSE2v1"

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

	if _, err := tmp.Write([]byte(brainCookie)); err != nil {
		return fmt.Errorf("write cookie: %w", err)
	}
	if err := binary.Write(tmp, binary.LittleEndian, uint8(m.Order)); err != nil {
		return fmt.Errorf("write order: %w", err)
	}
	if err := saveTree(tmp, m.Forward); err != nil {
		return fmt.Errorf("write forward tree: %w", err)
	}
	if err := saveTree(tmp, m.Backward); err != nil {
		return fmt.Errorf("write backward tree: %w", err)
	}
	if err := saveDictionary(tmp, m.Dictionary); err != nil {
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

func loadBrain(path string, m *Model) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	cookie := make([]byte, len(brainCookie))
	if _, err := io.ReadFull(f, cookie); err != nil {
		return fmt.Errorf("read cookie: %w", err)
	}
	if string(cookie) != brainCookie {
		return fmt.Errorf("invalid brain file: bad cookie %q", string(cookie))
	}

	var order uint8
	if err := binary.Read(f, binary.LittleEndian, &order); err != nil {
		return fmt.Errorf("read order: %w", err)
	}
	m.Order = int(order)
	m.Context = make([]*Node, m.Order+2)

	m.Forward = newNode()
	if err := loadTree(f, m.Forward); err != nil {
		return fmt.Errorf("read forward tree: %w", err)
	}
	m.Backward = newNode()
	if err := loadTree(f, m.Backward); err != nil {
		return fmt.Errorf("read backward tree: %w", err)
	}

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
