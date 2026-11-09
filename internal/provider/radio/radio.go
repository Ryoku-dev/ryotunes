package radio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
	sourceprovider "github.com/ryoku-dev/ryotunes/internal/provider"
)

const (
	// IDPrefix prefixes internet-radio track IDs.
	IDPrefix            = "RYOTUNES_RADIO:"
	defaultDiscoveryURL = "https://all.api.radio-browser.info/json/servers"
	userAgent           = "Ryotunes/2.4 (+https://github.com/ashmitvoid/ryotunes)"
	maxResponseBytes    = 2 << 20
	maxQueryCharacters  = 128
)

var fallbackServers = [...]string{
	"https://de1.api.radio-browser.info",
	"https://nl1.api.radio-browser.info",
	"https://at1.api.radio-browser.info",
}

// Cache persists normalized stations needed to restore live-radio queues.
type Cache interface {
	PutRadioStation(model.RadioStation) error
	RadioStation(stationUUID string) (model.RadioStation, bool, error)
}

// Options configures the Radio Browser client.
type Options struct {
	BaseURL      string
	DiscoveryURL string
	HTTPClient   *http.Client
	Cache        Cache
	Log          *slog.Logger
}

// Provider serves Radio Browser directory rows as live streams.
type Provider struct {
	baseURL      string
	discoveryURL string
	client       *http.Client
	cache        Cache
	log          *slog.Logger
	mu           sync.RWMutex
	stations     map[string]model.RadioStation
}

