package state

import (
	"math"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	recommendationPoolMax = 300
	recommendationOutMax  = 12
	rankDecay             = 0.06
	weightRank            = 0.45
	weightArtist          = 0.40
	weightRecency         = 0.20
	weightPick            = 0.15
	weightFatigue         = 0.70
	weightMissingArt      = 0.30
	mmrLambda             = 0.72
)

var (
	artistHalfLife  = 14 * 24 * time.Hour
	fatigueHalfLife = 3 * 24 * time.Hour
)

type recommendationCandidate struct {
	item          model.BrowseItem
	rank          int
	relevance     float64
	artist        string
	kind          string
	maxSimilarity float64
	selected      bool
}

type recommendationIndex struct {
	recentAt    map[string]int64
	artists     map[string]ArtistStat
	byName      map[string]ArtistStat
	pickIDs     map[string]struct{}
	pickArtists map[string]struct{}
	maxCount    int
}

// PickedForYou ranks the active provider feed using on-device personal signals.
func PickedForYou(feed model.Feed, personal Personal) []model.BrowseItem {
	return PickedForYouAt(feed, personal, time.Now())
}

// PickedForYouAt is the deterministic clock-injected form of PickedForYou.
func PickedForYouAt(feed model.Feed, personal Personal, now time.Time) []model.BrowseItem {
	provider := feedProvider(feed)
	index := recommendationPersonalIndex(personal, provider)
	pool := recommendationPool(feed, provider, index.pickIDs)
	for i := range pool {
		scoreRecommendation(&pool[i], index, now)
	}
	chosen := rerankRecommendations(pool, recommendationOutMax)
	out := make([]model.BrowseItem, len(chosen))
	for i := range chosen {
		out[i] = chosen[i].item
	}
	return out
}

func feedProvider(feed model.Feed) model.Provider {
	for _, section := range feed.Sections {
		for _, item := range section.Items {
			if item.ID != "" && !model.IsRyotunesOwn(item.ID) {
				if provider := model.ProviderOf(item.ID); provider != "" {
					return provider
				}
			}
		}
	}
	return model.ProviderYouTube
}

func recommendationPersonalIndex(personal Personal, provider model.Provider) recommendationIndex {
	index := recommendationIndex{
		recentAt: make(map[string]int64), artists: make(map[string]ArtistStat),
		byName: make(map[string]ArtistStat), pickIDs: make(map[string]struct{}),
		pickArtists: make(map[string]struct{}),
	}
	for id, recent := range personal.Recent {
		if model.ProviderOf(id) == provider {
			index.recentAt[id] = recent.At
		}
	}
	for key, artist := range personal.Artists {
		if artist.Count <= 0 || model.ProviderOf(key) != provider {
			continue
		}
		index.artists[key] = artist
		name := strings.ToLower(FirstArtist(artist.Name))
		if prior, ok := index.byName[name]; name != "" && (!ok || artist.Count > prior.Count) {
			index.byName[name] = artist
		}
		if artist.Count > index.maxCount {
			index.maxCount = artist.Count
		}
	}
	for _, pick := range personal.Picks {
		index.pickIDs[pick.ID] = struct{}{}
		if model.ProviderOf(pick.ID) != provider {
			continue
		}
		if key := pickAffinityKey(pick); key != "" {
			index.pickArtists[key] = struct{}{}
		}
	}
	return index
}

func recommendationPool(feed model.Feed, provider model.Provider, picked map[string]struct{}) []recommendationCandidate {
	pool := make([]recommendationCandidate, 0, min(recommendationPoolMax, 64))
	seen := make(map[string]struct{})
	for _, section := range feed.Sections {
		for _, item := range section.Items {
			if len(pool) == recommendationPoolMax {
				return pool
			}
			if !usableRecommendation(item) {
				continue
			}
			if _, exists := seen[item.ID]; exists {
				continue
			}
			seen[item.ID] = struct{}{}
			if model.ProviderOf(item.ID) != provider {
				continue
			}
			if _, exists := picked[item.ID]; exists {
				continue
			}
			pool = append(pool, recommendationCandidate{item: item, rank: len(pool)})
		}
	}
	return pool
}

