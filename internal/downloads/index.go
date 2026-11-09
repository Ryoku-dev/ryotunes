package downloads

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

type folderIndex struct {
	stems map[string]string
	keys  map[string]string
}

func buildFolderIndex(root string) folderIndex {
	index := folderIndex{stems: make(map[string]string), keys: make(map[string]string)}
	files := 0
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		depth := strings.Count(relative, string(filepath.Separator)) + 1
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), stagingName) || depth > 6 || entry.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		files++
		if files > 20000 {
			return filepath.SkipAll
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return nil
		}
		extension := strings.ToLower(strings.TrimPrefix(filepath.Ext(entry.Name()), "."))
		if !downloadExtensions[extension] {
			return nil
		}
		stem := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		index.stems[stem] = path
		key := normalizeFileKey(stripIDTag(stem))
		if key != "" {
			if _, exists := index.keys[key]; !exists {
				index.keys[key] = path
			}
		}
		return nil
	})
	return index
}

func (index folderIndex) find(settings model.DownloadSettings, entry model.DownloadCollectionEntry) (string, bool) {
	base := strings.TrimSpace(entry.Title)
	if !settings.OrganizeByArtist && strings.TrimSpace(entry.Artists) != "" {
		base = strings.TrimSpace(entry.Artists) + " - " + base
	}
	stem := sanitizeComponent(base) + " [" + sanitizeComponent(entry.VideoID) + "]"
	if path, ok := index.stems[stem]; ok {
		return path, true
	}
	for suffix := 1; suffix <= 50; suffix++ {
		if path, ok := index.stems[stem+" ("+strconv.Itoa(suffix)+")"]; ok {
			return path, true
		}
	}
	key := normalizeFileKey(stripIDTag(stem))
	path, ok := index.keys[key]
	return path, ok && key != ""
}

func stripIDTag(stem string) string {
	open := strings.LastIndex(stem, " [")
	if open >= 0 && strings.HasSuffix(stem, "]") && open+2 < len(stem) {
		return stem[:open]
	}
	return stem
}

func normalizeFileKey(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