// New creates a demand-driven radio provider without making a network request.
func New(o Options) *Provider {
	client := o.HTTPClient
	if client == nil {
		client = netx.Client(10 * time.Second)
	} else {
		cloned := *client
		if cloned.Timeout == 0 {
			cloned.Timeout = 10 * time.Second
		}
		client = &cloned
	}
	if o.DiscoveryURL == "" {
		o.DiscoveryURL = defaultDiscoveryURL
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	return &Provider{
		baseURL: strings.TrimRight(o.BaseURL, "/"), discoveryURL: o.DiscoveryURL,
		client: client, cache: o.Cache, log: o.Log, stations: make(map[string]model.RadioStation),
	}
}

// ID identifies the internet-radio provider.
func (p *Provider) ID() model.Provider { return model.ProviderRadio }

// Capabilities reports that radio playback is exposed through the dedicated directory RPC.
func (p *Provider) Capabilities() sourceprovider.Capabilities {
	return sourceprovider.Capabilities{}
}

// Home is unsupported for internet radio.
func (p *Provider) Home(context.Context, string) (model.Feed, error) {
	return model.Feed{}, sourceprovider.ErrUnsupported
}

// HomeMore is unsupported for internet radio.
func (p *Provider) HomeMore(context.Context, string) (model.Feed, error) {
	return model.Feed{}, sourceprovider.ErrUnsupported
}

// Search is unsupported; Stations is the dedicated directory search.
func (p *Provider) Search(context.Context, string, model.SearchKind) (model.SearchPage, error) {
	return model.SearchPage{}, sourceprovider.ErrUnsupported
}

// SearchMore is unsupported for internet radio.
func (p *Provider) SearchMore(context.Context, string) (model.SearchPage, error) {
	return model.SearchPage{}, sourceprovider.ErrUnsupported
}

// SearchAll is unsupported for internet radio.
func (p *Provider) SearchAll(context.Context, string) (model.SearchAll, error) {
	return model.SearchAll{}, sourceprovider.ErrUnsupported
}

// SearchAllMore is unsupported for internet radio.
func (p *Provider) SearchAllMore(context.Context, string) (model.SearchAll, error) {
	return model.SearchAll{}, sourceprovider.ErrUnsupported
}

// SearchCards is unsupported for internet radio.
func (p *Provider) SearchCards(context.Context, string, string) (model.CardsPage, error) {
	return model.CardsPage{}, sourceprovider.ErrUnsupported
}

// SearchCardsMore is unsupported for internet radio.
func (p *Provider) SearchCardsMore(context.Context, string) (model.CardsPage, error) {
	return model.CardsPage{}, sourceprovider.ErrUnsupported
}

// Suggest is unsupported for internet radio.
func (p *Provider) Suggest(context.Context, string) ([]string, error) {
	return nil, sourceprovider.ErrUnsupported
}

// Album is unsupported for internet radio.
func (p *Provider) Album(context.Context, string) (model.AlbumPage, error) {
	return model.AlbumPage{}, sourceprovider.ErrUnsupported
}

// Artist is unsupported for internet radio.
func (p *Provider) Artist(context.Context, string) (model.ArtistPage, error) {
	return model.ArtistPage{}, sourceprovider.ErrUnsupported
}

// Playlist is unsupported for internet radio.
func (p *Provider) Playlist(context.Context, string, model.PlaylistSort, bool) (model.PlaylistPage, error) {
	return model.PlaylistPage{}, sourceprovider.ErrUnsupported
}

// PlaylistMore is unsupported for internet radio.
func (p *Provider) PlaylistMore(context.Context, string) (model.PlaylistTracksPage, error) {
	return model.PlaylistTracksPage{}, sourceprovider.ErrUnsupported
}

// Library is unsupported for internet radio.
func (p *Provider) Library(context.Context) (model.Library, error) {
	return model.Library{}, sourceprovider.ErrUnsupported
}

// Resolve resolves a synthetic radio ID to a live station stream.
func (p *Provider) Resolve(ctx context.Context, track model.Track, _ model.Quality) (model.Stream, error) {
	uuid, ok := UUIDFromID(track.VideoID)
	if !ok {
		return model.Stream{}, sourceprovider.ErrNotFound
	}
	station, err := p.Station(ctx, uuid)
	if err != nil {
		return model.Stream{}, err
	}
	if p.cache != nil {
		if err := p.cache.PutRadioStation(station); err != nil {
			p.log.Debug("could not cache selected radio station", "station", uuid, "error", err)
		}
	}
	go p.countClick(uuid)
	return model.Stream{URL: station.StreamURL, IsLive: true, Codec: station.Codec, Bitrate: int(station.Bitrate)}, nil
}

// Next returns no continuation because a live station is an endless single item.
func (p *Provider) Next(context.Context, model.Track, []string) ([]model.Track, error) {
	return []model.Track{}, nil
}

// Account returns the radio source's permanently signed-out account shape.
func (p *Provider) Account(context.Context) (model.Account, error) {
	return model.Account{SignedIn: false}, nil
}

// StartRadio rejects provider-radio seeding for live-radio IDs.
func (p *Provider) StartRadio(context.Context, string, string) ([]model.Track, string, error) {
	return nil, "", sourceprovider.ErrUnsupported
}

// Stations returns top-voted stations or a station-name search page.
func (p *Provider) Stations(ctx context.Context, query string, offset, limit int) ([]model.RadioStation, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) > maxQueryCharacters {
		return nil, errors.New("Radio search is limited to 128 characters.")
	}
	if offset < 0 {
		offset = 0
	}
	if offset > 10_000 {
		offset = 10_000
	}
	if limit == 0 {
		limit = 36
	} else if limit < 0 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}
	values := url.Values{}
	values.Set("offset", strconv.Itoa(offset))
	values.Set("limit", strconv.Itoa(limit))
	values.Set("hidebroken", "true")
	path := "/json/stations/topvote"
	if query != "" {
		path = "/json/stations/search"
		values.Set("name", query)
		values.Set("order", "votes")
		values.Set("reverse", "true")
	}
	var rows []stationJSON
	if err := p.getJSON(ctx, path+"?"+values.Encode(), &rows); err != nil {
		return nil, err
	}
	stations := make([]model.RadioStation, 0, len(rows))
	for _, row := range rows {
		station, ok := normalizeStation(row.model())
		if !ok {
			continue
		}
		stations = append(stations, station)
	}
	p.remember(stations)
	return stations, nil
}

// Station resolves one UUID from the bounded in-memory directory result or native lookup endpoint.
func (p *Provider) Station(ctx context.Context, raw string) (model.RadioStation, error) {
	uuid := strings.TrimSpace(raw)
	if !validUUID(uuid) {
		return model.RadioStation{}, errors.New("Invalid radio station id.")
	}
	p.mu.RLock()
	station, ok := p.stations[uuid]
	p.mu.RUnlock()
	if ok {
		return station, nil
	}
	if p.cache != nil {
		station, ok, err := p.cache.RadioStation(uuid)
		if err != nil {
			return model.RadioStation{}, err
		}
		if ok {
			station, valid := normalizeStation(station)
			if valid {
				p.remember([]model.RadioStation{station})
				return station, nil
			}
		}
	}
	var rows []stationJSON
	if err := p.getJSON(ctx, "/json/stations/byuuid/"+url.PathEscape(uuid), &rows); err != nil {
		return model.RadioStation{}, err
	}
	for _, row := range rows {
		candidate, valid := normalizeStation(row.model())
		if valid && candidate.StationUUID == uuid {
			p.remember([]model.RadioStation{candidate})
			return candidate, nil
		}
	}
	return model.RadioStation{}, &stationNotFoundError{}
}

