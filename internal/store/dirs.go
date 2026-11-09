package store

import (
	"fmt"
	"os"
	"path/filepath"
)

const appDataName = "dev.ryoku.ryotunes"

// Dirs contains every directory used by the daemon.
type Dirs struct {
	Data    string
	Cache   string
	Config  string
	State   string
	Runtime string
}

// ResolveDirs resolves absolute XDG paths and ignores relative XDG overrides.
func ResolveDirs() (Dirs, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Dirs{}, fmt.Errorf("resolve home directory: %w", err)
	}
	return Dirs{
		Data:    filepath.Join(xdgHome("XDG_DATA_HOME", filepath.Join(home, ".local", "share")), appDataName),
		Cache:   filepath.Join(xdgHome("XDG_CACHE_HOME", filepath.Join(home, ".cache")), appDataName, "audio"),
		Config:  filepath.Join(xdgHome("XDG_CONFIG_HOME", filepath.Join(home, ".config")), "ryotunes"),
		State:   filepath.Join(xdgHome("XDG_STATE_HOME", filepath.Join(home, ".local", "state")), "ryotunes"),
		Runtime: runtimeDir(),
	}, nil
}

func xdgHome(name, fallback string) string {
	if value := os.Getenv(name); filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return fallback
}

func runtimeDir() string {
	if value := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(value) {
		return filepath.Join(filepath.Clean(value), "ryotunes")
	}
	return filepath.Join("/tmp", fmt.Sprintf("ryotunes-%d", os.Getuid()))
}

func (d Dirs) ensure() error {
	for name, path := range map[string]string{
		"data": d.Data, "cache": d.Cache, "config": d.Config, "state": d.State, "runtime": d.Runtime,
	} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("%s directory must be absolute", name)
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create %s directory: %w", name, err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("secure %s directory: %w", name, err)
		}
	}
	for _, path := range []string{
		filepath.Join(d.Data, "covers", "playlists"),
		filepath.Join(d.Data, "logs"),
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create application directory: %w", err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("secure application directory: %w", err)
		}
	}
	return nil
}
