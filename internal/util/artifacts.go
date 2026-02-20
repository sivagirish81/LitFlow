package util

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func WriteJSONAtomic(path string, v any) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "tmp-*.json")
	if err != nil {
		return fmt.Errorf("create temp json: %w", err)
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encode json: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp json: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename temp json: %w", err)
	}
	return nil
}

func WriteJSONLinesAtomic(path string, rows []any) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "tmp-*.jsonl")
	if err != nil {
		return fmt.Errorf("create temp jsonl: %w", err)
	}
	w := bufio.NewWriter(tmp)
	for _, row := range rows {
		b, err := json.Marshal(row)
		if err != nil {
			_ = tmp.Close()
			return fmt.Errorf("marshal row: %w", err)
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("write row: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("flush jsonl: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close jsonl: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename jsonl: %w", err)
	}
	return nil
}
