package daemon

import (
	"context"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func TestSetArtistSubscribedValidatesID(t *testing.T) {
	_, err := setArtistSubscribed(context.Background(), &App{}, protocol.SetArtistSubscribedParams{Subscribed: true})
	if err == nil {
		t.Fatal("empty artist id was accepted")
	}
}
