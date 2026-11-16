package lyrics

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	lrclibRoot = "https://lrclib.net/api"
	maxJSON    = 8 << 20
)

type lrclibTrack struct {
	Instrumental bool     `json:"instrumental"`
	PlainLyrics  string   `json:"plainLyrics"`
	SyncedLyrics string   `json:"syncedLyrics"`
	Duration     *float64 `json:"duration"`
	TrackName    string   `json:"trackName"`
	ArtistName   string   `json:"artistName"`
}

func (track *lrclibTrack) best() *model.Lyrics {
	if track == nil {
		return nil
	}
	if track.Instrumental {
		return &model.Lyrics{Source: "LRCLIB", Instrumental: true, Lines: []model.LyricsLine{}}
	}
	if strings.TrimSpace(track.SyncedLyrics) != "" {
		if lyrics := lyricsFromLines("LRCLIB", parseLRC(track.SyncedLyrics)); lyrics != nil {
			return lyrics
		}
	}
	return plainLyrics(track.PlainLyrics, "LRCLIB")
}

func describeLRCLIB(track lrclibTrack) candidate {
	about := candidate{title: track.TrackName, artists: track.ArtistName}
	if track.Duration != nil {
		about.duration, about.known = *track.Duration, true
	}
	return about
}

// lrclibExact asks LRCLIB for the song by its exact names. A tempo variant
// leaves the length out so LRCLIB answers with the original to retime.
func (s *Service) lrclibExact(ctx context.Context, q query) (*model.Lyrics, error) {
	values := url.Values{"track_name": {q.Title}, "artist_name": {q.Artists}}
	if q.Album != nil && strings.TrimSpace(*q.Album) != "" {
		values.Set("album_name", *q.Album)
	}
	if q.Duration != nil && !q.Variant {
		values.Set("duration", strconv.FormatInt(int64(*q.Duration+0.5), 10))
	}
	var track lrclibTrack
	status, err := s.getJSON(ctx, 15*time.Second, lrclibRoot+"/get?"+values.Encode(), nil, &track)
	if status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	hit, how, ok := pick(q, []lrclibTrack{track}, describeLRCLIB)
	if !ok {
		return nil, nil
	}
	return how.apply(hit.best()), nil
}

func (s *Service) lrclibSearch(ctx context.Context, q query) (*model.Lyrics, error) {
	values := url.Values{"track_name": {q.Title}, "artist_name": {q.Artists}}
	var tracks []lrclibTrack
	_, err := s.getJSON(ctx, 15*time.Second, lrclibRoot+"/search?"+values.Encode(), nil, &tracks)
	if err != nil {
		return nil, err
	}
	synced := make([]lrclibTrack, 0, len(tracks))
	plain := make([]lrclibTrack, 0, len(tracks))
	for _, track := range tracks {
		if strings.TrimSpace(track.SyncedLyrics) != "" {
			synced = append(synced, track)
		} else {
			plain = append(plain, track)
		}
	}
	for _, candidates := range [][]lrclibTrack{synced, plain} {
		if hit, how, ok := pick(q, candidates, describeLRCLIB); ok {
			return how.apply(hit.best()), nil
		}
	}
	return nil, nil
}

// boidu asks Boidu, which matches names and length on its side and answers
// with TTML that states the recording's length; that length is checked
// like any other provider's.
func (s *Service) boidu(ctx context.Context, q query) (fetchOutcome, error) {
	values := url.Values{"s": {q.Title}, "a": {q.Artists}}
	if q.Album != nil && strings.TrimSpace(*q.Album) != "" {
		values.Set("al", *q.Album)
	}
	if q.Duration != nil {
		values.Set("d", strconv.FormatInt(int64(*q.Duration+0.5), 10))
	}
	var response map[string]any
	status, err := s.getJSON(ctx, 8*time.Second, "https://lyrics-api.boidu.dev/getLyrics?"+values.Encode(), nil, &response)
	if status == http.StatusNotFound {
		return fetchOutcome{definitive: true}, nil
	}
	if err != nil {
		return fetchOutcome{}, err
	}
	for _, key := range []string{"ttml", "syncedLyrics", "lyrics", "lrc"} {
		value, ok := response[key]
		if !ok {
			continue
		}
		var text string
		if raw, ok := value.(string); ok {
			text = raw
		} else if encoded, marshalErr := json.Marshal(value); marshalErr == nil {
			text = string(encoded)
		}
		lyrics := lyricsFromLines("Boidu", parseProviderText(text))
		if lyrics == nil {
			continue
		}
		about := candidate{title: q.Title, artists: q.Artists}
		about.duration, about.known = ttmlDuration(text)
		_, how, matched := pick(q, []candidate{about}, func(value candidate) candidate { return value })
		if !matched {
			return fetchOutcome{definitive: true}, nil
		}
		return fetchOutcome{lyrics: how.apply(lyrics), definitive: true}, nil
	}
	return fetchOutcome{definitive: true}, nil
}

