package settings

import (
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// section is one section of the Settings sheet: its id (the route's Params,
// the section's anchor), its title in sentence case, its number under
// Settings ("05.4", sheet.SettingsNumber) and its kanji gloss (empty when
// there is no real word for it). The sidebar reads them through Outline.
type section struct {
	id     string
	title  string
	number string
	gloss  string
}

// sections lists Settings' sections in the sheet's order.
var sections = func() []section {
	list := []section{
		{id: "account", title: "Account", gloss: "利用者"},
		{id: "playback", title: "Playback", gloss: sheet.NowPlaying.Gloss},
		{id: "sound", title: "Sound"},
		{id: "lyrics", title: "Lyrics", gloss: "歌詞"},
		{id: "downloads", title: "Downloads", gloss: sheet.Downloads.Gloss},
		{id: "local", title: "Local music", gloss: "手元"},
		{id: "playlists", title: "Playlists", gloss: sheet.Playlists.Gloss},
		{id: "appearance", title: "Appearance"},
		{id: "integrations", title: "Integrations", gloss: "連携"},
		{id: "data", title: "Data and storage", gloss: "保管"},
		{id: "diagnostics", title: "Diagnostics", gloss: "診断"},
		{id: "about", title: "About"},
	}
	for index := range list {
		list[index].number = sheet.SettingsNumber(index + 1)
		// the Hub's own words where it has the section too
		if gloss := sheet.SectionGloss(list[index].id, true); gloss != "" {
			list[index].gloss = gloss
		}
	}
	return list
}()

// The section names (the outline and the chips) and the ruled heads' marks,
// built once so a frame allocates neither.
var outlineNames, sectionMarks = func() (names, marks []string) {
	for index, section := range sections {
		names = append(names, section.title)
		marks = append(marks, kit.Ordinal(index+1, len(sections)))
	}
	return
}()

// sectionIndex is the index in sections of the section with id, or 0 (the
// first section) for an unknown id.
func sectionIndex(id string) int {
	for index, section := range sections {
		if section.id == id {
			return index
		}
	}
	return 0
}

type fieldValue struct {
	field       kit.TextField
	source      string
	initialized bool
}

func (f *fieldValue) sync(value string) {
	current := f.field.Text()
	if !f.initialized {
		f.field.SetText(value)
		f.source, f.initialized = value, true
		return
	}
	if current == value {
		f.source = value
		return
	}
	if value != f.source && current == f.source {
		f.field.SetText(value)
		f.source = value
	}
}

type appearanceControls struct {
	density         look.Choice
	scale           kit.Stepper
	lowResource     kit.Toggle
	visualizer      kit.Toggle
	visualizerStyle look.FlowChoice
	waveProgress    kit.Toggle
	coverTint       kit.Toggle
	accent          look.FlowChoice
	openFloating    kit.Toggle
	tray, autostart kit.Toggle
	// the desktop's look, as the claims show it this frame
	claims    styleClaims
	claimRoll [claimCount]look.CountRoll
	handoff   kit.Plaque
}

type playbackControls struct {
	autoplay, duplicates, stopAfter, history kit.Toggle
	quality                                  look.Choice
	clients                                  map[model.StreamClient]*kit.Toggle
	more                                     disclosure
}

type soundControls struct {
	open, equalizer kit.Plaque
}

type lyricsControls struct {
	wordByWord, timedSource, romanize kit.Toggle
}

type authFlowControls struct {
	open, copy, cancel kit.Plaque
}

type accountControls struct {
	youtubeSignIn, youtubeSignOut kit.Plaque
	youtubeFlow                   authFlowControls
	identity                      look.Choice
	spotifySignIn, spotifySignOut kit.Plaque
	spotifyFlow                   authFlowControls
	soundCloudSignIn              kit.Plaque
	soundCloudSignOut             kit.Plaque
	soundCloudFlow                authFlowControls
	tidalDeviceID                 fieldValue
	tidalDeviceSecret             fieldValue
	tidalPKCEID                   fieldValue
	tidalSave, tidalSignIn        kit.Plaque
	tidalSignOut                  kit.Plaque
	tidalFlow                     authFlowControls
	more                          disclosure
}

type downloadControls struct {
	path          fieldValue
	format        look.Choice
	artistFolders kit.Toggle
	embedMetadata kit.Toggle
	workers       kit.Stepper
	save          kit.Plaque
}

type localControls struct {
	path         fieldValue
	add, rescan  kit.Plaque
	remove       map[string]*kit.Plaque
	counts       map[string]*look.CountRoll
	library      model.LocalLibrary
	loaded, busy bool
	errorMessage string
}

type playlistControls struct {
	importPath, exportPath fieldValue
	importButton           kit.Plaque
	exportButton           kit.Plaque
	busy                   bool
}

type integrationControls struct {
	discord                         kit.Toggle
	presence                        fieldValue
	savePresence                    kit.Plaque
	lastfmConnect, lastfmDisconnect kit.Plaque
	listenURL                       fieldValue
	saveListenURL                   kit.Plaque
	more                            disclosure
}

type dataControls struct {
	proxy     fieldValue
	saveProxy kit.Plaque
	rotate    kit.Toggle
	clear     kit.Plaque
	sync      kit.Plaque
	more      disclosure
}

type diagnosticsControls struct {
	open kit.Plaque
}

type aboutControls struct {
	check, install, changelog, licenses kit.Plaque
	links                               [3]kit.Plaque
	notes                               disclosure
}

type page struct {
	scroll       look.Scroller
	sectionChips look.FlowChoice
	// outlined is set by the shell each frame: the sidebar shows the
	// sections, so the page shows no chips of its own
	outlined bool

	// the scroller's items: the page head, then one per section
	itemHeights    []int
	currentSection int
	initialSection int
	initialScroll  bool
	// indexHold keeps the section a jump chose current until the reader
	// scrolls; -1 lets the scroll position say
	indexHold      int
	goTo           int
	viewportHeight int

	reveal       sheet.Reveal
	revealFrames int
	revealBase   int
	revealSeen   uint64

	changedRoll look.CountRoll
	sectionsFig [1]sheet.Spec

	rows       [][]*settingRow
	rowSection int
	rowCursor  int
	// slabStart is the cursor of the slab's first row, which draws no
	// hairline above it
	slabStart int
	// th is the theme of the frame being laid out, for the slabs
	th *look.Theme
	// env is the frame's environment, for the rows' reset and its undo
	env *env.Env
	// pendingSettings holds daemon settings just written, shown until the
	// daemon answers, so a toggle moves once and stays
	pendingSettings map[model.SettingKey]string

	refreshed  bool
	generation uint64
	loading    bool
	loadError  string

	streamClients []model.StreamClient
	identities    model.AccountIdentities
	downloads     model.DownloadSettings
	discord       model.DiscordStatus
	lastfm        model.LastfmStatus
	diagnostics   model.LogsResult
	daemonVersion string
	protocol      int
	update        *model.UpdateInfo
	checking      bool
	installing    bool
	clearing      bool
	syncing       bool

	appearance    appearanceControls
	playback      playbackControls
	lyrics        lyricsControls
	accounts      accountControls
	download      downloadControls
	local         localControls
	playlists     playlistControls
	integrations  integrationControls
	data          dataControls
	diagnosticsUI diagnosticsControls
	about         aboutControls
	sound         soundControls
	retry         kit.Plaque
}

// New creates a retained Settings page for the requested route; the route's
// Params names the section it opens on.
func New(r env.Route) env.Page {
	initialSection := -1
	if r.Params != "" {
		initialSection = sectionIndex(r.Params)
	}
	p := &page{
		itemHeights:     make([]int, len(sections)+1),
		initialSection:  initialSection,
		currentSection:  max(0, initialSection),
		indexHold:       initialSection,
		goTo:            -1,
		rows:            make([][]*settingRow, len(sections)),
		pendingSettings: make(map[model.SettingKey]string),
	}
	p.sectionsFig[0] = sheet.Spec{Label: "Sections", Value: kit.Pad2(len(sections))}
	p.playback.clients = make(map[model.StreamClient]*kit.Toggle)
	p.local.remove = make(map[string]*kit.Plaque)
	p.local.counts = make(map[string]*look.CountRoll)
	p.downloads = model.DownloadSettings{Format: model.DownloadOriginal, Workers: 1, OrganizeByArtist: true, EmbedMetadata: true}
	p.data.rotate.Value = true
	return p
}

// Title returns the page title shown by the shell.
func (p *page) Title() string { return "Settings" }

// SetOutline is called by the shell every frame before the page's Layout:
// shown says the sidebar shows the sections (expanded class, sidebar
// unfolded), so the page drops its own section chips.
func (p *page) SetOutline(shown bool) { p.outlined = shown }

// GoTo glides the page to the section at index (look.Scroller.GlideTo,
// every row between laid out on the way, R9) and holds it current until the
// reader scrolls; the sidebar calls it when a leaf is pressed.
func (p *page) GoTo(index int) {
	if index < 0 || index >= len(sections) {
		return
	}
	p.goTo = index
	p.currentSection, p.indexHold = index, index
}

// Outline is what the sidebar's outline shows: the sections' names and the
// section being read. The slice is shared; never write to it.
func (p *page) Outline() (names []string, current int) {
	return outlineNames, p.currentSection
}

// Masthead is what the frame's masthead says while Settings is open: the
// number of the section being read ("05.4"), its name, its gloss (ungated)
// and how far the page is read, 0..1, or -1 when it fits the stage.
func (p *page) Masthead() (number, part, partGloss string, progress float32) {
	section := sections[min(max(p.currentSection, 0), len(sections)-1)]
	return section.number, section.title, section.gloss, p.readProgress()
}