func usableRecommendation(item model.BrowseItem) bool {
	if item.ID == "" || strings.TrimSpace(item.Title) == "" {
		return false
	}
	switch item.Kind {
	case "song", "album", "playlist", "artist":
		return true
	default:
		return false
	}
}

func scoreRecommendation(candidate *recommendationCandidate, index recommendationIndex, now time.Time) {
	prior := 1 / (1 + rankDecay*float64(candidate.rank))
	key := itemAffinityKey(candidate.item)
	artist, found := index.artists[key]
	if !found && candidate.item.Subtitle != nil {
		artist, found = index.byName[strings.ToLower(FirstArtist(*candidate.item.Subtitle))]
	}
	affinity, recency := 0.0, 0.0
	if found && index.maxCount > 0 {
		affinity = clamp01(float64(artist.Count) / float64(index.maxCount))
		if artist.LastPlayedAt > 0 {
			recency = decaySignal(now.UnixMilli()-artist.LastPlayedAt, artistHalfLife)
		}
	}
	pick := 0.0
	if _, exists := index.pickArtists[key]; key != "" && exists {
		pick = 1
	}
	fatigue := 0.0
	if at, exists := index.recentAt[candidate.item.ID]; exists {
		fatigue = decaySignal(now.UnixMilli()-at, fatigueHalfLife)
	}
	missingArt := 1.0
	if candidate.item.Thumbnail != nil && *candidate.item.Thumbnail != "" {
		missingArt = 0
	}
	candidate.relevance = weightRank*prior + weightArtist*affinity + weightRecency*recency + weightPick*pick - weightFatigue*fatigue - weightMissingArt*missingArt
	if found && artist.Name != "" {
		candidate.artist = strings.ToLower(FirstArtist(artist.Name))
	} else {
		candidate.artist = strings.ToLower(key)
	}
	candidate.kind = candidate.item.Kind
}

func rerankRecommendations(pool []recommendationCandidate, limit int) []recommendationCandidate {
	chosen := make([]recommendationCandidate, 0, min(limit, len(pool)))
	for len(chosen) < limit && len(chosen) < len(pool) {
		var previous *recommendationCandidate
		if len(chosen) != 0 {
			previous = &chosen[len(chosen)-1]
		}
		best := -1
		bestScore := 0.0
		for i := range pool {
			candidate := &pool[i]
			if candidate.selected {
				continue
			}
			if previous != nil {
				candidate.maxSimilarity = math.Max(candidate.maxSimilarity, recommendationSimilarity(*candidate, *previous))
			}
			score := mmrLambda*candidate.relevance - (1-mmrLambda)*candidate.maxSimilarity
			if best < 0 || score > bestScore || (score == bestScore && recommendationBefore(*candidate, pool[best])) {
				best = i
				bestScore = score
			}
		}
		if best < 0 {
			break
		}
		pool[best].selected = true
		chosen = append(chosen, pool[best])
	}
	return chosen
}

func recommendationSimilarity(left, right recommendationCandidate) float64 {
	if left.artist != "" && left.artist == right.artist {
		return 1
	}
	if left.kind != "" && left.kind == right.kind {
		return 0.28
	}
	return 0
}

func recommendationBefore(left, right recommendationCandidate) bool {
	if left.relevance != right.relevance {
		return left.relevance > right.relevance
	}
	if left.rank != right.rank {
		return left.rank < right.rank
	}
	return left.item.ID < right.item.ID
}

func itemAffinityKey(item model.BrowseItem) string {
	for _, artist := range item.ArtistRuns {
		if artist.ID != nil && *artist.ID != "" {
			return *artist.ID
		}
	}
	if item.Kind == "artist" {
		return item.ID
	}
	if item.Subtitle != nil {
		return FirstArtist(*item.Subtitle)
	}
	return ""
}

func pickAffinityKey(pick Pick) string {
	if pick.ArtistID != nil && *pick.ArtistID != "" {
		return *pick.ArtistID
	}
	return itemAffinityKey(pick.BrowseItem)
}

func decaySignal(elapsedMS int64, halfLife time.Duration) float64 {
	if elapsedMS <= 0 {
		return 1
	}
	return clamp01(math.Pow(2, -float64(elapsedMS)/float64(halfLife.Milliseconds())))
}

func clamp01(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}
