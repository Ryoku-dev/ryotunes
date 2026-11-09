package spotify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	"github.com/devgianlu/go-librespot/audio"
	extmetadatapb "github.com/devgianlu/go-librespot/proto/spotify/extendedmetadata"
	audiofilespb "github.com/devgianlu/go-librespot/proto/spotify/extendedmetadata/audiofiles"
	metadatapb "github.com/devgianlu/go-librespot/proto/spotify/metadata"
	"github.com/ryoku-dev/ryotunes/internal/model"
	providerapi "github.com/ryoku-dev/ryotunes/internal/provider"
)

const spotifyOggHeaderOffset int64 = 0xa7

type activeStream struct {
	token  string
	track  string
	reader io.ReaderAt
	size   int64
	closer io.Closer
}

// Resolve implements provider.Provider by exposing decrypted Ogg Vorbis on loopback.
func (p *Spotify) Resolve(ctx context.Context, track model.Track, quality model.Quality) (model.Stream, error) {
	p.closeActiveStream()
	sess, err := p.session(ctx)
	if err != nil {
		return model.Stream{}, err
	}
	id, bare, err := trackID(track.VideoID)
	if err != nil {
		return model.Stream{}, fmt.Errorf("%w: %v", providerapi.ErrUnavailable, err)
	}

	var files audiofilespb.AudioFilesExtensionResponse
	if err := sess.Spclient().ExtendedMetadataSimple(ctx, *id, audiofilespbExtensionKind(), &files); err != nil {
		return model.Stream{}, mapProviderError(err)
	}
	file, bitrate := chooseVorbis(files.GetFiles(), quality)
	if file == nil || len(file.GetFileId()) == 0 {
		return model.Stream{}, fmt.Errorf("%w: Spotify has no supported Ogg Vorbis file", providerapi.ErrUnavailable)
	}
	key, err := sess.AudioKey().Request(ctx, id.Id(), file.GetFileId())
	if err != nil {
		return model.Stream{}, mapProviderError(err)
	}
	storage, err := sess.Spclient().ResolveStorageInteractive(ctx, file.GetFileId(), file.Format, false)
	if err != nil {
		return model.Stream{}, mapProviderError(err)
	}
	urls := storage.GetCdnurl()
	if len(urls) == 0 {
		return model.Stream{}, fmt.Errorf("%w: Spotify returned no audio location", providerapi.ErrUnavailable)
	}
	raw, err := audio.NewHttpChunkedReader(p.liblog.WithField("track", bare), p.http, urls[0])
	if err != nil {
		return model.Stream{}, mapProviderError(err)
	}
	decryptor, err := audio.NewAesAudioDecryptor(raw, key)
	if err != nil {
		raw.Close()
		return model.Stream{}, mapProviderError(err)
	}
	if raw.Size() <= spotifyOggHeaderOffset {
		decryptor.Close()
		return model.Stream{}, fmt.Errorf("%w: Spotify audio is truncated", providerapi.ErrUnavailable)
	}
	token, err := randomToken()
	if err != nil {
		decryptor.Close()
		return model.Stream{}, err
	}
	active := &activeStream{token: token, track: bare, reader: decryptor, size: raw.Size() - spotifyOggHeaderOffset, closer: decryptor}
	p.streamMu.Lock()
	p.stream = active
	p.streamMu.Unlock()
	return model.Stream{URL: p.baseURL + "/stream/" + token + "/" + bare + ".ogg", Codec: "vorbis", Bitrate: bitrate}, nil
}

func audiofilespbExtensionKind() extmetadatapb.ExtensionKind {
	return extmetadatapb.ExtensionKind_AUDIO_FILES
}

