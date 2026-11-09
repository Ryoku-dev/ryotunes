package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data through a same-directory temporary file and rename.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create file directory: %w", err)
	}
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporary := file.Name()
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(temporary)
		}
	}()
	if err := file.Chmod(mode); err != nil {
		return fmt.Errorf("set temporary file mode: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("publish file: %w", err)
	}
	keep = true
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("secure published file: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open file directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync file directory: %w", err)
	}
	return nil
}