// Track maps a normalized station to the one-item live-radio queue shape.
func Track(station model.RadioStation) model.Track {
	album := "Internet Radio"
	var thumbnail *string
	if station.Favicon != "" {
		value := station.Favicon
		thumbnail = &value
	}
	return model.Track{
		VideoID: ID(station.StationUUID), Title: station.Name, Artists: "Live Radio",
		Album: &album, Thumbnail: thumbnail,
	}
}

// ID builds an internet-radio track ID.
func ID(uuid string) string { return IDPrefix + uuid }

// UUIDFromID extracts and validates the UUID in an internet-radio track ID.
func UUIDFromID(id string) (string, bool) {
	uuid, ok := strings.CutPrefix(id, IDPrefix)
	return uuid, ok && validUUID(uuid)
}

type stationNotFoundError struct{}

func (*stationNotFoundError) Error() string        { return "That radio station is no longer available." }
func (*stationNotFoundError) Is(target error) bool { return target == sourceprovider.ErrNotFound }

type directoryUnavailableError struct {
	message string
}

func (e *directoryUnavailableError) Error() string { return e.message }
func (e *directoryUnavailableError) Is(target error) bool {
	return target == sourceprovider.ErrUnavailable
}

type stationJSON struct {
	StationUUID string `json:"stationuuid"`
	Name        string `json:"name"`
	URLResolved string `json:"url_resolved"`
	URL         string `json:"url"`
	Homepage    string `json:"homepage"`
	Favicon     string `json:"favicon"`
	Country     string `json:"country"`
	CountryCode string `json:"countrycode"`
	Tags        string `json:"tags"`
	Codec       string `json:"codec"`
	Bitrate     int64  `json:"bitrate"`
	Votes       int64  `json:"votes"`
	ClickCount  int64  `json:"clickcount"`
}

func (s stationJSON) model() model.RadioStation {
	return model.RadioStation{
		StationUUID: s.StationUUID, Name: s.Name, StreamURL: s.URLResolved, URL: s.URL,
		Homepage: s.Homepage, Favicon: s.Favicon, Country: s.Country,
		CountryCode: s.CountryCode, Tags: s.Tags, Codec: s.Codec,
		Bitrate: s.Bitrate, Votes: s.Votes, ClickCount: s.ClickCount,
	}
}

func normalizeStation(station model.RadioStation) (model.RadioStation, bool) {
	station.StationUUID = strings.TrimSpace(station.StationUUID)
	station.Name = cleanText(station.Name, 200)
	station.StreamURL = strings.TrimSpace(station.StreamURL)
	station.URL = strings.TrimSpace(station.URL)
	station.Homepage = strings.TrimSpace(station.Homepage)
	station.Favicon = strings.TrimSpace(station.Favicon)
	station.Country = cleanText(station.Country, 100)
	station.CountryCode = strings.ToUpper(cleanText(station.CountryCode, 8))
	station.Tags = cleanText(station.Tags, 512)
	station.Codec = strings.ToUpper(cleanText(station.Codec, 32))
	if !validUUID(station.StationUUID) {
		return model.RadioStation{}, false
	}
	if !safeHTTPURL(station.StreamURL) {
		station.StreamURL = station.URL
	}
	if station.Name == "" || !safeHTTPURL(station.StreamURL) {
		return model.RadioStation{}, false
	}
	if station.Homepage != "" && !safeHTTPURL(station.Homepage) {
		station.Homepage = ""
	}
	if station.Favicon != "" && !safeHTTPURL(station.Favicon) {
		station.Favicon = ""
	}
	return station, true
}

func cleanText(value string, maximum int) string {
	out := make([]rune, 0, minimum(utf8.RuneCountInString(value), maximum))
	for _, character := range strings.TrimSpace(value) {
		if unicode.IsControl(character) {
			continue
		}
		out = append(out, character)
		if len(out) == maximum {
			break
		}
	}
	return string(out)
}

