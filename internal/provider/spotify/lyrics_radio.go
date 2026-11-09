package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
	providerapi "github.com/ryoku-dev/ryotunes/internal/provider"
)

type colorLyricsAnswer struct {
	Lyrics *struct {
		SyncType string `json:"syncType"`
		Lines    []struct {
			Start     string `json:"startTimeMs"`
			End       string `json:"endTimeMs"`
			Words     string `json:"words"`
			Syllables []struct {
				Start string `json:"startTimeMs"`
				End   string `json:"endTimeMs"`
				Text  string `json:"text"`
			} `json:"syllables"`
		} `json:"lines"`
	} `json:"lyrics"`
}

// Lyrics implements provider.Lyricist using Spotify colour-lyrics.
func (p *Spotify) Lyrics(ctx context.Context, track model.Track) (*model.Lyrics, error) {
	bare, err := bareID(track.VideoID, "track")
	if err != nil {
		return nil, err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return nil, err
	}
	token, err := sess.Spclient().GetAccessToken(ctx, false)
	if err != nil {
		return nil, mapProviderError(err)
	}
	endpoint := "https://spclient.wg.spotify.com/color-lyrics/v2/track/" + url.PathEscape(bare) + "?format=json&vocalRemoval=false&market=from_token"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("app-platform", "WebPlayer")
	req.Header.Set("Authorization", "Bearer "+token)
	p.mu.RLock()
	clientToken := p.clientToken
	p.mu.RUnlock()
	if clientToken != "" {
		req.Header.Set("Client-Token", clientToken)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, mapProviderError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, providerapi.ErrSignInRequired
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, providerapi.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapProviderError(fmt.Errorf("Spotify lyrics status %d", resp.StatusCode))
	}
	var answer colorLyricsAnswer
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&answer); err != nil {
		return nil, mapProviderError(err)
	}
	if answer.Lyrics == nil {
		return nil, nil
	}
	lyrics := &model.Lyrics{Source: "Spotify", Synced: answer.Lyrics.SyncType != "UNSYNCED"}
	for _, line := range answer.Lyrics.Lines {
		text := strings.TrimSpace(line.Words)
		if text == "" || text == "♪" {
			continue
		}
		entry := model.LyricsLine{Text: text}
		if lyrics.Synced {
			stamp, ok := millis(line.Start)
			if !ok {
				continue
			}
			entry.TimeMS = new(stamp)
			if end, ok := millis(line.End); ok && end > stamp {
				entry.EndTimeMS = new(end)
			}
			for _, word := range line.Syllables {
				start, ok1 := millis(word.Start)
				end, ok2 := millis(word.End)
				if ok1 && ok2 {
					entry.Words = append(entry.Words, model.LyricsWord{Text: word.Text, StartMS: start, EndMS: end})
				}
			}
		}
		lyrics.Lines = append(lyrics.Lines, entry)
	}
	if len(lyrics.Lines) == 0 {
		return nil, nil
	}
	if lyrics.Synced {
		sort.SliceStable(lyrics.Lines, func(i, j int) bool { return *lyrics.Lines[i].TimeMS < *lyrics.Lines[j].TimeMS })
		for i := range len(lyrics.Lines) - 1 {
			if lyrics.Lines[i].EndTimeMS == nil && *lyrics.Lines[i+1].TimeMS > *lyrics.Lines[i].TimeMS {
				lyrics.Lines[i].EndTimeMS = new(*lyrics.Lines[i+1].TimeMS)
			}
		}
	}
	return lyrics, nil
}

func millis(value string) (uint64, bool) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return parsed, err == nil
}

// StartRadio implements provider.Radio for Spotify track seeds.
func (p *Spotify) StartRadio(ctx context.Context, kind, id string) ([]model.Track, string, error) {
	if kind != "" && kind != "track" {
		return nil, "", fmt.Errorf("Spotify radio supports track seeds only")
	}
	bare, err := bareID(id, "track")
	if err != nil {
		return nil, "", err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return nil, "", err
	}
	endpoint := "/radio-apollo/v3/stations/" + url.PathEscape("spotify:track:"+bare)
	resp, err := sess.Spclient().Request(ctx, http.MethodGet, endpoint, nil, nil, nil)
	if err != nil {
		return nil, "", mapProviderError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", mapProviderError(fmt.Errorf("Spotify radio status %d", resp.StatusCode))
	}
	var answer struct {
		MediaItems []struct {
			URI string `json:"uri"`
		} `json:"mediaItems"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&answer); err != nil {
		return nil, "", mapProviderError(err)
	}
	playlistID := ""
	for _, item := range answer.MediaItems {
		if strings.HasPrefix(item.URI, "spotify:playlist:") {
			playlistID = item.URI
			break
		}
	}
	if playlistID == "" {
		return nil, "", fmt.Errorf("%w: track radio response did not contain a playlist", providerapi.ErrUnavailable)
	}
	page, err := p.Playlist(ctx, playlistID, model.PlaylistSortDefault, false)
	if err != nil {
		return nil, "", err
	}
	return page.Items, "Track Radio", nil
}
