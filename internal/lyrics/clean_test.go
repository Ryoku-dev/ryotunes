package lyrics

import "testing"

func TestNewQueryCleansUploads(t *testing.T) {
	album := "Metallica"
	tests := []struct {
		name        string
		title       string
		artists     string
		album       *string
		wantTitle   string
		wantArtists string
		wantVariant bool
	}{
		{"artist prefix on another channel, slowed", "PIXY - LEGACY (slowed + reverb)", "GAJ3K", nil, "LEGACY", "PIXY", true},
		{"artist prefix after an em dash", "PIXY \u2014 LEGACY [Official Audio]", "GAJ3K", nil, "LEGACY", "PIXY", false},
		{"artist prefix on the artist's channel", "Metallica - Enter Sandman (Official Music Video)", "Metallica", nil, "Enter Sandman", "Metallica", false},
		{"remaster bracket", "Wherever I May Roam (Remastered)", "Metallica", &album, "Wherever I May Roam", "Metallica", false},
		{"remaster segment", "Wherever I May Roam - Remastered 2021", "Metallica", &album, "Wherever I May Roam", "Metallica", false},
		{"topic channel", "Blinding Lights", "The Weeknd - Topic", nil, "Blinding Lights", "The Weeknd", false},
		{"vevo channel", "Bad Romance (Official Music Video)", "LadyGagaVEVO", nil, "Bad Romance", "LadyGaga", false},
		{"unbracketed featuring and lyrics tag", "Save Your Tears ft. Ariana Grande [Lyrics]", "The Weeknd", nil, "Save Your Tears", "The Weeknd", false},
		{"bracketed featuring and audio tag", "Heat Waves (feat. Someone) [Audio]", "Glass Animals", nil, "Heat Waves", "Glass Animals", false},
		{"sped up upload by a fan channel", "Glass Animals - Heat Waves (sped up)", "nightcore channel", nil, "Heat Waves", "Glass Animals", true},
		{"variant as a trailing segment", "Sweater Weather - slowed + reverb", "The Neighbourhood", nil, "Sweater Weather", "The Neighbourhood", true},
		{"variant words after the title", "Industry Baby slowed reverb", "Lil Nas X", nil, "Industry Baby", "Lil Nas X", true},
		{"nightcore", "Song (Nightcore)", "Band", nil, "Song", "Band", true},
		{"8D", "Song [8D AUDIO]", "Band", nil, "Song", "Band", true},
		{"daycore", "Song (daycore)", "Band", nil, "Song", "Band", true},
		{"reverb", "Song (Reverb)", "Band", nil, "Song", "Band", true},
		{"lenticular brackets", "【MV】Song", "Band", nil, "Song", "Band", false},
		{"pipe decoration", "Song | Official Video", "Band", nil, "Song", "Band", false},
		{"plain song", "Never Gonna Give You Up", "Rick Astley", nil, "Never Gonna Give You Up", "Rick Astley", false},
		{"a title that is only tempo words", "Sped Up", "Someone", nil, "Sped Up", "Someone", false},
		{"live is a different recording", "Song (Live)", "Band", nil, "Song (Live)", "Band", false},
		{"remix is a different recording", "Song (Remix)", "Band", nil, "Song (Remix)", "Band", false},
		{"catalogue song with a dash keeps its title", "Act II - The Future", "Artist", &album, "Act II - The Future", "Artist", false},
		{"decoration only keeps the raw title", "(Official Video)", "Band", nil, "(Official Video)", "Band", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			duration := 200.0
			q := newQuery(Request{VideoID: "video", Title: test.title, Artists: test.artists, Album: test.album, Duration: &duration})
			if q.Title != test.wantTitle || q.Artists != test.wantArtists || q.Variant != test.wantVariant {
				t.Fatalf("newQuery(%q, %q) = (%q, %q, variant %v), want (%q, %q, variant %v)",
					test.title, test.artists, q.Title, q.Artists, q.Variant, test.wantTitle, test.wantArtists, test.wantVariant)
			}
			if q.Duration == nil || *q.Duration != duration || q.Album != test.album {
				t.Fatalf("newQuery dropped album or duration: %#v", q)
			}
		})
	}
}

func TestQueryMatchesCleanedCandidateNames(t *testing.T) {
	tests := []struct {
		q       query
		title   string
		artists string
		want    bool
	}{
		{query{Title: "Wherever I May Roam", Artists: "Metallica"}, "Wherever I May Roam (Remastered)", "Metallica", true},
		{query{Title: "Bad Romance", Artists: "LadyGaga"}, "Bad Romance", "Lady Gaga", true},
		{query{Title: "LEGACY", Artists: "PIXY"}, "Legacy", "Someone, PIXY", true},
		{query{Title: "LEGACY", Artists: "PIXY"}, "Legacy", "Other Artist", false},
		{query{Title: "LEGACY", Artists: "PIXY"}, "Pimp", "PIXY", false},
		{query{Title: "Shape of You", Artists: "Ed Sheeran"}, "Shape of You (DJ Edit)", "Ed Sheeran", false},
		{query{Title: "Song", Artists: ""}, "Song", "Anyone", true},
		{query{Title: "Song", Artists: "陈奕迅"}, "Song", "陈奕迅、王菲", true},
	}
	for _, test := range tests {
		if got := test.q.matches(test.title, test.artists); got != test.want {
			t.Errorf("%#v.matches(%q, %q) = %v, want %v", test.q, test.title, test.artists, got, test.want)
		}
	}
}
