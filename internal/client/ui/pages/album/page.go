// Package album implements the retained album detail page.
package album

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// Page is a retained album detail surface.
type Page struct {
	route  env.Route
	hero   widgets.Hero
	scroll look.Scroller
	table  widgets.TrackTable

	play      kit.Plaque
	shuffle   kit.IconPlaque
	save      kit.IconPlaque
	download  kit.IconPlaque
	more      kit.IconPlaque
	retry     kit.Plaque
	moreRetry kit.Plaque
	menu      widgets.ItemMenu

	page        model.AlbumPage
	loaded      bool
	loading     bool
	loadingMore bool
	errText     string
	moreError   string
	request     uint64
	seenTokens  map[string]struct{}
	libraryRev  uint64
	saved       bool
	savePending bool
	meta        []kit.Span
	trackIDs    map[string]struct{}
}

// New constructs an album detail page for r.ID.
func New(r env.Route) env.Page {
	return &Page{route: r, seenTokens: make(map[string]struct{}), trackIDs: make(map[string]struct{})}
}

// Title returns the loaded album title.
func (p *Page) Title() string {
	if title := value(p.page.Title); title != "" {
		return title
	}
	if p.route.Title != "" {
		return p.route.Title
	}
	return "Album"
}

// Refresh reloads the album and its saved-library state.
func (p *Page) Refresh(e *env.Env) {
	p.loaded = true
	p.loading = true
	p.errText = ""
	p.moreError = ""
	p.request++
	p.seenTokens = make(map[string]struct{})
	p.libraryRev = e.Store.Snapshot().LibraryRevision
	req := p.request
	act.Call[model.AlbumPage](e, protocol.MethodGetAlbum, protocol.IDParams{ID: p.route.ID}, func(page model.AlbumPage, err error) {
		if req != p.request {
			return
		}
		p.loading = false
		if err != nil {
			p.errText = err.Error()
			return
		}
		p.page = page
		p.refreshDerived()
	})
	if !albumCanSave(p.route.ID) {
		return
	}
	act.Call[[]model.BrowseItem](e, protocol.MethodGetLibraryAlbums, protocol.Null{}, func(items []model.BrowseItem, err error) {
		if req != p.request || err != nil {
			return
		}
		p.saved = false
		for _, item := range items {
			if item.ID == p.route.ID {
				p.saved = true
				break
			}
		}
	})
}

// Layout renders one virtualised scroller containing the hero and table.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	state := e.Store.Snapshot()
	if !p.loaded || p.libraryRev != state.LibraryRevision && !p.loading && !p.savePending {
		p.Refresh(e)
	}
	p.handleActions(gtx, e)
	if p.loading && p.page.Title == nil {
		return widgets.Loading(e, widgets.LoadingHero)(gtx)
	}
	if p.errText != "" && p.page.Title == nil {
		return widgets.Failed(e, p.errText, &p.retry)(gtx)
	}

	items := p.page.Items
	if len(items) == 0 {
		return p.scroll.Layout(gtx, e.Th, 2, func(gtx layout.Context, index int) layout.Dimensions {
			if index == 0 {
				return p.layoutHero(gtx, e)
			}
			return p.inset(e, 0, 34, widgets.EmptyState(e, look.Icon.Album, "No tracks on this album", "This album does not have any playable tracks."))(gtx)
		})
	}

	opts := widgets.TrackTableOpts{
		Columns:      widgets.ColArtist | widgets.ColPlays,
		Context:      "album",
		SourceID:     p.route.ID,
		SourceName:   p.Title(),
		SourceItems:  p.page.Items,
		DisplayItems: items,
		Continuation: p.page.Continuation,
		Number:       true,
	}
	count := len(items) + 3
	return p.scroll.Layout(gtx, e.Th, count, func(gtx layout.Context, index int) layout.Dimensions {
		switch {
		case index == 0:
			return p.layoutHero(gtx, e)
		case index == 1:
			return p.inset(e, 0, 0, func(gtx layout.Context) layout.Dimensions {
				return p.table.Header(gtx, e, opts)
			})(gtx)
		case index < len(items)+2:
			row := index - 2
			return p.inset(e, 0, 0, func(gtx layout.Context) layout.Dimensions {
				return p.table.Row(gtx, e, items, row, opts)
			})(gtx)
		default:
			return p.inset(e, 13, 34, p.layoutFooter(e))(gtx)
		}
	})
}

