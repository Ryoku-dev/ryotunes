package settings

import (
	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

type sectionSpec struct {
	slug  string
	title string
}

var settingsSections = []sectionSpec{
	{slug: "appearance", title: "Appearance"},
	{slug: "playback", title: "Playback"},
	{slug: "providers", title: "Providers and accounts"},
	{slug: "downloads", title: "Downloads"},
	{slug: "integrations", title: "Integrations"},
	{slug: "data", title: "Data and storage"},
	{slug: "local", title: "Local music"},
	{slug: "about", title: "About"},
}

type fieldValue struct {
	field       lithic.TextField
	source      string
	initialized bool
}

func (f *fieldValue) sync(value string) {
	current := f.field.Text()
	if !f.initialized {
		f.field.SetText(value)
		f.source = value
		f.initialized = true
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
	mode, density                 lithic.Picker
	reduce, romanize, lowResource lithic.Toggle
	scale                         lithic.Stepper
	palette                       fieldValue
	savePalette                   lithic.Button
}

type playbackControls struct {
	autoplay, duplicates, stopAfter, history, wordByWord lithic.Toggle
	qualityPicker                                        lithic.Picker
	clients                                              map[model.StreamClient]*lithic.Toggle
}

type providerControls struct {
	youtubeSignIn, youtubeSignOut lithic.Button
	identity                      lithic.Picker
	spotifySignIn, spotifySignOut lithic.Button
	soundCloudSignIn              lithic.Button
	soundCloudSignOut             lithic.Button
	tidalDeviceID                 fieldValue
	tidalDeviceSecret             fieldValue
	tidalPKCEID                   fieldValue
	tidalSave, tidalSignIn        lithic.Button
	tidalSignOut                  lithic.Button
	lastfmConnect                 lithic.Button
	lastfmDisconnect              lithic.Button
}

type downloadControls struct {
	path          fieldValue
	format        lithic.Picker
	artistFolders lithic.Toggle
	embedMetadata lithic.Toggle
	workers       lithic.Stepper
	save          lithic.Button
}

type integrationControls struct {
	discord, tray, autostart lithic.Toggle
	presence                 fieldValue
	savePresence             lithic.Button
	proxy                    fieldValue
	saveProxy                lithic.Button
	listenURL                fieldValue
	saveListenURL            lithic.Button
}

type dataControls struct {
	rotate lithic.Toggle
	clear  lithic.Button
	sync   lithic.Button
}

type aboutControls struct {
	check, install, diagnostics lithic.Button
	links                       [4]lithic.Button
}

type page struct {
	list lithic.List

	sectionButtons, chipButtons []lithic.Button
	chipList                    layout.List
	sectionHeights              []int
	currentSection              int
	initialSection              int
	initialScroll               bool

	refreshed  bool
	generation uint64
	loading    bool
	loadError  string

	streamClients        []model.StreamClient
	identities           model.AccountIdentities
	downloads            model.DownloadSettings
	discord              model.DiscordStatus
	lastfm               model.LastfmStatus
	daemonVersion        string
	update               *model.UpdateInfo
	checking, installing bool
	clearing             bool
	syncing              bool

	appearance       appearanceControls
	playback         playbackControls
	providers        providerControls
	download         downloadControls
	integrations     integrationControls
	data             dataControls
	about            aboutControls
	localOpen, retry lithic.Button
}

// New creates a retained Settings page for the requested route.
func New(r env.Route) env.Page {
	page := &page{
		sectionButtons: make([]lithic.Button, len(settingsSections)),
		chipButtons:    make([]lithic.Button, len(settingsSections)),
		sectionHeights: make([]int, len(settingsSections)),
		initialSection: sectionIndex(r.Params),
	}
	page.chipList.Axis = layout.Horizontal
	page.playback.clients = make(map[model.StreamClient]*lithic.Toggle)
	page.downloads = model.DownloadSettings{Format: model.DownloadOriginal, Workers: 1, OrganizeByArtist: true, EmbedMetadata: true}
	page.data.rotate.Value = true
	return page
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
