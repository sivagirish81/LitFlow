package util

import (
	"fmt"
	"os"
	"path/filepath"
)

func WriteTextAtomic(path string, content string) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "tmp-*.txt")
	if err != nil {
		return fmt.Errorf("create temp text: %w", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp text: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp text: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename temp text: %w", err)
	}
	return nil
}
