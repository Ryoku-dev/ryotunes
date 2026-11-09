package spotify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	playlist4pb "github.com/devgianlu/go-librespot/proto/spotify/playlist4"
	"github.com/devgianlu/go-librespot/spclient"
	"google.golang.org/protobuf/proto"
)

// CreatePlaylist implements provider.PlaylistEditor.
func (p *Spotify) CreatePlaylist(ctx context.Context, title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", fmt.Errorf("playlist title is empty")
	}
	sess, err := p.session(ctx)
	if err != nil {
		return "", err
	}
	body := playlistChanges(nil, sess.Username(), playlistAttributesOp(&playlist4pb.ListAttributes{Name: new(title)}))
	resp, err := postProto(ctx, sess.Spclient(), "/playlist/v2/playlist", body)
	if err != nil {
		return "", mapProviderError(err)
	}
	var reply playlist4pb.CreateListReply
	if err := proto.Unmarshal(resp, &reply); err != nil {
		return "", err
	}
	id := strings.TrimPrefix(reply.GetUri(), "spotify:playlist:")
	if id == "" || id == reply.GetUri() {
		return "", fmt.Errorf("cannot read the created playlist id")
	}
	if err := p.addPlaylistToRootlist(ctx, sess.Spclient(), sess.Username(), "spotify:playlist:"+id); err != nil {
		return "", err
	}
	return "spotify:playlist:" + id, nil
}

// EditPlaylist implements provider.PlaylistEditor.
func (p *Spotify) EditPlaylist(ctx context.Context, id string, name, description *string, public *bool) error {
	bare, err := bareID(id, "playlist")
	if err != nil {
		return err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return err
	}
	if name != nil || description != nil {
		values := &playlist4pb.ListAttributes{Name: name, Description: description}
		body := playlistChanges(nil, sess.Username(), playlistAttributesOp(values))
		if _, err := postProto(ctx, sess.Spclient(), "/playlist/v2/playlist/"+bare+"/changes", body); err != nil {
			return mapProviderError(err)
		}
	}
	if public != nil {
		return p.setPlaylistVisibility(ctx, sess.Spclient(), sess.Username(), "spotify:playlist:"+bare, *public)
	}
	return nil
}

// DeletePlaylist implements provider.PlaylistEditor.
func (p *Spotify) DeletePlaylist(ctx context.Context, id string) error {
	bare, err := bareID(id, "playlist")
	if err != nil {
		return err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return err
	}
	uri := "spotify:playlist:" + bare
	if err := p.removePlaylistFromRootlist(ctx, sess.Spclient(), sess.Username(), uri); err != nil {
		return err
	}
	values := &playlist4pb.ListAttributes{DeletedByOwner: new(true)}
	_, err = postProto(ctx, sess.Spclient(), "/playlist/v2/playlist/"+bare+"/changes", playlistChanges(nil, sess.Username(), playlistAttributesOp(values)))
	return mapProviderError(err)
}

// AddToPlaylist implements provider.PlaylistEditor.
func (p *Spotify) AddToPlaylist(ctx context.Context, playlistID, trackID string) error {
	playlistBare, err := bareID(playlistID, "playlist")
	if err != nil {
		return err
	}
	trackBare, err := bareID(trackID, "track")
	if err != nil {
		return err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return err
	}
	playlist, _ := librespot.SpotifyIdFromBase62(librespot.SpotifyIdTypePlaylist, playlistBare)
	for range 2 {
		revision, err := sess.Spclient().PlaylistRevision(ctx, *playlist)
		if err != nil {
			return mapProviderError(err)
		}
		err = sess.Spclient().PlaylistAppend(ctx, *playlist, sess.Username(), revision, []string{"spotify:track:" + trackBare})
		if err == nil {
			return nil
		}
		if !errors.Is(err, spclient.ErrPlaylistConflict) {
			return mapProviderError(err)
		}
	}
	return spclient.ErrPlaylistConflict
}

// RemoveFromPlaylist implements provider.PlaylistEditor.
func (p *Spotify) RemoveFromPlaylist(ctx context.Context, playlistID, trackID, _ string) error {
	playlistBare, err := bareID(playlistID, "playlist")
	if err != nil {
		return err
	}
	trackBare, err := bareID(trackID, "track")
	if err != nil {
		return err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return err
	}
	playlist, _ := librespot.SpotifyIdFromBase62(librespot.SpotifyIdTypePlaylist, playlistBare)
	uri := "spotify:track:" + trackBare
	for range 2 {
		revision, items, err := sess.Spclient().PlaylistContents(ctx, *playlist)
		if err != nil {
			return mapProviderError(err)
		}
		index := -1
		for i, item := range items {
			if item == uri {
				index = i
				break
			}
		}
		if index < 0 {
			return nil
		}
		err = sess.Spclient().PlaylistRemove(ctx, *playlist, sess.Username(), revision, uri, index)
		if err == nil {
			return nil
		}
		if !errors.Is(err, spclient.ErrPlaylistConflict) {
			return mapProviderError(err)
		}
	}
	return spclient.ErrPlaylistConflict
}