func (p *Page) inset(e *env.Env, top, bottom unit.Dp, body layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		inset := look.ContentInset(e.Th.Class)
		return layout.Inset{Left: inset, Right: inset, Top: top, Bottom: bottom}.Layout(gtx, body)
	}
}

func (p *Page) layoutHero(gtx layout.Context, e *env.Env) layout.Dimensions {
	artistID, artistName := albumArtist(p.page)
	var openArtist func()
	if artistID != "" {
		openArtist = func() { act.OpenArtist(e, artistID, artistName) }
	}
	return p.hero.Layout(gtx, e, widgets.HeroContent{
		Art:        value(p.page.Thumbnail),
		Kind:       albumKind(p.page),
		Title:      p.Title(),
		Meta:       p.meta,
		MetaAction: openArtist,
		Acts:       p.layoutActs(e),
	})
}

func (p *Page) handleActions(gtx layout.Context, e *env.Env) {
	if p.retry.Clicked(gtx) {
		p.Refresh(e)
	}
	if p.moreRetry.Clicked(gtx) {
		p.loadMore(e)
	}
	if p.play.Clicked(gtx) && len(p.page.Items) > 0 {
		if p.collectionPlaying(e) {
			act.Call[protocol.Null](e, protocol.MethodTogglePause, protocol.Null{}, func(_ protocol.Null, err error) {
				if err != nil {
					e.Toast(err.Error(), env.ToastError)
				}
			})
		} else {
			act.PlayCollection(e, p.page.Items, 0, p.route.ID, p.Title(), false, p.page.Continuation)
		}
	}
	if p.shuffle.Clicked(gtx) && len(p.page.Items) > 0 {
		act.PlayCollection(e, p.page.Items, 0, p.route.ID, p.Title(), true, p.page.Continuation)
	}
	if p.download.Clicked(gtx) && model.ProviderOf(p.route.ID) != model.ProviderLocal {
		p.confirmDownload(e)
	}
	if p.save.Clicked(gtx) && !p.savePending {
		if !albumSaveSignedIn(e, p.route.ID) {
			e.Toast("Sign in to save albums to your library", env.ToastInfo)
		} else {
			p.setSaved(e, !p.saved)
		}
	}
	if p.more.Clicked(gtx) {
		card := model.BrowseItem{Kind: "album", ID: p.route.ID, Title: p.Title(), Thumbnail: p.page.Thumbnail}
		if _, artist := albumArtist(p.page); artist != "" {
			card.Subtitle = &artist
		}
		if e.Overlays != nil {
			p.menu.OpenAt(e, env.MenuTarget{Card: &card}, e.Overlays.Press())
		}
	}
}

func (p *Page) collectionPlaying(e *env.Env) bool {
	state := e.Store.Snapshot()
	return !state.Playback.Paused && state.Playback.Now != nil && state.Queue.SourceName != nil && *state.Queue.SourceName == p.Title()
}

