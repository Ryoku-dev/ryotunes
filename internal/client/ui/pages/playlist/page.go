// Package playlist implements the retained playlist detail page.
package playlist

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
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/widgets"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

var sortLabels = []string{"Default", "Newest first", "Oldest first", "Title", "Artist", "Album"}
var playlistMoreItems = []kit.MenuItem{
	{Icon: look.Icon.More, Label: "Playlist actions"},
	{Icon: look.Icon.Download, Label: "Export to file"},
}
var playlistMoreEditableItems = []kit.MenuItem{
	{Icon: look.Icon.More, Label: "Playlist actions"},
	{Icon: look.Icon.Download, Label: "Export to file"},
	{Icon: look.Icon.Settings, Label: "Edit playlist"},
}

var sortValues = []model.PlaylistSort{model.PlaylistSortDefault, model.PlaylistSortNewest, model.PlaylistSortOldest, model.PlaylistSortTitle, model.PlaylistSortArtist, model.PlaylistSortAlbum}

// Page is a retained playlist detail surface.
type Page struct {
	route       env.Route
	hero        widgets.Hero
	heroActions widgets.HeroActions
	scroll      look.Scroller
	heroHeight  int
	table       widgets.TrackTable
	sortChoice  look.Choice
	play        kit.Plaque
	shuffle     kit.IconPlaque
	save        kit.IconPlaque
	download    kit.IconPlaque
	more        kit.IconPlaque
	retry       kit.Plaque
	moreRetry   kit.Plaque

	moreMenu     kit.Menu
	moreTag      int
	itemMenu     widgets.ItemMenu
	editDialog   widgets.EditPlaylistDialog
	exportDialog exportDialog

	page        model.PlaylistPage
	loaded      bool
	loading     bool
	loadingMore bool
	sortPending bool
	errText     string
	moreError   string
	request     uint64
	seenTokens  map[string]struct{}
	libraryRev  uint64
	meta        []kit.Span
	trackIDs    map[string]struct{}
}

// New constructs a playlist detail page for r.ID.
func New(r env.Route) env.Page {
	return &Page{route: r, seenTokens: make(map[string]struct{}), trackIDs: make(map[string]struct{})}
}

// Title returns the loaded playlist title.
func (p *Page) Title() string {
	if title := value(p.page.Title); title != "" {
		return title
	}
	if p.route.Title != "" {
		return p.route.Title
	}
	return "Playlist"
}

