package spotify

import (
	"context"

	"github.com/devgianlu/go-librespot/spclient"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Rate implements provider.Rater by updating Spotify Liked Songs.
func (p *Spotify) Rate(ctx context.Context, trackID string, rating model.Rating) error {
	bare, err := bareID(trackID, "track")
	if err != nil {
		return err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return err
	}
	remove := rating != model.RatingLike
	return mapProviderError(sess.Spclient().CollectionWrite(ctx, sess.Username(), spclient.CollectionSetLikedSongs, []string{"spotify:track:" + bare}, remove))
}

// SetAlbumSaved implements provider.AlbumSaver.
func (p *Spotify) SetAlbumSaved(ctx context.Context, albumID string, saved bool) error {
	bare, err := bareID(albumID, "album")
	if err != nil {
		return err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return err
	}
	return mapProviderError(sess.Spclient().CollectionWrite(ctx, sess.Username(), spclient.CollectionSetLikedSongs, []string{"spotify:album:" + bare}, !saved))
}