func (p *Page) layoutActs(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		playing := p.collectionPlaying(e)
		p.play.Kind, p.play.Icon, p.play.Label = kit.PlaqueGold, look.Icon.Play, "Play"
		if playing {
			p.play.Icon, p.play.Label = look.Icon.Pause, "Pause"
		}
		p.shuffle.Kind, p.shuffle.Icon, p.shuffle.Description = kit.PlaqueQuiet, look.Icon.Shuffle, "Shuffle album"
		p.save.Kind, p.save.Icon, p.save.Description = kit.PlaqueQuiet, look.Icon.HeartOutline, "Save album"
		if p.saved {
			p.save.Icon, p.save.Description = look.Icon.Heart, "Remove album from library"
		}
		p.download.Kind, p.download.Icon, p.download.Description = kit.PlaqueQuiet, look.Icon.Download, "Download album"
		active, progress, current, complete := collectionDownload(e, p.Title(), p.trackIDs, len(p.page.Items))
		if active {
			p.download.Notch = e.Th.R.Lapis
		} else {
			p.download.Notch = color.NRGBA{}
		}
		if active {
			p.download.Description = fmt.Sprintf("Downloading %d of %d", current, len(p.page.Items))
		} else if complete {
			p.download.Icon, p.download.Description = look.Icon.DownloadDone, "Saved"
		}
		p.more.Kind, p.more.Icon, p.more.Description = kit.PlaqueQuiet, look.Icon.More, "More album actions"
		play := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(p.page.Items) == 0 {
				gtx = gtx.Disabled()
			}
			return p.play.Layout(gtx, e.Th.Theme)
		})
		gap := layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout)
		shuffle := layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.shuffle.Layout(gtx, e.Th.Theme) })
		save := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.savePending {
				gtx = gtx.Disabled()
			}
			return p.save.Layout(gtx, e.Th.Theme)
		})
		download := layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutDownload(gtx, e, active, progress) })
		more := layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.more.Layout(gtx, e.Th.Theme) })
		canSave := albumCanSave(p.route.ID)
		canDownload := model.ProviderOf(p.route.ID) != model.ProviderLocal
		switch {
		case canSave && canDownload:
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, play, gap, shuffle, gap, save, gap, download, gap, more)
		case canSave:
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, play, gap, shuffle, gap, save, gap, more)
		case canDownload:
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, play, gap, shuffle, gap, download, gap, more)
		default:
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, play, gap, shuffle, gap, more)
		}
	}
}

func (p *Page) layoutDownload(gtx layout.Context, e *env.Env, active bool, progress int) layout.Dimensions {
	dims := p.download.Layout(gtx, e.Th.Theme)
	if !active {
		return dims
	}
	height := max(1, gtx.Dp(2))
	cut := gtx.Dp(e.Th.Cut.Small)
	width := max(1, dims.Size.X-cut*2)
	progressGTX := gtx
	progressGTX.Constraints = layout.Exact(image.Pt(width, height))
	offset := op.Offset(image.Pt(cut, dims.Size.Y-height)).Push(gtx.Ops)
	kit.BladeProgress{Fraction: float64(progress) / 100}.Layout(progressGTX, e.Th.Theme)
	offset.Pop()
	return dims
}

func (p *Page) setSaved(e *env.Env, saved bool) {
	p.savePending = true
	act.Call[protocol.Null](e, protocol.MethodSetAlbumSaved, protocol.SetAlbumSavedParams{PlaylistID: p.route.ID, Saved: saved}, func(_ protocol.Null, err error) {
		p.savePending = false
		if err != nil {
			e.Toast(err.Error(), env.ToastError)
			return
		}
		p.saved = saved
		if saved {
			e.Toast("Saved to library", env.ToastSuccess)
		} else {
			e.Toast("Removed from library", env.ToastSuccess)
		}
	})
}

func (p *Page) loadMore(e *env.Env) {
	if p.loadingMore || p.page.Continuation == nil || *p.page.Continuation == "" {
		return
	}
	token := *p.page.Continuation
	if _, repeated := p.seenTokens[token]; repeated {
		p.moreError = "Pagination repeated."
		return
	}
	p.seenTokens[token] = struct{}{}
	p.loadingMore = true
	p.moreError = ""
	act.Call[model.PlaylistTracksPage](e, protocol.MethodGetPlaylistMore, protocol.TokenParams{Token: token}, func(more model.PlaylistTracksPage, err error) {
		p.loadingMore = false
		if err != nil {
			delete(p.seenTokens, token)
			p.moreError = err.Error()
			return
		}
		p.page.Items = append(p.page.Items, more.Items...)
		p.page.Continuation = more.Continuation
		p.refreshDerived()
	})
}

func (p *Page) layoutFooter(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if p.loadingMore {
			return kit.Text(e.Th.Theme, e.Th.T.Body, e.Th.R.Fg2, "Loading more…")(gtx)
		}
		if p.moreError != "" {
			p.moreRetry.Kind, p.moreRetry.Label = kit.PlaqueQuiet, "Try again"
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, kit.Text(e.Th.Theme, e.Th.T.Body, e.Th.R.Jasper, p.moreError)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.moreRetry.Layout(gtx, e.Th.Theme) }),
			)
		}
		if p.page.Continuation != nil && *p.page.Continuation != "" {
			p.loadMore(e)
			return kit.Text(e.Th.Theme, e.Th.T.Body, e.Th.R.Fg2, "Loading more…")(gtx)
		}
		return layout.Dimensions{}
	}
}

