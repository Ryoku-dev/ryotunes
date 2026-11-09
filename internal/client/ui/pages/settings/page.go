package settings

import (
	"gioui.org/layout"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type sectionSpec struct {
	slug  string
	title string
}

var settingsSections = []sectionSpec{
	{slug: "account", title: "Account"},
	{slug: "playback", title: "Playback"},
	{slug: "sound", title: "Sound"},
	{slug: "lyrics", title: "Lyrics"},
	{slug: "downloads", title: "Downloads"},
	{slug: "local", title: "Local music"},
	{slug: "playlists", title: "Playlists"},
	{slug: "appearance", title: "Appearance"},
	{slug: "integrations", title: "Integrations"},
	{slug: "data", title: "Data and storage"},
	{slug: "diagnostics", title: "Diagnostics"},
	{slug: "about", title: "About"},
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
	colours, density, motion look.Choice
	scale                    kit.Stepper
	lowResource              kit.Toggle
	visualizer               kit.Toggle
	visualizerStyle          kit.Filters
	visualizerStyleScroll    look.HScroller
	waveProgress             kit.Toggle
	coverTint                kit.Toggle
	accent                   kit.Filters
	accentScroll             look.HScroller
	corners                  kit.Filters
	cornersScroll            look.HScroller
	corner                   kit.Slider
	cornerSaveArmed          bool
	palette                  fieldValue
	usePalette, clearPalette kit.Plaque
	tray, autostart          kit.Toggle
}

type playbackControls struct {
	autoplay, duplicates, stopAfter, history kit.Toggle
	quality                                  look.Choice
	clients                                  map[model.StreamClient]*kit.Toggle
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
}

type dataControls struct {
	proxy     fieldValue
	saveProxy kit.Plaque
	rotate    kit.Toggle
	clear     kit.Plaque
	sync      kit.Plaque
}

type diagnosticsControls struct {
	open kit.Plaque
}

type aboutControls struct {
	check, install, changelog, notes kit.Plaque
	links                            [4]kit.Plaque
	showNotes                        bool
}

type page struct {
	scroll look.Scroller
	index  kit.SectionIndex

	sectionLabels  []string
	sectionHeights []int
	currentSection int
	initialSection int
	initialScroll  bool
	indexHold      int
	viewportHeight int

	rowDescriptors [][]*settingRowDescriptor
	rowChildren    [][]layout.FlexChild
	rowSection     int
	rowCursor      int
	refreshed      bool
	generation     uint64
	loading        bool
	loadError      string

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

// New creates a retained Settings page for the requested route.
func New(r env.Route) env.Page {
	initialSection := sectionIndex(r.Params)
	p := &page{
		sectionLabels:  make([]string, len(settingsSections)),
		sectionHeights: make([]int, len(settingsSections)),
		initialSection: initialSection,
		currentSection: initialSection,
		indexHold:      initialSection,
		rowDescriptors: make([][]*settingRowDescriptor, len(settingsSections)),
		rowChildren:    make([][]layout.FlexChild, len(settingsSections)),
	}
	for i, section := range settingsSections {
		p.sectionLabels[i] = section.title
	}
	p.playback.clients = make(map[model.StreamClient]*kit.Toggle)
	p.local.remove = make(map[string]*kit.Plaque)
	p.downloads = model.DownloadSettings{Format: model.DownloadOriginal, Workers: 1, OrganizeByArtist: true, EmbedMetadata: true}
	p.data.rotate.Value = true
	return p
}

// Title returns the page title shown by the shell.
func (p *page) Title() string { return "Settings" }

func sectionIndex(slug string) int {
	for i, section := range settingsSections {
		if section.slug == slug {
			return i
		}
	}
	return 0
}