func (s *Service) netease(ctx context.Context, q query) (fetchOutcome, error) {
	form := url.Values{"s": {strings.TrimSpace(q.Title + " " + q.Artists)}, "type": {"1"}, "limit": {"5"}, "offset": {"0"}}
	headers := map[string]string{"Referer": "https://music.163.com/"}
	var search map[string]any
	_, err := s.postFormJSON(ctx, 8*time.Second, "https://music.163.com/api/search/get", form, headers, &search)
	if err != nil {
		return fetchOutcome{}, err
	}
	songs := objectSlice(pointer(search, "result", "songs"))
	hit, how, ok := pick(q, songs, func(song map[string]any) candidate {
		duration, known := number(song["duration"])
		return candidate{title: stringValue(song["name"]), artists: neteaseArtists(song["artists"]), duration: duration / 1000, known: known}
	})
	if !ok {
		return fetchOutcome{definitive: true}, nil
	}
	id, ok := integerString(hit["id"])
	if !ok {
		return fetchOutcome{definitive: true}, nil
	}
	var response map[string]any
	endpoint := "https://music.163.com/api/song/lyric?" + url.Values{"id": {id}, "lv": {"1"}, "kv": {"1"}, "tv": {"-1"}}.Encode()
	_, err = s.getJSON(ctx, 8*time.Second, endpoint, headers, &response)
	if err != nil {
		return fetchOutcome{}, err
	}
	lrc := nestedString(response, "lrc", "lyric")
	if strings.TrimSpace(lrc) == "" {
		return fetchOutcome{definitive: true}, nil
	}
	lines := parseProviderText(lrc)
	if wordLRC := nestedString(response, "klyric", "lyric"); strings.TrimSpace(wordLRC) != "" {
		lines = muxLines(lines, parseProviderText(wordLRC))
	}
	if translation := nestedString(response, "tlyric", "lyric"); strings.TrimSpace(translation) != "" {
		translations := parseLRC(translation)
		for i := range lines {
			if lines[i].TimeMS == nil {
				continue
			}
			for _, translated := range translations {
				if translated.TimeMS != nil && *translated.TimeMS == *lines[i].TimeMS && strings.TrimSpace(translated.Text) != "" {
					value := translated.Text
					lines[i].Translation = &value
					break
				}
			}
		}
	}
	return fetchOutcome{lyrics: how.apply(lyricsFromLines("Netease Cloud Music", lines)), definitive: true}, nil
}

func (s *Service) qqMusic(ctx context.Context, q query) (fetchOutcome, error) {
	search := strings.TrimSpace(q.Title + " " + q.Artists)
	endpoint := "https://c.y.qq.com/soso/fcgi-bin/client_search_cp?" + url.Values{"w": {search}, "format": {"json"}}.Encode()
	headers := map[string]string{"Referer": "https://y.qq.com/"}
	var found map[string]any
	_, err := s.getJSON(ctx, 8*time.Second, endpoint, headers, &found)
	if err != nil {
		return fetchOutcome{}, err
	}
	songs := objectSlice(pointer(found, "data", "song", "list"))
	hit, how, ok := pick(q, songs, func(song map[string]any) candidate {
		duration, known := number(song["interval"])
		return candidate{title: stringValue(song["songname"]), artists: qqArtists(song["singer"]), duration: duration, known: known}
	})
	if !ok {
		return fetchOutcome{definitive: true}, nil
	}
	mid := stringValue(hit["songmid"])
	if mid == "" {
		return fetchOutcome{definitive: true}, nil
	}
	lyricEndpoint := "https://c.y.qq.com/lyric/fcgi-bin/fcg_query_lyric_new.fcg?" + url.Values{"songmid": {mid}, "format": {"json"}, "nobase64": {"1"}}.Encode()
	var response map[string]any
	_, err = s.getJSON(ctx, 8*time.Second, lyricEndpoint, headers, &response)
	if err != nil {
		return fetchOutcome{}, err
	}
	lyric := stringValue(response["lyric"])
	if decoded, decodeErr := base64.StdEncoding.DecodeString(lyric); decodeErr == nil && utf8.Valid(decoded) {
		lyric = string(decoded)
	}
	return fetchOutcome{lyrics: how.apply(lyricsFromLines("QQ Music", parseProviderText(lyric))), definitive: true}, nil
}

