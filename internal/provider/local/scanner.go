package local

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dhowden/tag"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	maxDepth      = 10
	maxCoverBytes = 16 << 20
	keyCharacters = 80
)

var audioExtensions = map[string]struct{}{
	".mp3": {}, ".flac": {}, ".m4a": {}, ".m4b": {}, ".aac": {},
	".ogg": {}, ".oga": {}, ".opus": {}, ".wav": {}, ".wma": {},
	".aiff": {}, ".aif": {}, ".ape": {}, ".wv": {}, ".mka": {},
}

var sidecarNames = [...]string{
	"cover.jpg", "cover.png", "folder.jpg", "folder.png", "front.jpg", "album.jpg",
}

// Scanner walks configured roots and maintains the persisted local index.
type Scanner struct {
	index    Index
	cacheDir string
	ffprobe  string
	log      *slog.Logger
}

// NewScanner creates a scanner; cacheDir is the destination for extracted artwork.
func NewScanner(index Index, cacheDir, ffprobe string, log *slog.Logger) *Scanner {
	if ffprobe == "" {
		ffprobe = "ffprobe"
	}
	if log == nil {
		log = slog.Default()
	}
	return &Scanner{index: index, cacheDir: cacheDir, ffprobe: ffprobe, log: log}
}

// Scan refreshes changed files, prunes missing rows, and returns the complete library.
func (s *Scanner) Scan(ctx context.Context) (model.LocalLibrary, error) {
	if err := ctx.Err(); err != nil {
		return model.LocalLibrary{}, err
	}
	folders, err := s.index.LocalFolders()
	if err != nil {
		return model.LocalLibrary{}, err
	}
	before, err := s.index.AllLocalTracks()
	if err != nil {
		return model.LocalLibrary{}, err
	}
	known := make(map[string]int64, len(before))
	for _, track := range before {
		known[track.Path] = track.MTime
	}
	version, err := s.index.LocalScanVersion()
	if err != nil {
		return model.LocalLibrary{}, err
	}
	reparse := version != ScanVersion
	found := make([]string, 0, len(before))
	seenFiles := make(map[string]struct{}, len(before))
	seenDirs := make(map[string]struct{})
	for _, folder := range folders {
		err = walkAudio(ctx, folder, 0, seenDirs, func(path string, info os.FileInfo) error {
			if _, duplicate := seenFiles[path]; duplicate {
				return nil
			}
			seenFiles[path] = struct{}{}
			found = append(found, path)
			mtime := info.ModTime().Unix()
			previous, knownPath := known[path]
			if !reparse && knownPath && previous == mtime {
				return nil
			}
			track := s.readTrack(ctx, path, mtime)
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := s.index.UpsertLocalTrack(track); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return model.LocalLibrary{}, err
		}
	}
	if reparse {
		if err := s.index.SetLocalScanVersion(ScanVersion); err != nil {
			return model.LocalLibrary{}, err
		}
	}
	gone, err := s.index.PruneMissingLocalTracks(found)
	if err != nil {
		return model.LocalLibrary{}, err
	}
	after, err := s.index.AllLocalTracks()
	if err != nil {
		return model.LocalLibrary{}, err
	}
	removed := make([]string, 0, len(gone)+3)
	for _, path := range gone {
		removed = append(removed, SongID(path))
	}
	beforeCards := cardIDs(before)
	afterCards := cardIDs(after)
	for id := range beforeCards {
		if _, present := afterCards[id]; !present {
			removed = append(removed, id)
		}
	}
	sortStrings(removed)
	return model.LocalLibrary{
		Folders: append([]string(nil), folders...),
		Albums:  albumsOf(after),
		Artists: artistsOf(after),
		Songs:   songsOf(after),
		Removed: removed,
	}, nil
}

func walkAudio(ctx context.Context, dir string, depth int, seen map[string]struct{}, visit func(string, os.FileInfo) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > maxDepth {
		return nil
	}
	identity := canonicalDirectory(dir)
	if _, duplicate := seen[identity]; duplicate {
		return nil
	}
	seen[identity] = struct{}{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			if err := walkAudio(ctx, path, depth+1, seen, visit); err != nil {
				return err
			}
			continue
		}
		if info.Mode().IsRegular() && acceptedAudio(path) {
			if err := visit(path, info); err != nil {
				return err
			}
		}
	}
	return nil
}

