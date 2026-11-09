package daemon

import (
	"context"
	"errors"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

func setArtistSubscribed(ctx context.Context, a *App, p protocol.SetArtistSubscribedParams) (protocol.SetArtistSubscribedResult, error) {
	id := strings.TrimSpace(p.ID)
	if id == "" {
		return protocol.SetArtistSubscribedResult{}, errors.New("Choose an artist to subscribe to.")
	}
	pr := providerForID(a, id)
	if pr.ID() != model.ProviderYouTube {
		return protocol.SetArtistSubscribedResult{}, provider.ErrUnsupported
	}
	subscriber, ok := pr.(provider.ArtistSubscriber)
	if !ok {
		return protocol.SetArtistSubscribedResult{}, provider.ErrUnsupported
	}
	if err := subscriber.SetArtistSubscribed(ctx, id, p.Subscribed); err != nil {
		return protocol.SetArtistSubscribedResult{}, providerError(pr.ID(), err)
	}
	return protocol.SetArtistSubscribedResult{Subscribed: p.Subscribed}, nil
}
