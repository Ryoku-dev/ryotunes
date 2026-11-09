package discord

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestBuildActivity(t *testing.T) {
	thumbnail := "https://lh3.googleusercontent.com/image=w60-h60"
	track := &model.PlaybackTrack{
		VideoID:   "abcdefghijk",
		Title:     "Song",
		Artists:   "Artist",
		Thumbnail: &thumbnail,
	}
	now := time.UnixMilli(1_700_000_120_000)
	got := buildActivity(now, "Ryotunes", playbackUpdate{
		track:    *track,
		hasTrack: true,
		position: 120,
		duration: 300,
		at:       now,
	})
	if got.Name != "Ryotunes" || got.Type != 2 || got.StatusDisplayType != "state" {
		t.Fatalf("identity fields = %#v", got)
	}
	if got.Details != "Song" || got.State != "Artist" {
		t.Fatalf("text fields = %#v", got)
	}
	if got.Timestamps.Start != 1_700_000_000_000 || got.Timestamps.End == nil || *got.Timestamps.End != 1_700_000_300_000 {
		t.Fatalf("timestamps = %#v", got.Timestamps)
	}
	if got.Assets.LargeImage != "https://lh3.googleusercontent.com/image=w512-h512" {
		t.Fatalf("large image = %q", got.Assets.LargeImage)
	}
	if got.Assets.SmallImage != brandImage || got.Assets.SmallText != "Ryotunes" {
		t.Fatalf("badge = %#v", got.Assets)
	}
	if len(got.Buttons) != 1 || got.Buttons[0].URL != "https://music.youtube.com/watch?v=abcdefghijk" {
		t.Fatalf("buttons = %#v", got.Buttons)
	}
}

func TestBuildActivityOmitsUnverifiableFields(t *testing.T) {
	localArt := "file:///home/user/Music/cover.jpg"
	track := &model.PlaybackTrack{VideoID: "LOCAL:/home/user/Music/a.flac", Title: "A", Thumbnail: &localArt}
	got := buildActivity(time.Unix(100, 0), "R", playbackUpdate{track: *track, hasTrack: true})
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"state":`)) || bytes.Contains(encoded, []byte(`"buttons":`)) {
		t.Fatalf("optional fields were not omitted: %s", encoded)
	}
	if got.Assets.LargeImage != brandImage || got.Assets.SmallImage != "" {
		t.Fatalf("local art leaked into assets: %#v", got.Assets)
	}
	if got.Name != "R\u2800" {
		t.Fatalf("short field was not padded: %q", got.Name)
	}
}

func TestIPCFrameEncoding(t *testing.T) {
	payload := []byte(`{"v":1,"client_id":"42"}`)
	frame := encodeFrame(opHandshake, payload)
	if len(frame) != len(payload)+8 {
		t.Fatalf("frame length = %d", len(frame))
	}
	if opcode := int32(binary.LittleEndian.Uint32(frame[:4])); opcode != opHandshake {
		t.Fatalf("opcode = %d", opcode)
	}
	if length := binary.LittleEndian.Uint32(frame[4:8]); length != uint32(len(payload)) {
		t.Fatalf("payload length = %d", length)
	}
	if !bytes.Equal(frame[8:], payload) {
		t.Fatalf("payload = %q", frame[8:])
	}
	opcode, decoded, err := readFrame(bytes.NewReader(frame))
	if err != nil || opcode != opHandshake || !bytes.Equal(decoded, payload) {
		t.Fatalf("decoded = %d %q %v", opcode, decoded, err)
	}
}
