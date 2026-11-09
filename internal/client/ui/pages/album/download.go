package album

import (
	"fmt"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (p *Page) confirmDownload(e *env.Env) {
	if len(p.page.Items) == 0 {
		return
	}
	e.Confirm(
		"Download album?",
		"This queues every available track in "+p.Title()+". Tracks already saved are left alone.",
		"Download",
		"",
		func() {
			e.Toast("Preparing "+p.Title()+" downloads…", env.ToastInfo)
			p.collectDownloads(e, append([]model.Track(nil), p.page.Items...), p.page.Continuation, make(map[string]struct{}))
		},
	)
}

func (p *Page) collectDownloads(e *env.Env, items []model.Track, token *string, seen map[string]struct{}) {
	if token == nil || *token == "" {
		p.enqueueDownloads(e, items)
		return
	}
	if _, repeated := seen[*token]; repeated {
		e.Toast("Pagination repeated; no downloads were added.", env.ToastError)
		return
	}
	seen[*token] = struct{}{}
	act.Call[model.PlaylistTracksPage](e, protocol.MethodGetPlaylistMore, protocol.TokenParams{Token: *token}, func(more model.PlaylistTracksPage, err error) {
		if err != nil {
			e.Toast("Could not download "+p.Title(), env.ToastError)
			return
		}
		items = append(items, more.Items...)
		p.collectDownloads(e, items, more.Continuation, seen)
	})
}

func (p *Page) enqueueDownloads(e *env.Env, items []model.Track) {
	collection := p.Title()
	kind := "album"
	entries := make([]model.DownloadCollectionEntry, 0, len(items))
	for _, track := range items {
		entries = append(entries, model.DownloadCollectionEntry{
			VideoID:        track.VideoID,
			Title:          track.Title,
			Artists:        track.Artists,
			Thumbnail:      value(track.Thumbnail),
			Collection:     &collection,
			CollectionKind: &kind,
		})
	}
	act.Call[model.DownloadCollectionResult](e, protocol.MethodEnqueueCollection, protocol.EnqueueCollectionParams{Entries: entries}, func(result model.DownloadCollectionResult, err error) {
		if err != nil {
			e.Toast("Could not download "+p.Title(), env.ToastError)
			return
		}
		if result.Added == 0 {
			e.Toast("Nothing new to download", env.ToastInfo)
			return
		}
		message := fmt.Sprintf("%s added · %d already saved · %d unavailable", trackCount(result.Added), result.AlreadyDownloaded, result.Skipped)
		e.Toast(message, env.ToastSuccess)
	})
}

func trackCount(n int) string {
	if n == 1 {
		return "1 track"
	}
	return fmt.Sprintf("%d tracks", n)
}
