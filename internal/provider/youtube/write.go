package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

func (c *Client) requireAccount() error {
	c.mu.RLock()
	signedIn := c.cookie != ""
	c.mu.RUnlock()
	if !signedIn {
		return provider.ErrSignInRequired
	}
	return nil
}

// Rate likes, dislikes, or clears the rating on a track.
func (c *Client) Rate(ctx context.Context, trackID string, rating model.Rating) error {
	if err := c.requireAccount(); err != nil {
		return err
	}
	path := "like/removelike"
	switch rating {
	case model.RatingLike:
		path = "like/like"
	case model.RatingDislike:
		path = "like/dislike"
	case model.RatingIndifferent:
	default:
		return fmt.Errorf("youtube: invalid rating %q", rating)
	}
	_, err := c.do(ctx, "WEB_REMIX", path, map[string]any{"target": map[string]any{"videoId": trackID}}, true)
	return err
}

// SetAlbumSaved adds or removes an album playlist from the account library.
func (c *Client) SetAlbumSaved(ctx context.Context, albumID string, saved bool) error {
	if err := c.requireAccount(); err != nil {
		return err
	}
	targetID := strings.TrimPrefix(albumID, "VL")
	if strings.HasPrefix(albumID, "MPRE") {
		root, browseErr := c.browse(ctx, "WEB_REMIX", albumID, "")
		if browseErr != nil {
			return browseErr
		}
		walkMaps(root, func(m map[string]any) {
			if candidate := firstString(m, "playlistId"); strings.HasPrefix(candidate, "OLAK5uy_") {
				targetID = candidate
			}
		})
		if targetID == albumID {
			return fmt.Errorf("%w: album audio playlist", provider.ErrNotFound)
		}
	}
	path := "like/removelike"
	if saved {
		path = "like/like"
	}
	_, err := c.do(ctx, "WEB_REMIX", path, map[string]any{"target": map[string]any{"playlistId": targetID}}, true)
	return err
}

// CreatePlaylist creates a private playlist and returns its raw playlist ID.
func (c *Client) CreatePlaylist(ctx context.Context, title string) (string, error) {
	if err := c.requireAccount(); err != nil {
		return "", err
	}
	root, err := c.do(ctx, "WEB_REMIX", "playlist/create", map[string]any{"title": title, "privacyStatus": "PRIVATE"}, true)
	if err != nil {
		return "", err
	}
	id := findString(root, "playlistId")
	if id == "" {
		return "", errors.New("create_playlist: no playlistId in response")
	}
	return id, nil
}

// EditPlaylist updates only the supplied playlist details.
func (c *Client) EditPlaylist(ctx context.Context, id string, name, description *string, public *bool) error {
	var actions []any
	if name != nil {
		actions = append(actions, map[string]any{"action": "ACTION_SET_PLAYLIST_NAME", "playlistName": *name})
	}
	if description != nil {
		actions = append(actions, map[string]any{"action": "ACTION_SET_PLAYLIST_DESCRIPTION", "playlistDescription": *description})
	}
	if public != nil {
		privacy := "PRIVATE"
		if *public {
			privacy = "PUBLIC"
		}
		actions = append(actions, map[string]any{"action": "ACTION_SET_PLAYLIST_PRIVACY", "playlistPrivacy": privacy})
	}
	if len(actions) == 0 {
		return nil
	}
	_, err := c.editPlaylist(ctx, id, actions)
	return err
}

// DeletePlaylist deletes an owned playlist.
func (c *Client) DeletePlaylist(ctx context.Context, id string) error {
	if err := c.requireAccount(); err != nil {
		return err
	}
	_, err := c.do(ctx, "WEB_REMIX", "playlist/delete", map[string]any{"playlistId": strings.TrimPrefix(id, "VL")}, true)
	return err
}

// AddToPlaylist adds a track and maps YouTube's successful duplicate rejection.
func (c *Client) AddToPlaylist(ctx context.Context, playlistID, trackID string) error {
	_, err := c.editPlaylist(ctx, playlistID, []any{map[string]any{"action": "ACTION_ADD_VIDEO", "addedVideoId": trackID}})
	return err
}

// RemoveFromPlaylist removes a concrete playlist row.
func (c *Client) RemoveFromPlaylist(ctx context.Context, playlistID, trackID, setVideoID string) error {
	_, err := c.editPlaylist(ctx, playlistID, []any{map[string]any{"action": "ACTION_REMOVE_VIDEO", "setVideoId": setVideoID, "removedVideoId": trackID}})
	return err
}

// SetPlaylistSort persists an owned playlist's sort order.
func (c *Client) SetPlaylistSort(ctx context.Context, playlistID string, sort model.PlaylistSort) error {
	action := map[string]any{"action": "ACTION_SET_PLAYLIST_VIDEO_ORDER", "playlistVideoOrder": 0}
	switch sort {
	case model.PlaylistSortNewest:
		action["playlistVideoOrder"] = 1
	case model.PlaylistSortOldest:
		action["playlistVideoOrder"] = 2
	case model.PlaylistSortTop:
		action["playlistVideoOrder"] = 6
	case model.PlaylistSortTitle:
		action = map[string]any{"action": "ACTION_SET_PLAYLIST_DYNAMIC_SORT_PREFERENCE", "playlistDynamicSortPreference": 1}
	case model.PlaylistSortArtist:
		action = map[string]any{"action": "ACTION_SET_PLAYLIST_DYNAMIC_SORT_PREFERENCE", "playlistDynamicSortPreference": 2}
	case model.PlaylistSortAlbum:
		action = map[string]any{"action": "ACTION_SET_PLAYLIST_DYNAMIC_SORT_PREFERENCE", "playlistDynamicSortPreference": 3}
	}
	_, err := c.editPlaylist(ctx, playlistID, []any{action})
	return err
}

