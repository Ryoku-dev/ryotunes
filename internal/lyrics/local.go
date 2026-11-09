package lyrics

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/dhowden/tag"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

const companionLimit = 512 << 10

func pathFromLocalID(id string) (string, bool) {
	path, ok := strings.CutPrefix(id, "LOCAL:")
	return path, ok && path != ""
}

func localLyrics(audio string) (*model.Lyrics, error) {
	base := strings.TrimSuffix(audio, filepath.Ext(audio))
	if text, err := readCompanion(base + ".lrc"); err == nil && text != "" {
		if lyrics := lyricsFromLines("Local LRC", parseLRC(text)); lyrics != nil {
			return lyrics, nil
		}
	}
	if text, err := readEmbeddedLyrics(audio); err == nil {
		if lyrics := plainLyrics(text, "Embedded lyrics"); lyrics != nil {
			return lyrics, nil
		}
	}
	if text, err := readCompanion(base + ".txt"); err == nil && text != "" {
		if lyrics := plainLyrics(text, "Local lyrics"); lyrics != nil {
			return lyrics, nil
		}
	}
	return nil, nil
}

func readCompanion(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > companionLimit {
		return "", errors.New("lyrics companion is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, companionLimit+1))
	if err != nil {
		return "", err
	}
	if len(data) > companionLimit {
		return "", errors.New("lyrics companion exceeds size limit")
	}
	if !utf8.Valid(data) {
		return "", errors.New("lyrics companion is not UTF-8")
	}
	return string(data), nil
}

func readEmbeddedLyrics(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	metadata, err := tag.ReadFrom(file)
	if err != nil {
		return "", err
	}
	return metadata.Lyrics(), nil
}