func (p *Page) refreshDerived() {
	p.meta = albumMeta(p.page)
	clear(p.trackIDs)
	for _, track := range p.page.Items {
		p.trackIDs[track.VideoID] = struct{}{}
	}
}

func albumMeta(page model.AlbumPage) []kit.Span {
	meta := make([]kit.Span, 0, 9)
	_, artist := albumArtist(page)
	appendMeta := func(text string, mono bool) {
		if strings.TrimSpace(text) == "" {
			return
		}
		if len(meta) > 0 {
			meta = append(meta, kit.Span{Text: " · "})
		}
		meta = append(meta, kit.Span{Text: text, Mono: mono})
	}
	appendMeta(artist, false)
	appendMeta(findYear(value(page.Subtitle)+" "+value(page.Description)), true)
	appendMeta(songCount(len(page.Items)), true)
	appendMeta(collectionDuration(totalDuration(page.Items)), true)
	return meta
}

func songCount(n int) string {
	if n == 1 {
		return "1 song"
	}
	return fmt.Sprintf("%d songs", n)
}

func collectionDuration(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	minutes := (seconds + 30) / 60
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	hours := minutes / 60
	minutes %= 60
	if minutes == 0 {
		return fmt.Sprintf("%d hr", hours)
	}
	return fmt.Sprintf("%d hr %d min", hours, minutes)
}

func albumKind(page model.AlbumPage) string {
	text := strings.ToLower(value(page.Subtitle) + " " + value(page.Description))
	if strings.Contains(text, " ep") || strings.HasPrefix(text, "ep") {
		return "EP"
	}
	return "Album"
}

func albumArtist(page model.AlbumPage) (string, string) {
	for _, track := range page.Items {
		if track.ArtistID != nil && *track.ArtistID != "" {
			return *track.ArtistID, track.Artists
		}
		for _, run := range track.ArtistRuns {
			if run.ID != nil && *run.ID != "" {
				return *run.ID, run.Text
			}
		}
	}
	return "", ""
}

func findYear(value string) string {
	for _, field := range strings.FieldsFunc(value, func(r rune) bool { return r < '0' || r > '9' }) {
		if len(field) != 4 {
			continue
		}
		if year, err := strconv.Atoi(field); err == nil && year >= 1900 && year <= 2200 {
			return field
		}
	}
	return ""
}

func totalDuration(items []model.Track) int {
	total := 0
	for _, item := range items {
		if item.Duration == nil {
			continue
		}
		seconds := 0
		valid := true
		for _, part := range strings.Split(*item.Duration, ":") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				valid = false
				break
			}
			seconds = seconds*60 + n
		}
		if valid {
			total += seconds
		}
	}
	return total
}

func collectionDownload(e *env.Env, title string, ids map[string]struct{}, itemCount int) (active bool, progress, current int, complete bool) {
	matched, completed, totalProgress := 0, 0, 0
	for _, job := range e.Store.Snapshot().Downloads.Jobs {
		_, trackMatch := ids[job.VideoID]
		collectionMatch := job.Collection != nil && *job.Collection == title
		if !trackMatch && !collectionMatch {
			continue
		}
		switch job.Status {
		case model.DownloadStatusQueued:
			active = true
			matched++
		case model.DownloadStatusDownloading:
			active = true
			matched++
			current++
			totalProgress += min(max(job.Progress, 0), 100)
		case model.DownloadStatusCompleted:
			matched++
			completed++
			current++
			totalProgress += 100
		}
	}
	if itemCount > 0 {
		progress = totalProgress / itemCount
		current = min(current, itemCount)
	} else if matched > 0 {
		progress = totalProgress / matched
	}
	complete = itemCount > 0 && completed >= itemCount
	return active, progress, current, complete
}

func value(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func albumCanSave(id string) bool {
	provider := model.ProviderOf(id)
	return provider == model.ProviderYouTube || provider == model.ProviderSpotify
}

func albumSaveSignedIn(e *env.Env, id string) bool {
	state := e.Store.Snapshot()
	if model.ProviderOf(id) == model.ProviderSpotify {
		return state.Spotify.SignedIn
	}
	return state.Auth.SignedIn
}