func (c *Client) editPlaylist(ctx context.Context, playlistID string, actions []any) (map[string]any, error) {
	if err := c.requireAccount(); err != nil {
		return nil, err
	}
	root, err := c.do(ctx, "WEB_REMIX", "browse/edit_playlist", map[string]any{"playlistId": strings.TrimPrefix(playlistID, "VL"), "actions": actions}, true)
	if err != nil {
		return nil, err
	}
	if firstString(root, "status") == "STATUS_FAILED" || findString(root, "status") == "STATUS_FAILED" {
		if findString(root, "dedupeOption") != "" {
			return nil, errors.New("This track is already in the playlist.")
		}
		return nil, fmt.Errorf("%w: playlist edit", provider.ErrUnavailable)
	}
	return root, nil
}

// SetArtistSubscribed subscribes or unsubscribes an artist channel.
func (c *Client) SetArtistSubscribed(ctx context.Context, channelID string, subscribed bool) error {
	if err := c.requireAccount(); err != nil {
		return err
	}
	path := "subscription/unsubscribe"
	if subscribed {
		path = "subscription/subscribe"
	}
	_, err := c.do(ctx, "WEB_REMIX", path, map[string]any{"channelIds": []string{channelID}}, true)
	return err
}

// SetPlaylistCover uploads and attaches a custom playlist thumbnail.
func (c *Client) SetPlaylistCover(ctx context.Context, playlistID string, image []byte) error {
	if err := c.requireAccount(); err != nil {
		return err
	}
	uploadID, err := c.openArtworkUpload(ctx, len(image))
	if err != nil {
		return err
	}
	blob, err := c.finishArtworkUpload(ctx, uploadID, image)
	if err != nil {
		return err
	}
	action := map[string]any{
		"action": "ACTION_SET_CUSTOM_THUMBNAIL",
		"addedCustomThumbnail": map[string]any{
			"imageKey":                      map[string]any{"name": "studio_square_thumbnail", "type": "PLAYLIST_IMAGE_TYPE_CUSTOM_THUMBNAIL"},
			"playlistScottyEncryptedBlobId": blob,
		},
	}
	if _, err := c.editPlaylist(ctx, playlistID, []any{action}); err != nil {
		return errors.New("YouTube Music only allows custom playlist art on accounts with a verified phone number.")
	}
	return nil
}

// ClearPlaylistCover removes a custom thumbnail and returns the rebuilt cover when supplied.
func (c *Client) ClearPlaylistCover(ctx context.Context, playlistID string) (*string, error) {
	if err := c.requireAccount(); err != nil {
		return nil, err
	}
	action := map[string]any{"action": "ACTION_REMOVE_CUSTOM_THUMBNAIL", "deletedCustomThumbnail": map[string]any{"name": "studio_square_thumbnail", "type": "PLAYLIST_IMAGE_TYPE_CUSTOM_THUMBNAIL"}}
	root, err := c.editPlaylist(ctx, playlistID, []any{action})
	if err != nil {
		return nil, errors.New("YouTube Music only allows custom playlist art on accounts with a verified phone number.")
	}
	cover := findThumbnail(root["newHeader"])
	if cover == "" {
		return nil, nil
	}
	return &cover, nil
}

func (c *Client) uploadRequest(ctx context.Context, target string, headers map[string]string, body []byte) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	c.mu.RLock()
	visitor, cookie := c.visitor, c.cookie
	c.mu.RUnlock()
	cfg := clients["WEB_REMIX"]
	setHeaders(req, cfg, visitor)
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
		if sapisid := sapisidCookie(cookie); sapisid != "" {
			req.Header.Set("Authorization", SAPISIDHash(timeNow(), sapisid))
		}
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	data, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return resp, nil, readErr
	}
	if resp.StatusCode/100 != 2 {
		return resp, data, fmt.Errorf("youtube artwork: %s", resp.Status)
	}
	return resp, data, nil
}

var timeNow = func() time.Time { return time.Now() }

func (c *Client) openArtworkUpload(ctx context.Context, size int) (string, error) {
	resp, _, err := c.uploadRequest(ctx, musicOrigin+"/playlist_image_upload/playlist_custom_thumbnail", map[string]string{
		"x-goog-upload-command":               "start",
		"x-goog-upload-protocol":              "resumable",
		"x-goog-upload-header-content-length": strconv.Itoa(size),
	}, nil)
	if err != nil {
		return "", err
	}
	id := resp.Header.Get("x-guploader-uploadid")
	if id == "" {
		return "", errors.New("YouTube didn't open an upload for the artwork.")
	}
	return id, nil
}

func (c *Client) finishArtworkUpload(ctx context.Context, uploadID string, image []byte) (string, error) {
	target := musicOrigin + "/playlist_image_upload/playlist_custom_thumbnail?upload_id=" + url.QueryEscape(uploadID) + "&upload_protocol=resumable"
	_, data, err := c.uploadRequest(ctx, target, map[string]string{"x-goog-upload-command": "upload, finalize", "x-goog-upload-offset": "0"}, image)
	if err != nil {
		return "", err
	}
	var root map[string]any
	if json.Unmarshal(data, &root) != nil {
		return "", errors.New("YouTube rejected the artwork upload.")
	}
	blob := findString(root, "encryptedBlobId")
	if blob == "" {
		return "", errors.New("YouTube rejected the artwork upload.")
	}
	return blob, nil
}
