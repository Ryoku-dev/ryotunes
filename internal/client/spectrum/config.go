package spectrum

import (
	"errors"
	"os"
	"path/filepath"
)

const (
	configDirectoryMode = 0o700
	configFileMode      = 0o600
)

func writeConfig(source string) (string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return "", errors.New("XDG_RUNTIME_DIR is not set")
	}
	dir := filepath.Join(runtimeDir, "ryotunes")
	if err := os.MkdirAll(dir, configDirectoryMode); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, configDirectoryMode); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "cava.conf")
	if source != automaticSource {
		source += ".monitor"
	}
	data := []byte("[general]\n" +
		"framerate = 60\n" +
		"bars = 64\n" +
		"autosens = 1\n\n" +
		"[input]\n" +
		"method = pipewire\n" +
		"source = " + source + "\n\n" +
		"[output]\n" +
		"method = raw\n" +
		"raw_target = /dev/stdout\n" +
		"data_format = ascii\n" +
		"ascii_max_range = 1000\n" +
		"bar_delimiter = 59\n" +
		"frame_delimiter = 10\n" +
		"channels = mono\n" +
		"mono_option = average\n\n" +
		"[smoothing]\n" +
		"noise_reduction = 50\n")
	if err := os.WriteFile(path, data, configFileMode); err != nil {
		return "", err
	}
	if err := os.Chmod(path, configFileMode); err != nil {
		return "", err
	}
	return path, nil
}