// Refresh reloads playlist metadata and tracks.
func (p *Page) Refresh(e *env.Env) {
	p.loaded = true
	p.loading = true
	p.errText = ""
	p.moreError = ""
	p.request++
	p.seenTokens = make(map[string]struct{})
	p.libraryRev = e.Store.Snapshot().LibraryRevision
	req := p.request
	params := protocol.PlaylistParams{ID: p.route.ID}
	if selected := p.selectedSort(); selected != model.PlaylistSortDefault {
		params.Sort = &selected
	}
	act.Call[model.PlaylistPage](e, protocol.MethodGetPlaylist, params, func(page model.PlaylistPage, err error) {
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
}

// Layout renders one virtualised scroller containing the hero and table.
func (p *Page) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	stateSnapshot := e.Store.Snapshot()
	if !p.loaded {
		p.Refresh(e)
	} else if p.libraryRev != stateSnapshot.LibraryRevision {
		if stateSnapshot.ChangedLibraryID == "" || stateSnapshot.ChangedLibraryID == p.route.ID {
			if !p.loading {
				p.Refresh(e)
			}
		} else {
			p.libraryRev = stateSnapshot.LibraryRevision
		}
	}
	p.handleActions(gtx, e)
	if selected, ok := p.sortChoice.Changed(); ok && !p.sortPending && selected >= 0 && selected < len(sortValues) {
		p.setSort(e, sortValues[selected])
	}
	if p.loading && p.page.Title == nil {
		return widgets.Loading(e, widgets.LoadingHero)(gtx)
	}
	if p.errText != "" && p.page.Title == nil {
		return widgets.Failed(e, p.errText, &p.retry)(gtx)
	}

	items := p.page.Items
	sortInHero := gtx.Constraints.Max.X >= gtx.Dp(800) || len(items) == 0
	if len(items) == 0 {
		viewport := gtx.Constraints.Max.Y
		dims := p.scroll.Layout(gtx, e.Th, 2, func(gtx layout.Context, index int) layout.Dimensions {
			if index == 0 {
				dims := p.layoutHero(gtx, e, sortInHero)
				p.heroHeight = dims.Size.Y
				return dims
			}
			// Centred in the room under the hero, scrolling when there is none.
			gtx.Constraints.Min.Y = max(0, viewport-p.heroHeight)
			return widgets.EmptyState(e, look.Icon.Playlist, "No tracks in this playlist", "Add a song and it will appear here.")(gtx)
		})
		p.editDialog.Layout(gtx, e)
		p.exportDialog.Layout(gtx, e, p)
		return dims
	}

	opts := widgets.TrackTableOpts{
		Columns:      widgets.ColArt | widgets.ColAlbum,
		Context:      p.route.ID,
		SourceID:     p.route.ID,
		SourceName:   p.Title(),
		SourceItems:  p.page.Items,
		DisplayItems: items,
		Continuation: p.page.Continuation,
	}
	count := len(items) + 3
	dims := p.scroll.Layout(gtx, e.Th, count, func(gtx layout.Context, index int) layout.Dimensions {
		switch {
		case index == 0:
			return p.layoutHero(gtx, e, sortInHero)
		case index == 1:
			return p.inset(e, 0, 0, func(gtx layout.Context) layout.Dimensions {
				return p.layoutTableHeader(gtx, e, opts, sortInHero)
			})(gtx)
		case index < len(items)+2:
			row := index - 2
			return p.inset(e, 0, 0, func(gtx layout.Context) layout.Dimensions { return p.table.Row(gtx, e, items, row, opts) })(gtx)
		default:
			return p.inset(e, 13, 34, p.layoutFooter(e))(gtx)
		}
	})
	p.editDialog.Layout(gtx, e)
	p.exportDialog.Layout(gtx, e, p)
	return dims
}

func (p *Page) inset(e *env.Env, top, bottom unit.Dp, body layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		inset := look.ContentInset(e.Th.Class)
		return layout.Inset{Left: inset, Right: inset, Top: top, Bottom: bottom}.Layout(gtx, body)
	}
}

func (p *Page) layoutHero(gtx layout.Context, e *env.Env, sortInHero bool) layout.Dimensions {
	return p.hero.Layout(gtx, e, widgets.HeroContent{
		Art:     value(p.page.Thumbnail),
		Collage: strings.HasPrefix(value(p.page.Thumbnail), model.CollageThumbnailPrefix),
		Kind:    playlistKind(p.route.ID),
		Title:   p.Title(),
		Meta:    p.meta,
		Acts:    p.layoutActs(e, sortInHero),
	})
}

func (p *Page) sortAvailable() bool {
	return p.page.SortMenu != nil && p.page.SortMenu.Editable && !isDevice(p.route.ID)
}
func (p *Page) layoutTableHeader(gtx layout.Context, e *env.Env, opts widgets.TrackTableOpts, sortInHero bool) layout.Dimensions {
	if sortInHero || !p.sortAvailable() {
		return p.table.Header(gtx, e, opts)
	}
	headerGTX := gtx
	headerGTX.Constraints.Min.Y = max(headerGTX.Constraints.Min.Y, gtx.Dp(40))
	dims := p.table.Header(headerGTX, e, opts)
	width := min(gtx.Dp(128), dims.Size.X)
	choiceGTX := gtx
	choiceGTX.Constraints.Min = image.Pt(width, 0)
	choiceGTX.Constraints.Max = image.Pt(width, dims.Size.Y)
	x := max(0, dims.Size.X-gtx.Dp(125)-width)
	offset := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
	if p.sortPending {
		choiceGTX = choiceGTX.Disabled()
	}
	p.sortChoice.Layout(choiceGTX, e.Th, e.Overlays, sortLabels, sortIndex(p.selectedSort()))
	offset.Pop()
	return dims
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
	if p.save.Clicked(gtx) {
		p.setSaved(e, !p.saved(e))
	}
	if p.download.Clicked(gtx) {
		p.confirmDownload(e)
	}
	if p.more.Clicked(gtx) && e.Overlays != nil {
		anchor := e.Overlays.AnchorOf(&p.more.Clickable, image.Pt(gtx.Dp(40), gtx.Dp(40)))
		e.Overlays.Open(&p.moreTag, anchor, look.Placement{Prefer: look.Below, Align: look.End}, p.layoutMore(e))
	}
}