func canonicalDirectory(path string) string {
	canonical, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = canonical
	}
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func acceptedAudio(path string) bool {
	_, ok := audioExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func (s *Scanner) readTrack(ctx context.Context, path string, mtime int64) LocalTrack {
	metadata := readTags(path)
	title := strings.TrimSpace(metadata.title)
	artist := strings.TrimSpace(metadata.artist)
	album := strings.TrimSpace(metadata.album)
	albumArtist := strings.TrimSpace(metadata.albumArtist)
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	filenameArtist, filenameTitle := splitFilename(stem)
	if title == "" {
		title = filenameTitle
	}
	if artist == "" {
		artist = filenameArtist
	}
	if artist == "" {
		artist = UnknownArtist
	}
	directory := filepath.Dir(path)
	taggedAlbum := album != ""
	var key string
	if taggedAlbum {
		if albumArtist == "" {
			albumArtist = artist
		}
		key = albumKey(albumArtist, album)
	} else {
		album = filepath.Base(directory)
		if album == "." || album == string(filepath.Separator) || album == "" {
			album = "Local music"
		}
		key = folderKey(directory, album)
	}
	duration := metadata.duration
	if duration == 0 {
		duration = s.probeDuration(ctx, path)
	}
	cover := ""
	if taggedAlbum {
		cover = s.embeddedCover(key, metadata.picture)
		if cover == "" {
			cover = s.sidecarCover(key, directory)
		}
	} else {
		cover = s.embeddedCover("file-"+shortDigest(path), metadata.picture)
	}
	return LocalTrack{
		Path: path, Title: title, Artist: artist, Album: album, AlbumKey: key,
		TrackNo: int64(metadata.track), DurationSecs: duration, Cover: cover, MTime: mtime,
	}
}

type tagData struct {
	title, artist, album, albumArtist string
	track                             int
	duration                          int64
	picture                           *tag.Picture
}

func readTags(path string) tagData {
	file, err := os.Open(path)
	if err != nil {
		return tagData{}
	}
	defer file.Close()
	metadata, err := tag.ReadFrom(file)
	if err != nil {
		return tagData{}
	}
	track, _ := metadata.Track()
	return tagData{
		title: metadata.Title(), artist: metadata.Artist(), album: metadata.Album(),
		albumArtist: metadata.AlbumArtist(), track: track, duration: rawDuration(metadata.Raw()),
		picture: metadata.Picture(),
	}
}

func rawDuration(raw map[string]interface{}) int64 {
	for _, key := range []string{"TLEN", "DURATION", "duration"} {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if key == "TLEN" {
			if milliseconds, ok := numericValue(value); ok && milliseconds > 0 {
				return int64(milliseconds / 1000)
			}
			continue
		}
		if seconds, ok := durationValue(value); ok && seconds > 0 {
			return int64(seconds)
		}
	}
	return 0
}

func numericValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return number, err == nil
	case []string:
		if len(typed) > 0 {
			return numericValue(typed[0])
		}
	}
	return 0, false
}

func durationValue(value interface{}) (float64, bool) {
	if number, ok := numericValue(value); ok {
		return number, true
	}
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	parts := strings.Split(strings.TrimSpace(text), ":")
	if len(parts) != 3 {
		return 0, false
	}
	hours, errH := strconv.ParseFloat(parts[0], 64)
	minutes, errM := strconv.ParseFloat(parts[1], 64)
	seconds, errS := strconv.ParseFloat(parts[2], 64)
	if errH != nil || errM != nil || errS != nil {
		return 0, false
	}
	return hours*3600 + minutes*60 + seconds, true
}

