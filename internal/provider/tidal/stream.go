package tidal

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ryoku-dev/ryotunes/internal/model"
	baseprovider "github.com/ryoku-dev/ryotunes/internal/provider"
)

type quality int

const (
	qualityLow quality = iota
	qualityHigh
	qualityLossless
	qualityHiResLossless
)

func (q quality) String() string {
	switch q {
	case qualityLow:
		return "LOW"
	case qualityHigh:
		return "HIGH"
	case qualityLossless:
		return "LOSSLESS"
	default:
		return "HI_RES_LOSSLESS"
	}
}

func parseQuality(value string) (quality, bool) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "LOW":
		return qualityLow, true
	case "HIGH":
		return qualityHigh, true
	case "LOSSLESS":
		return qualityLossless, true
	case "HI_RES", "HI_RES_LOSSLESS":
		return qualityHiResLossless, true
	default:
		return qualityLow, false
	}
}

func ceilingForEntitlement(entitlement string) quality {
	if strings.EqualFold(entitlement, "hires") {
		return qualityHiResLossless
	}
	return qualityLossless
}

func qualityFor(requested model.Quality, ceiling quality) quality {
	want, ok := parseQuality(string(requested))
	if !ok || requested == model.QualityAuto || requested == model.QualityHigh {
		want = ceiling
	}
	if want > ceiling {
		return ceiling
	}
	return want
}

type playbackInfo struct {
	ManifestMimeType  string  `json:"manifestMimeType"`
	Manifest          string  `json:"manifest"`
	AudioQuality      *string `json:"audioQuality"`
	AssetPresentation *string `json:"assetPresentation"`
	SampleRate        *uint32 `json:"sampleRate"`
	BitDepth          *uint32 `json:"bitDepth"`
}

type btsManifest struct {
	URLs           []string `json:"urls"`
	Codecs         *string  `json:"codecs"`
	EncryptionType *string  `json:"encryptionType"`
	KeyID          *string  `json:"keyId"`
}

func streamFromPlaybackInfo(info playbackInfo) (model.Stream, error) {
	if info.AssetPresentation != nil && !strings.EqualFold(*info.AssetPresentation, "FULL") {
		return model.Stream{}, unavailableError(fmt.Sprintf("tidal returned a %s asset instead of the full track", *info.AssetPresentation))
	}
	raw, err := base64.StdEncoding.DecodeString(info.Manifest)
	if err != nil {
		return model.Stream{}, fmt.Errorf("tidal manifest could not be read: manifest is not base64")
	}
	ok := false
	if info.AudioQuality != nil {
		_, ok = parseQuality(*info.AudioQuality)
	}
	if !ok {
		return model.Stream{}, fmt.Errorf("tidal manifest could not be read: response named no audio quality")
	}
	switch info.ManifestMimeType {
	case "application/vnd.tidal.bts":
		var manifest btsManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return model.Stream{}, fmt.Errorf("tidal manifest could not be read: direct manifest is not valid json")
		}
		encryption := "NONE"
		if manifest.EncryptionType != nil {
			encryption = *manifest.EncryptionType
		}
		if (!strings.EqualFold(strings.TrimSpace(encryption), "NONE") && strings.TrimSpace(encryption) != "") || (manifest.KeyID != nil && strings.TrimSpace(*manifest.KeyID) != "") {
			return model.Stream{}, unavailableError(fmt.Sprintf("tidal returned a protected asset (%s); this build does not play protected streams", encryption))
		}
		for _, rawURL := range manifest.URLs {
			if strings.TrimSpace(rawURL) == "" {
				continue
			}
			codec := ""
			if manifest.Codecs != nil {
				codec = *manifest.Codecs
			}
			return model.Stream{URL: rawURL, Codec: codec}, nil
		}
		return model.Stream{}, fmt.Errorf("tidal manifest could not be read: direct manifest carried no url")
	case "application/dash+xml":
		if !utf8.Valid(raw) {
			return model.Stream{}, fmt.Errorf("tidal manifest could not be read: presentation is not utf-8")
		}
		if bytes.Contains(raw, []byte("ContentProtection")) {
			return model.Stream{}, unavailableError("tidal returned a protected asset (dash content protection); this build does not play protected streams")
		}
		codec := xmlAttribute(raw, "codecs")
		return model.Stream{URL: "data:application/dash+xml;base64," + info.Manifest, Codec: codec}, nil
	default:
		return model.Stream{}, fmt.Errorf("tidal manifest could not be read: unknown manifest type %s", info.ManifestMimeType)
	}
}

func xmlAttribute(xml []byte, name string) string {
	needle := []byte(name + `="`)
	start := bytes.Index(xml, needle)
	if start < 0 {
		return ""
	}
	rest := xml[start+len(needle):]
	end := bytes.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(string(rest[:end]))
}

type unavailableError string

func (e unavailableError) Error() string      { return string(e) }
func (unavailableError) Is(target error) bool { return target == baseprovider.ErrUnavailable }