func validUUID(uuid string) bool {
	if len(uuid) < 1 || len(uuid) > 96 {
		return false
	}
	for _, character := range uuid {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '-' && character != '_' {
			return false
		}
		if character > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func safeHTTPURL(raw string) bool {
	if len(raw) > 4096 {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return false
	}
	if parsed.User != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return publicIP(ip)
	}
	return strings.Contains(host, ".")
}

func publicIP(ip net.IP) bool {
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		a, b, c := ipv4[0], ipv4[1], ipv4[2]
		return !(a == 0 || (a == 100 && b >= 64 && b <= 127) ||
			(a == 192 && b == 0 && (c == 0 || c == 2)) ||
			(a == 198 && (b == 18 || b == 19 || (b == 51 && c == 100))) ||
			(a == 203 && b == 0 && c == 113) || a >= 224)
	}
	if len(ip) != net.IPv6len {
		return false
	}
	allZeroPrefix := true
	for _, octet := range ip[:12] {
		allZeroPrefix = allZeroPrefix && octet == 0
	}
	if allZeroPrefix {
		return false
	}
	return !(ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8)
}

func (p *Provider) remember(stations []model.RadioStation) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, station := range stations {
		p.stations[station.StationUUID] = station
	}
	if len(p.stations) <= 256 {
		return
	}
	for id := range p.stations {
		delete(p.stations, id)
		if len(p.stations) <= 256 {
			break
		}
	}
}

func (p *Provider) countClick(uuid string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var ignored json.RawMessage
	if err := p.getJSON(ctx, "/json/url/"+url.PathEscape(uuid), &ignored); err != nil {
		p.log.Debug("could not count Radio Browser click", "station", uuid, "error", err)
	}
}

func (p *Provider) getJSON(ctx context.Context, path string, destination interface{}) error {
	bases, err := p.servers(ctx)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var failures []string
	for _, base := range bases {
		if err := ctx.Err(); err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		request.Header.Set("User-Agent", userAgent)
		response, err := p.client.Do(request)
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			failures = append(failures, base+": "+err.Error())
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			failures = append(failures, fmt.Sprintf("%s: HTTP %d", base, response.StatusCode))
			continue
		}
		data, err := boundedBody(response)
		if err != nil {
			failures = append(failures, base+": "+err.Error())
			continue
		}
		if err := json.Unmarshal(data, destination); err != nil {
			failures = append(failures, base+": invalid response")
			continue
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(failures) == 0 {
		return &directoryUnavailableError{message: "Radio Browser is unavailable right now"}
	}
	return &directoryUnavailableError{
		message: fmt.Sprintf("Radio Browser is unavailable right now (%s)", strings.Join(failures, "; ")),
	}
}

func (p *Provider) servers(ctx context.Context) ([]string, error) {
	if p.baseURL != "" {
		return []string{p.baseURL}, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.discoveryURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", userAgent)
	response, err := p.client.Do(request)
	if err != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	servers := make([]string, 0, len(fallbackServers)+4)
	if err == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
		if data, bodyErr := boundedBody(response); bodyErr == nil {
			var mirrors []struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(data, &mirrors) == nil {
				for _, mirror := range mirrors {
					if host, ok := officialMirrorHost(mirror.Name); ok {
						servers = append(servers, "https://"+host)
					}
				}
			}
		}
	} else if err == nil {
		response.Body.Close()
	}
	servers = append(servers, fallbackServers[:]...)
	unique := servers[:0]
	seen := make(map[string]struct{}, len(servers))
	for _, server := range servers {
		if _, duplicate := seen[server]; duplicate {
			continue
		}
		seen[server] = struct{}{}
		unique = append(unique, server)
	}
	return unique, nil
}

func officialMirrorHost(raw string) (string, bool) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	valid := host == "api.radio-browser.info" || strings.HasSuffix(host, ".api.radio-browser.info")
	return host, valid
}

func boundedBody(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	if response.ContentLength > maxResponseBytes {
		return nil, errors.New("Radio Browser response was too large.")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponseBytes {
		return nil, errors.New("Radio Browser response was too large.")
	}
	return data, nil
}

func minimum(left, right int) int {
	if left < right {
		return left
	}
	return right
}

var (
	_ sourceprovider.Provider = (*Provider)(nil)
	_ sourceprovider.Radio    = (*Provider)(nil)
)