func (p *Page) collectionPlaying(e *env.Env) bool {
	stateSnapshot := e.Store.Snapshot()
	return !stateSnapshot.Playback.Paused && stateSnapshot.Playback.Now != nil && stateSnapshot.Queue.SourceName != nil && *stateSnapshot.Queue.SourceName == p.Title()
}

func (p *Page) saved(e *env.Env) bool {
	for _, item := range e.Store.Snapshot().Personal.Saved {
		if item.ID == p.route.ID && strings.EqualFold(item.Kind, "playlist") {
			return true
		}
	}
	return false
}

func (p *Page) setSaved(e *env.Env, save bool) {
	card := model.BrowseItem{Kind: "playlist", ID: p.route.ID, Title: p.Title(), Thumbnail: p.page.Thumbnail, Subtitle: p.page.Subtitle}
	changed := e.Store.UpdatePersonal(func(personal *state.Personal) bool {
		for index, item := range personal.Saved {
			if item.ID != card.ID || !strings.EqualFold(item.Kind, card.Kind) {
				continue
			}
			if save {
				return false
			}
			personal.Saved = append(personal.Saved[:index], personal.Saved[index+1:]...)
			return true
		}
		if !save {
			return false
		}
		personal.Saved = append(personal.Saved, card)
		return true
	})
	if !changed {
		return
	}
	if save {
		e.Toast("Saved to library", env.ToastSuccess)
	} else {
		e.Toast("Removed from library", env.ToastSuccess)
	}
}

func (p *Page) layoutActs(e *env.Env, sortInHero bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		playing := p.collectionPlaying(e)
		p.play.Kind, p.play.Icon, p.play.Label = kit.PlaqueGold, look.Icon.Play, "Play"
		if playing {
			p.play.Icon, p.play.Label = look.Icon.Pause, "Pause"
		}
		p.shuffle.Kind, p.shuffle.Icon, p.shuffle.Description = kit.PlaqueQuiet, look.Icon.Shuffle, "Shuffle playlist"
		p.save.Kind, p.save.Icon, p.save.Description = kit.PlaqueQuiet, look.Icon.HeartOutline, "Save playlist"
		if p.saved(e) {
			p.save.Icon, p.save.Description = look.Icon.Heart, "Remove playlist from library"
		}
		p.download.Kind, p.download.Icon, p.download.Description = kit.PlaqueQuiet, look.Icon.Download, "Download playlist"
		active, progress, current, complete := playlistDownload(e, p.Title(), p.trackIDs, len(p.page.Items))
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
		p.more.Kind, p.more.Icon, p.more.Description = kit.PlaqueQuiet, look.Icon.More, "More playlist actions"
		play := func(gtx layout.Context) layout.Dimensions {
			if len(p.page.Items) == 0 {
				gtx = gtx.Disabled()
			}
			return p.play.Layout(gtx, e.Th.Theme)
		}
		shuffle := func(gtx layout.Context) layout.Dimensions { return p.shuffle.Layout(gtx, e.Th.Theme) }
		save := func(gtx layout.Context) layout.Dimensions { return p.save.Layout(gtx, e.Th.Theme) }
		download := func(gtx layout.Context) layout.Dimensions { return p.layoutDownload(gtx, e, active, progress) }
		more := func(gtx layout.Context) layout.Dimensions { return p.more.Layout(gtx, e.Th.Theme) }
		var actions [6]layout.Widget
		actions[0], actions[2], actions[3], actions[4], actions[5] = play, shuffle, save, download, more
		if p.sortAvailable() && sortInHero {
			actions[1] = func(gtx layout.Context) layout.Dimensions {
				if p.sortPending {
					gtx = gtx.Disabled()
				}
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(128))
				return p.sortChoice.Layout(gtx, e.Th, e.Overlays, sortLabels, sortIndex(p.selectedSort()))
			}
		}
		return p.heroActions.Layout(gtx, actions[:])
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