func playlistChanges(revision []byte, username string, ops ...*playlist4pb.Op) *playlist4pb.ListChanges {
	return &playlist4pb.ListChanges{BaseRevision: revision, Deltas: []*playlist4pb.Delta{{Ops: ops, Info: &playlist4pb.ChangeInfo{User: new(username), Timestamp: new(time.Now().UnixMilli())}}}, WantResultingRevisions: new(true)}
}
func playlistAttributesOp(values *playlist4pb.ListAttributes) *playlist4pb.Op {
	return &playlist4pb.Op{Kind: playlist4pb.Op_UPDATE_LIST_ATTRIBUTES.Enum(), UpdateListAttributes: &playlist4pb.UpdateListAttributes{NewAttributes: &playlist4pb.ListAttributesPartialState{Values: values}}}
}
func addItemOp(uri string) *playlist4pb.Op {
	return &playlist4pb.Op{Kind: playlist4pb.Op_ADD.Enum(), Add: &playlist4pb.Add{Items: []*playlist4pb.Item{{Uri: new(uri)}}, AddLast: new(true)}}
}
func removeItemOp(uri string, index int) *playlist4pb.Op {
	return &playlist4pb.Op{Kind: playlist4pb.Op_REM.Enum(), Rem: &playlist4pb.Rem{FromIndex: new(int32(index)), Length: new(int32(1)), Items: []*playlist4pb.Item{{Uri: new(uri)}}, ItemsAsKey: new(true)}}
}

func postProto(ctx context.Context, sp *spclient.Spclient, path string, message proto.Message) ([]byte, error) {
	body, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}
	resp, err := sp.RequestOnce(ctx, http.MethodPost, path, nil, nil, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Spotify playlist service status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

func (p *Spotify) addPlaylistToRootlist(ctx context.Context, sp *spclient.Spclient, username, uri string) error {
	root, err := sp.Rootlist(ctx, username, 0, 10000)
	if err != nil {
		return err
	}
	for _, item := range root.GetContents().GetItems() {
		if item.GetUri() == uri {
			return nil
		}
	}
	_, err = postProto(ctx, sp, "/playlist/v2/user/"+url.PathEscape(username)+"/rootlist/changes", playlistChanges(root.GetRevision(), username, addItemOp(uri)))
	return mapProviderError(err)
}
func (p *Spotify) removePlaylistFromRootlist(ctx context.Context, sp *spclient.Spclient, username, uri string) error {
	root, err := sp.Rootlist(ctx, username, 0, 10000)
	if err != nil {
		return err
	}
	index := -1
	for i, item := range root.GetContents().GetItems() {
		if item.GetUri() == uri {
			index = i
			break
		}
	}
	if index < 0 {
		return nil
	}
	_, err = postProto(ctx, sp, "/playlist/v2/user/"+url.PathEscape(username)+"/rootlist/changes", playlistChanges(root.GetRevision(), username, removeItemOp(uri, index)))
	return mapProviderError(err)
}
func (p *Spotify) setPlaylistVisibility(ctx context.Context, sp *spclient.Spclient, username, uri string, public bool) error {
	root, err := sp.Rootlist(ctx, username, 0, 10000)
	if err != nil {
		return err
	}
	index := -1
	for i, item := range root.GetContents().GetItems() {
		if item.GetUri() == uri {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("playlist is not in the library")
	}
	op := &playlist4pb.Op{Kind: playlist4pb.Op_UPDATE_ITEM_ATTRIBUTES.Enum(), UpdateItemAttributes: &playlist4pb.UpdateItemAttributes{Index: new(int32(index)), NewAttributes: &playlist4pb.ItemAttributesPartialState{Values: &playlist4pb.ItemAttributes{Public: new(public)}}}}
	_, err = postProto(ctx, sp, "/playlist/v2/user/"+url.PathEscape(username)+"/rootlist/changes", playlistChanges(root.GetRevision(), username, op))
	return mapProviderError(err)
}