func (s *Service) kugou(ctx context.Context, q query) (fetchOutcome, error) {
	keyword := strings.TrimSpace(q.Title + " " + q.Artists)
	endpoint := "https://songsearch.kugou.com/song_search_v2?" + url.Values{"keyword": {keyword}, "page": {"1"}, "pagesize": {"5"}}.Encode()
	var search map[string]any
	_, err := s.getJSON(ctx, 8*time.Second, endpoint, nil, &search)
	if err != nil {
		return fetchOutcome{}, err
	}
	songs := objectSlice(pointer(search, "data", "lists"))
	hit, how, ok := pick(q, songs, func(song map[string]any) candidate {
		duration, known := number(song["Duration"])
		return candidate{title: stringValue(song["SongName"]), artists: stringValue(song["SingerName"]), duration: duration, known: known}
	})
	if !ok {
		return fetchOutcome{definitive: true}, nil
	}
	hash := stringValue(hit["FileHash"])
	if hash == "" {
		return fetchOutcome{definitive: true}, nil
	}
	candidateEndpoint := "https://krcs.kugou.com/search?" + url.Values{"ver": {"1"}, "man": {"yes"}, "client": {"mobi"}, "hash": {hash}}.Encode()
	var candidates map[string]any
	_, err = s.getJSON(ctx, 8*time.Second, candidateEndpoint, nil, &candidates)
	if err != nil {
		return fetchOutcome{}, err
	}
	list := objectSlice(candidates["candidates"])
	if len(list) == 0 {
		return fetchOutcome{definitive: true}, nil
	}
	id := stringValue(list[0]["id"])
	accessKey := stringValue(list[0]["accesskey"])
	if id == "" || accessKey == "" {
		return fetchOutcome{definitive: true}, nil
	}
	downloadEndpoint := "https://lyrics.kugou.com/download?" + url.Values{"ver": {"1"}, "client": {"pc"}, "id": {id}, "accesskey": {accessKey}, "fmt": {"lrc"}}.Encode()
	var download map[string]any
	_, err = s.getJSON(ctx, 8*time.Second, downloadEndpoint, nil, &download)
	if err != nil {
		return fetchOutcome{}, err
	}
	decoded, err := base64.StdEncoding.DecodeString(stringValue(download["content"]))
	if err != nil || !utf8.Valid(decoded) {
		return fetchOutcome{definitive: true}, nil
	}
	return fetchOutcome{lyrics: how.apply(lyricsFromLines("Kugou", parseProviderText(string(decoded)))), definitive: true}, nil
}

func (s *Service) getJSON(ctx context.Context, timeout time.Duration, endpoint string, headers map[string]string, target any) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	return s.doJSON(request, timeout, headers, target)
}

func (s *Service) postFormJSON(ctx context.Context, timeout time.Duration, endpoint string, form url.Values, headers map[string]string, target any) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.doJSON(request, timeout, headers, target)
}

func (s *Service) doJSON(request *http.Request, timeout time.Duration, headers map[string]string, target any) (int, error) {
	ctx, cancel := context.WithTimeout(request.Context(), timeout)
	defer cancel()
	request = request.Clone(ctx)
	request.Header.Set("User-Agent", s.userAgent)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := s.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return response.StatusCode, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return response.StatusCode, fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxJSON))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return response.StatusCode, err
	}
	return response.StatusCode, nil
}

func pointer(root map[string]any, path ...string) any {
	var current any = root
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[key]
	}
	return current
}

func objectSlice(value any) []map[string]any {
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, value := range list {
		if object, ok := value.(map[string]any); ok {
			out = append(out, object)
		}
	}
	return out
}

func number(value any) (float64, bool) {
	switch value := value.(type) {
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	case float64:
		return value, true
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func integerString(value any) (string, bool) {
	switch value := value.(type) {
	case json.Number:
		if integer, err := value.Int64(); err == nil {
			return strconv.FormatInt(integer, 10), true
		}
		return value.String(), value.String() != ""
	case string:
		return value, value != ""
	case float64:
		return strconv.FormatInt(int64(value), 10), true
	default:
		return "", false
	}
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	if number, ok := value.(json.Number); ok {
		return number.String()
	}
	return ""
}

func nestedString(root map[string]any, path ...string) string {
	return stringValue(pointer(root, path...))
}

func neteaseArtists(value any) string {
	artists := objectSlice(value)
	names := make([]string, 0, len(artists))
	for _, artist := range artists {
		if name := stringValue(artist["name"]); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

func qqArtists(value any) string {
	return neteaseArtists(value)
}

func muxLines(primary, wordSource []model.LyricsLine) []model.LyricsLine {
	for i := range primary {
		if primary[i].TimeMS == nil {
			continue
		}
		for _, candidate := range wordSource {
			if candidate.TimeMS == nil || abs(float64(*candidate.TimeMS)-float64(*primary[i].TimeMS)) > 800 {
				continue
			}
			if len(primary[i].Words) == 0 && len(candidate.Words) != 0 {
				primary[i].Words = append([]model.LyricsWord(nil), candidate.Words...)
			}
			if primary[i].Translation == nil && candidate.Translation != nil {
				value := *candidate.Translation
				primary[i].Translation = &value
			}
			if primary[i].EndTimeMS == nil {
				primary[i].EndTimeMS = candidate.EndTimeMS
			}
			break
		}
	}
	return primary
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