func chooseVorbis(files []*audiofilespb.ExtendedAudioFile, quality model.Quality) (*metadatapb.AudioFile, int) {
	byFormat := make(map[metadatapb.AudioFile_Format]*metadatapb.AudioFile)
	for _, wrapped := range files {
		if wrapped != nil && wrapped.File != nil && wrapped.File.Format != nil {
			byFormat[wrapped.File.GetFormat()] = wrapped.File
		}
	}
	var order []metadatapb.AudioFile_Format
	switch quality {
	case model.QualityLow:
		order = []metadatapb.AudioFile_Format{metadatapb.AudioFile_OGG_VORBIS_96, metadatapb.AudioFile_OGG_VORBIS_160, metadatapb.AudioFile_OGG_VORBIS_320}
	case model.QualityHigh:
		order = []metadatapb.AudioFile_Format{metadatapb.AudioFile_OGG_VORBIS_320, metadatapb.AudioFile_OGG_VORBIS_160, metadatapb.AudioFile_OGG_VORBIS_96}
	default:
		order = []metadatapb.AudioFile_Format{metadatapb.AudioFile_OGG_VORBIS_160, metadatapb.AudioFile_OGG_VORBIS_320, metadatapb.AudioFile_OGG_VORBIS_96}
	}
	for _, format := range order {
		if file := byFormat[format]; file != nil {
			switch format {
			case metadatapb.AudioFile_OGG_VORBIS_320:
				return file, 320
			case metadatapb.AudioFile_OGG_VORBIS_160:
				return file, 160
			default:
				return file, 96
			}
		}
	}
	return nil, 0
}

func trackID(value string) (*librespot.SpotifyId, string, error) {
	bare := value
	if len(value) >= len("spotify:track:") && value[:len("spotify:track:")] == "spotify:track:" {
		bare = value[len("spotify:track:"):]
	}
	if !validBase62(bare) {
		return nil, "", fmt.Errorf("invalid Spotify track id %q", value)
	}
	id, err := librespot.SpotifyIdFromBase62(librespot.SpotifyIdTypeTrack, bare)
	if err != nil || id.Base62() != bare {
		return nil, "", fmt.Errorf("invalid Spotify track id %q", value)
	}
	return id, bare, nil
}

func (p *Spotify) startStreamServer() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start Spotify stream server: %w", err)
	}
	p.baseURL = "http://" + listener.Addr().String()
	p.server = &http.Server{Handler: http.HandlerFunc(p.serveStream), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := p.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			p.log.Error("Spotify stream server stopped", "error", err)
		}
	}()
	return nil
}

func (p *Spotify) serveStream(w http.ResponseWriter, r *http.Request) {
	token := path.Base(path.Dir(r.URL.Path))
	p.streamMu.RLock()
	active := p.stream
	if active == nil || token != active.token {
		p.streamMu.RUnlock()
		http.NotFound(w, r)
		return
	}
	reader, size, name := active.reader, active.size, active.track+".ogg"
	p.streamMu.RUnlock()
	serveSeekable(w, r, name, newOggReader(reader, size), size)
}

func newOggReader(reader io.ReaderAt, size int64) io.ReadSeeker {
	return io.NewSectionReader(reader, spotifyOggHeaderOffset, size)
}

func serveSeekable(w http.ResponseWriter, r *http.Request, name string, reader io.ReadSeeker, size int64) {
	w.Header().Set("Content-Type", "audio/ogg")
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, name, time.Time{}, sizedReadSeeker{ReadSeeker: reader, size: size})
}

type sizedReadSeeker struct {
	io.ReadSeeker
	size int64
}

// Seek implements io.Seeker while preserving the exposed stream size.
func (s sizedReadSeeker) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekEnd {
		return s.ReadSeeker.Seek(s.size+offset, io.SeekStart)
	}
	return s.ReadSeeker.Seek(offset, whence)
}

func (p *Spotify) closeActiveStream() {
	p.streamMu.Lock()
	old := p.stream
	p.stream = nil
	p.streamMu.Unlock()
	if old != nil {
		_ = old.closer.Close()
	}
}

func randomToken() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
