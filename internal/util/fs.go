package util

import (
	"fmt"
	"os"
	"path/filepath"
)

func EnsureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", path, err)
	}
	return nil
}

func SafeJoin(root, name string) string {
	return filepath.Join(root, filepath.Base(name))
}