func (p *Page) layoutMore(e *env.Env) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		editable := playlistEditable(p.route.ID, p.page.Owned)
		items := playlistMoreItems
		if editable {
			items = playlistMoreEditableItems
		}
		dims := p.moreMenu.Layout(gtx, e.Th.Theme, items)
		if index, ok := p.moreMenu.Clicked(gtx); ok {
			e.Overlays.Close(&p.moreTag)
			switch index {
			case 0:
				target := env.MenuTarget{Playlist: &p.page, Context: p.route.ID}
				p.itemMenu.OpenAt(e, target, e.Overlays.Press())
			case 1:
				p.exportDialog.Open(p.Title())
			case 2:
				if editable {
					p.editDialog.Page, p.editDialog.Open = &p.page, true
				}
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		return dims
	}
}

func (p *Page) setSort(e *env.Env, value model.PlaylistSort) {
	p.sortPending = true
	act.Call[protocol.Null](e, protocol.MethodSetPlaylistSort, protocol.SetPlaylistSortParams{PlaylistID: p.route.ID, Sort: value}, func(_ protocol.Null, err error) {
		p.sortPending = false
		if err != nil {
			e.Toast(err.Error(), env.ToastError)
			return
		}
		if p.page.SortMenu == nil {
			p.page.SortMenu = &model.PlaylistSortMenu{Editable: true}
		}
		p.page.SortMenu.Selected = &value
		p.Refresh(e)
	})
}

func (p *Page) selectedSort() model.PlaylistSort {
	if p.page.SortMenu == nil || p.page.SortMenu.Selected == nil {
		return model.PlaylistSortDefault
	}
	return *p.page.SortMenu.Selected
}

func sortIndex(value model.PlaylistSort) int {
	for index, candidate := range sortValues {
		if candidate == value {
			return index
		}
	}
	return 0
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
			e.Toast("Could not load more", env.ToastError)
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
	p.meta = playlistMeta(p.page)
	clear(p.trackIDs)
	for _, track := range p.page.Items {
		p.trackIDs[track.VideoID] = struct{}{}
	}
}

func playlistMeta(page model.PlaylistPage) []kit.Span {
	meta := make([]kit.Span, 0, 7)
	appendMeta := func(text string, mono bool) {
		if strings.TrimSpace(text) == "" {
			return
		}
		if len(meta) > 0 {
			meta = append(meta, kit.Span{Text: " · "})
		}
		meta = append(meta, kit.Span{Text: text, Mono: mono})
	}
	appendMeta(playlistByline(value(page.Subtitle)), false)
	appendMeta(trackCount(len(page.Items)), true)
	appendMeta(collectionDuration(totalDuration(page.Items)), true)
	return meta
}
func playlistByline(subtitle string) string {
	parts := strings.Split(subtitle, "·")
	kept := parts[:0]
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && !playlistFact(part) {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " · ")
}

func playlistFact(part string) bool {
	lower := strings.ToLower(part)
	hasDigit := strings.IndexFunc(lower, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0
	if !hasDigit {
		return false
	}
	return strings.Contains(lower, "song") ||
		strings.Contains(lower, "track") ||
		strings.Contains(lower, " min") ||
		strings.Contains(lower, "minute") ||
		strings.Contains(lower, " hr") ||
		strings.Contains(lower, "hour")
}

func playlistKind(id string) string {
	if id == "VLLM" || id == "spotify:playlist:liked" || id == "liked" || id == model.LocalLikedPlaylistID {
		return "Your library"
	}
	switch id {
	case model.OnRepeatPlaylistID, model.RecentlyPlayedPlaylistID, model.RediscoverPlaylistID:
		return "Smart playlist"
	}
	if isDevice(id) {
		return "Local playlist"
	}
	return "Playlist"
}

func playlistEditable(id string, owned bool) bool {
	switch id {
	case "VLLM", "spotify:playlist:liked", "liked", model.LocalLikedPlaylistID,
		model.OnRepeatPlaylistID, model.RecentlyPlayedPlaylistID, model.RediscoverPlaylistID:
		return false
	default:
		return owned || isDevice(id)
	}
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

func playlistDownload(e *env.Env, title string, ids map[string]struct{}, itemCount int) (active bool, progress, current int, complete bool) {
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

func isDevice(id string) bool { return strings.HasPrefix(id, model.LocalPlaylistPrefix) }

func value(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