func (s *Scanner) probeDuration(ctx context.Context, path string) int64 {
	command := exec.CommandContext(ctx, s.ffprobe,
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path)
	output, err := command.Output()
	if err != nil {
		return 0
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || seconds <= 0 || math.IsInf(seconds, 0) || math.IsNaN(seconds) {
		return 0
	}
	return int64(seconds)
}

func splitFilename(stem string) (string, string) {
	artist, title, found := strings.Cut(stem, " - ")
	if !found || strings.TrimSpace(artist) == "" || strings.TrimSpace(title) == "" {
		return "", stem
	}
	return strings.TrimSpace(artist), strings.TrimSpace(title)
}

func albumKey(artist, album string) string {
	raw := artist + "--" + album
	key := sanitizeKey(raw)
	if utf8.RuneCountInString(raw) > keyCharacters {
		return key + "-" + shortDigest(raw)
	}
	return key
}

func folderKey(directory, name string) string {
	return "dir-" + shortDigest(directory) + "-" + sanitizeKey(name)
}

func sanitizeKey(raw string) string {
	runes := make([]rune, 0, keyCharacters)
	for _, character := range strings.ToLower(raw) {
		if len(runes) == keyCharacters {
			break
		}
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-' {
			runes = append(runes, character)
		} else {
			runes = append(runes, '-')
		}
	}
	return string(runes)
}

func shortDigest(value string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(value))
	return fmt.Sprintf("%012x", hash.Sum64()&0xffffffffffff)
}

func (s *Scanner) embeddedCover(key string, picture *tag.Picture) string {
	if s.cacheDir == "" {
		return ""
	}
	for _, extension := range []string{"jpg", "png"} {
		path := filepath.Join(s.cacheDir, key+"."+extension)
		if regularFile(path) {
			return path
		}
	}
	if picture == nil || len(picture.Data) == 0 || len(picture.Data) > maxCoverBytes {
		return ""
	}
	extension := imageExtension(picture.Data, picture.MIMEType, picture.Ext)
	if extension == "" {
		return ""
	}
	path := filepath.Join(s.cacheDir, key+"."+extension)
	if err := atomicWrite(path, picture.Data); err != nil {
		s.log.Debug("could not cache embedded local artwork", "path", path, "error", err)
		return ""
	}
	return path
}

func (s *Scanner) sidecarCover(key, directory string) string {
	if s.cacheDir == "" {
		return ""
	}
	for _, name := range sidecarNames {
		source := filepath.Join(directory, name)
		info, err := os.Stat(source)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > maxCoverBytes {
			continue
		}
		data, err := readBounded(source, maxCoverBytes)
		if err != nil {
			continue
		}
		extension := imageExtension(data, "", filepath.Ext(source))
		if extension == "" {
			continue
		}
		destination := filepath.Join(s.cacheDir, key+"-sidecar."+extension)
		if destinationInfo, err := os.Stat(destination); err == nil && !info.ModTime().After(destinationInfo.ModTime()) {
			return destination
		}
		if err := atomicWrite(destination, data); err != nil {
			s.log.Debug("could not cache local sidecar artwork", "path", source, "error", err)
			return ""
		}
		return destination
	}
	return ""
}

func readBounded(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, errors.New("file exceeds limit")
	}
	return data, nil
}

func imageExtension(data []byte, mime, extension string) string {
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "jpg"
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "png"
	}
	mime = strings.ToLower(strings.TrimSpace(mime))
	extension = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(extension), "."))
	if mime == "image/jpeg" && (extension == "jpg" || extension == "jpeg" || extension == "") {
		return "jpg"
	}
	if mime == "image/png" && (extension == "png" || extension == "") {
		return "png"
	}
	return ""
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".ryotunes-cover-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func cardIDs(tracks []LocalTrack) map[string]struct{} {
	ids := make(map[string]struct{}, len(tracks)*2)
	for _, track := range tracks {
		ids[AlbumID(track.AlbumKey)] = struct{}{}
		ids[ArtistID(primaryArtist(track.Artist))] = struct{}{}
	}
	return ids
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
