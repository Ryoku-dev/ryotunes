package spotify

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	extmetadatapb "github.com/devgianlu/go-librespot/proto/spotify/extendedmetadata"
	metadatapb "github.com/devgianlu/go-librespot/proto/spotify/metadata"
	"github.com/devgianlu/go-librespot/session"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func (p *Spotify) trackMetadata(ctx context.Context, sess *session.Session, uris []string) (map[string]model.Track, error) {
	known := make(map[string]model.Track, len(uris))
	for start := 0; start < len(uris); start += 500 {
		end := min(start+500, len(uris))
		requests := make([]*extmetadatapb.EntityRequest, 0, end-start)
		for _, uri := range uris[start:end] {
			requests = append(requests, &extmetadatapb.EntityRequest{EntityUri: uri, Query: []*extmetadatapb.ExtensionQuery{{ExtensionKind: extmetadatapb.ExtensionKind_TRACK_V4}}})
		}
		answer, err := sess.Spclient().ExtendedMetadata(ctx, &extmetadatapb.BatchedEntityRequest{EntityRequest: requests})
		if err != nil {
			return nil, err
		}
		for _, group := range answer.GetExtendedMetadata() {
			if group.GetExtensionKind() != extmetadatapb.ExtensionKind_TRACK_V4 {
				continue
			}
			for _, entity := range group.GetExtensionData() {
				if entity.GetHeader().GetStatusCode() != 200 || entity.ExtensionData == nil {
					continue
				}
				var raw metadatapb.Track
				if entity.ExtensionData.UnmarshalTo(&raw) != nil {
					continue
				}
				known[entity.GetEntityUri()] = mapProtoTrack(entity.GetEntityUri(), &raw)
			}
		}
	}
	return known, nil
}

func mapProtoTrack(uri string, raw *metadatapb.Track) model.Track {
	artists := make([]model.Artist, 0, len(raw.GetArtist()))
	for _, artist := range raw.GetArtist() {
		if artist.GetName() == "" {
			continue
		}
		var id *string
		if len(artist.GetGid()) == 16 {
			id = new("spotify:artist:" + librespot.GidToBase62(artist.GetGid()))
		}
		artists = append(artists, model.Artist{Text: artist.GetName(), ID: id})
	}
	artistNames := make([]string, 0, len(artists))
	for _, artist := range artists {
		artistNames = append(artistNames, artist.Text)
	}
	track := model.Track{VideoID: uri, Title: raw.GetName(), Artists: strings.Join(artistNames, ", "), ArtistRuns: artists, Explicit: raw.GetExplicit()}
	if track.Title == "" {
		track.Title = "Unknown"
	}
	if track.Artists == "" {
		track.Artists = "Unknown"
	}
	if len(artists) > 0 {
		track.ArtistID = artists[0].ID
	}
	if album := raw.GetAlbum(); album != nil {
		if name := album.GetName(); name != "" {
			track.Album = new(name)
		}
		if len(album.GetGid()) == 16 {
			track.AlbumID = new("spotify:album:" + librespot.GidToBase62(album.GetGid()))
		}
		track.Thumbnail = coverURL(album.GetCoverGroup(), false)
	}
	if duration := raw.GetDuration(); duration > 0 {
		track.Duration = new(durationLabel(time.Duration(duration) * time.Millisecond))
	}
	return track
}

func coverURL(group *metadatapb.ImageGroup, largest bool) *string {
	if group == nil || len(group.GetImage()) == 0 {
		return nil
	}
	var picked *metadatapb.Image
	for _, image := range group.GetImage() {
		if len(image.GetFileId()) == 0 {
			continue
		}
		if picked == nil {
			picked = image
			continue
		}
		if largest && image.GetSize() > picked.GetSize() {
			picked = image
		}
		if !largest && image.GetSize() < picked.GetSize() {
			picked = image
		}
	}
	if picked == nil {
		return nil
	}
	return new("https://i.scdn.co/image/" + hex.EncodeToString(picked.GetFileId()))
}

func durationLabel(duration time.Duration) string {
	seconds := int(duration / time.Second)
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func countLabel(count uint64) *string {
	if count == 0 {
		return nil
	}
	return new(strconv.FormatUint(count, 10))
}

func bareID(value, kind string) (string, error) {
	prefix := "spotify:" + kind + ":"
	if strings.HasPrefix(value, prefix) {
		value = strings.TrimPrefix(value, prefix)
	}
	if !validBase62(value) {
		return "", fmt.Errorf("invalid Spotify %s id", kind)
	}
	if kind == "track" || kind == "album" || kind == "artist" || kind == "playlist" {
		if _, err := librespot.SpotifyIdFromBase62(librespot.SpotifyIdType(kind), value); err != nil {
			return "", fmt.Errorf("invalid Spotify %s id", kind)
		}
	}
	return value, nil
}

func validBase62(value string) bool {
	if len(value) != 21 && len(value) != 22 {
		return false
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
			return false
		}
	}
	return true
}
