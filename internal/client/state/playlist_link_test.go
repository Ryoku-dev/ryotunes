package state

import "testing"

func TestParsePlaylistLink(t *testing.T) {
	tests := []struct {
		name, input, id, reason, message string
	}{
		{name: "playlist", input: "https://music.youtube.com/playlist?list=PLabc_123", id: "VLPLabc_123"},
		{name: "bare share", input: "youtu.be/video?list=VLPLabc", id: "VLPLabc"},
		{name: "browse", input: "https://music.youtube.com/browse/VLPLxyz", id: "VLPLxyz"},
		{name: "album", input: "https://music.youtube.com/browse/MPREalbum", reason: "no-playlist", message: "That link is an album, not a playlist."},
		{name: "artist", input: "https://music.youtube.com/browse/UCartist", reason: "no-playlist", message: "That link is an artist, not a playlist."},
		{name: "ambiguous", input: "https://youtube.com/watch?list=one&list=two", reason: "ambiguous", message: "This link points at more than one playlist."},
		{name: "host", input: "https://youtube.com.evil.test/playlist?list=PLx", reason: "host", message: "Only YouTube and YouTube Music links are supported."},
		{name: "scheme", input: "file://youtube.com/playlist?list=PLx", reason: "scheme", message: "Only http and https links are supported."},
		{name: "credentials", input: "https://user@youtube.com/playlist?list=PLx", reason: "invalid", message: "Paste a public share link without login details."},
		{name: "bad id", input: "https://youtube.com/playlist?list=%21", reason: "bad-id", message: "That does not look like a valid playlist id."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ParsePlaylistLink(test.input)
			if test.id != "" {
				if !got.OK || got.ID != test.id {
					t.Fatalf("ParsePlaylistLink(%q) = %#v", test.input, got)
				}
				return
			}
			if got.OK || got.Reason != test.reason || got.Message != test.message {
				t.Fatalf("ParsePlaylistLink(%q) = %#v", test.input, got)
			}
		})
	}
}
