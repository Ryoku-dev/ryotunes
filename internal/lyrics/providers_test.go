package lyrics

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

// The live case: a fan channel's slowed upload of PIXY's LEGACY. Netease's
// search answers with an unrelated rap song of the same length first.
const neteaseUnrelatedSearch = `{"code":200,"result":{"songs":[
	{"id":1001,"name":"Pimpin","artists":[{"name":"Some Rapper"}],"duration":163000},
	{"id":1002,"name":"Legacy","artists":[{"name":"Other Artist"}],"duration":163500}
]}}`

const neteaseOriginalSearch = `{"code":200,"result":{"songs":[
	{"id":1001,"name":"Pimpin","artists":[{"name":"Some Rapper"}],"duration":163000},
	{"id":1003,"name":"LEGACY","artists":[{"name":"PIXY"}],"duration":140000}
]}}`

type providerFixture struct {
	t              *testing.T
	neteaseSearch  string
	mu             sync.Mutex
	neteaseQueries []string
	lyricIDs       []string
	lrclibGets     []url.Values
}

func (f *providerFixture) client() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		body, status := "", http.StatusOK
		switch {
		case request.URL.Host == "lrclib.net" && request.URL.Path == "/api/get":
			f.lrclibGets = append(f.lrclibGets, request.URL.Query())
			status = http.StatusNotFound
		case request.URL.Host == "lrclib.net" && request.URL.Path == "/api/search":
			body = `[]`
		case request.URL.Host == "music.163.com" && request.URL.Path == "/api/search/get":
			if err := request.ParseForm(); err != nil {
				f.t.Errorf("netease search form: %v", err)
			}
			f.neteaseQueries = append(f.neteaseQueries, request.PostForm.Get("s"))
			body = f.neteaseSearch
		case request.URL.Host == "music.163.com" && request.URL.Path == "/api/song/lyric":
			id := request.URL.Query().Get("id")
			f.lyricIDs = append(f.lyricIDs, id)
			switch id {
			case "1001":
				body = `{"lrc":{"lyric":"[00:01.00]I'm a pimp, hoes fallin' for the pimpin'\n[00:04.00]rap game"}}`
			case "1003":
				body = `{"lrc":{"lyric":"[00:10.00]first line\n[00:20.00]second line"}}`
			default:
				body = `{}`
			}
		case request.URL.Host == "c.y.qq.com":
			body = `{"data":{"song":{"list":[]}}}`
		case request.URL.Host == "songsearch.kugou.com":
			body = `{"data":{"lists":[]}}`
		default:
			f.t.Errorf("unexpected request %s", request.URL)
			status = http.StatusNotFound
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
}

func pixyRequest() Request {
	return Request{VideoID: "YjCBpTCgmjM", Title: "PIXY - LEGACY (slowed + reverb)", Artists: "GAJ3K", Duration: new(163.0)}
}

func fixtureService(f *providerFixture) *Service {
	return New(Options{HTTPClient: f.client(), Settings: provider.MapKV{string(model.SettingLyricsBoidu): "false"}})
}

func TestNeteaseRejectsUnrelatedSongOfTheSameLength(t *testing.T) {
	t.Setenv("RYOTUNES_LYRICS_ONLY", "")
	fixture := &providerFixture{t: t, neteaseSearch: neteaseUnrelatedSearch}
	service := fixtureService(fixture)

	out, err := service.netease(context.Background(), newQuery(pixyRequest()))
	if err != nil || out.lyrics != nil || !out.definitive {
		t.Fatalf("netease = %#v, %v; want a definitive miss", out, err)
	}

	lyrics, err := service.Get(context.Background(), pixyRequest())
	if err != nil || lyrics != nil {
		t.Fatalf("Get = %#v, %v; want no lyrics", lyrics, err)
	}
	if len(fixture.lyricIDs) != 0 {
		t.Fatalf("fetched Netease lyrics for %v", fixture.lyricIDs)
	}
	for _, sent := range fixture.neteaseQueries {
		if sent != "LEGACY PIXY" {
			t.Fatalf("Netease search = %q, want the cleaned %q", sent, "LEGACY PIXY")
		}
	}
	for _, sent := range fixture.lrclibGets {
		if sent.Get("track_name") != "LEGACY" || sent.Get("artist_name") != "PIXY" || sent.Has("duration") {
			t.Fatalf("LRCLIB get = %v; want the cleaned names and no length for a variant", sent)
		}
	}
}

func TestNeteaseRetimesOriginalForSlowedUpload(t *testing.T) {
	t.Setenv("RYOTUNES_LYRICS_ONLY", "")
	fixture := &providerFixture{t: t, neteaseSearch: neteaseOriginalSearch}
	lyrics, err := fixtureService(fixture).Get(context.Background(), pixyRequest())
	if err != nil || lyrics == nil {
		t.Fatalf("Get = %#v, %v", lyrics, err)
	}
	if lyrics.Source != "Netease Cloud Music" || !lyrics.Synced || len(lyrics.Lines) != 2 {
		t.Fatalf("lyrics = %#v", lyrics)
	}
	if strings.Join(fixture.lyricIDs, ",") != "1003" {
		t.Fatalf("Netease lyric ids = %v, want only the original", fixture.lyricIDs)
	}
	// 163 s against the original's 140 s
	for i, want := range []uint64{11643, 23286} {
		if got := lyrics.Lines[i].TimeMS; got == nil || *got != want {
			t.Fatalf("line %d time = %v, want %d", i, got, want)
		}
	}
}

func TestBoiduChecksTheLengthItsTTMLStates(t *testing.T) {
	ttml := `{"ttml":"<tt xmlns=\"http://www.w3.org/ns/ttml\"><body dur=\"2:14.000\"><div><p begin=\"3.5s\" end=\"5s\">Why you so mad?</p></div></body></tt>"}`
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(ttml))}, nil
	})}
	service := New(Options{HTTPClient: client})

	original := newQuery(Request{VideoID: "v", Title: "LEGACY", Artists: "PIXY", Duration: new(134.0)})
	out, err := service.boidu(context.Background(), original)
	if err != nil || out.lyrics == nil || *out.lyrics.Lines[0].TimeMS != 3500 {
		t.Fatalf("original = %#v, %v; want the TTML as it is", out.lyrics, err)
	}

	otherCut := newQuery(Request{VideoID: "v", Title: "LEGACY", Artists: "PIXY", Duration: new(163.0)})
	if out, err := service.boidu(context.Background(), otherCut); err != nil || out.lyrics != nil {
		t.Fatalf("a 29 s longer original = %#v, %v; want no lyrics", out.lyrics, err)
	}

	slowed := newQuery(pixyRequest())
	out, err = service.boidu(context.Background(), slowed)
	if err != nil || out.lyrics == nil || !out.lyrics.Synced || *out.lyrics.Lines[0].TimeMS != 4257 {
		t.Fatalf("slowed = %#v, %v; want 3.5 s scaled by 163/134", out.lyrics, err)
	}
}
