package queue

import (
	"slices"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func cloneTracks(items []model.Track) []model.Track {
	return append([]model.Track(nil), items...)
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cleanQueueFlags(item model.Track) model.Track {
	item.Queued = false
	item.QueuedEnd = false
	item.QueuedFrom = nil
	item.QueuedBy = nil
	item.Autoplay = false
	return item
}

func upcomingManual(items []model.Track, current int) []model.Track {
	if current < -1 {
		current = -1
	}
	var out []model.Track
	for _, item := range items[min(current+1, len(items)):] {
		if item.Queued || item.QueuedEnd {
			out = append(out, item)
		}
	}
	return out
}

func manualBoundary(items []model.Track, current int, playNext bool) int {
	at := min(max(current+1, 0), len(items))
	for at < len(items) {
		item := items[at]
		if playNext {
			if !item.Queued {
				break
			}
		} else if !item.Queued && !item.QueuedEnd {
			break
		}
		at++
	}
	return at
}

func (q *Queue) prepareManualLocked(items []model.Track, from *model.QueueContext, next bool) []model.Track {
	items = cloneTracks(items)
	prevent := q.hooks != nil && q.hooks.PreventDuplicates()
	var seen map[string]struct{}
	if prevent {
		seen = make(map[string]struct{}, len(items))
	}
	out := items[:0]
	for _, item := range items {
		if item.VideoID == "" {
			continue
		}
		if prevent {
			if _, exists := seen[item.VideoID]; exists {
				continue
			}
			seen[item.VideoID] = struct{}{}
		}
		item.Queued = next
		item.QueuedEnd = !next
		item.Autoplay = false
		if from == nil {
			item.QueuedFrom = nil
		} else {
			value := string(*from)
			item.QueuedFrom = &value
		}
		out = append(out, item)
	}
	if !prevent {
		return out
	}
	for i := len(q.state.items) - 1; i > q.state.current; i-- {
		item := q.state.items[i]
		if (!item.Queued && !item.QueuedEnd) || !containsID(out, item.VideoID) {
			continue
		}
		q.state.items = slices.Delete(q.state.items, i, i+1)
	}
	return out
}

func containsID(items []model.Track, id string) bool {
	return slices.ContainsFunc(items, func(item model.Track) bool { return item.VideoID == id })
}

func (q *Queue) shuffleUpcomingLocked() {
	current := q.state.current
	start := min(current+1, len(q.state.items))
	for start < len(q.state.items) && (q.state.items[start].Queued || q.state.items[start].QueuedEnd) {
		start++
	}
	for i := current + 1; i < start; {
		from := q.state.items[i].QueuedFrom
		if from == nil {
			i++
			continue
		}
		end := i + 1
		for end < start && q.state.items[end].QueuedFrom != nil && *q.state.items[end].QueuedFrom == *from {
			end++
		}
		q.rng.Shuffle(end-i, func(a, b int) { q.state.items[i+a], q.state.items[i+b] = q.state.items[i+b], q.state.items[i+a] })
		i = end
	}
	if start >= len(q.state.items) {
		return
	}
	q.rng.Shuffle(len(q.state.items)-start, func(a, b int) {
		q.state.items[start+a], q.state.items[start+b] = q.state.items[start+b], q.state.items[start+a]
	})
	slices.SortStableFunc(q.state.items[start:], func(a, b model.Track) int {
		switch {
		case a.Autoplay == b.Autoplay:
			return 0
		case a.Autoplay:
			return 1
		default:
			return -1
		}
	})
}

func (q *Queue) unshuffleLocked() {
	orig := q.state.shuffleOrig
	playingID := q.state.items[q.state.current].VideoID
	heard := make(map[string]struct{}, q.state.current+1)
	for _, item := range q.state.items[:q.state.current+1] {
		heard[item.VideoID] = struct{}{}
	}
	index := slices.IndexFunc(orig, func(item model.Track) bool { return item.VideoID == playingID })
	if index < 0 {
		index = min(q.state.current, len(orig)-1)
	}
	items := cloneTracks(orig)
	for i := len(items) - 1; i > index; i-- {
		if _, ok := heard[items[i].VideoID]; ok {
			items = slices.Delete(items, i, i+1)
		}
	}
	tail := cloneTracks(items[index+1:])
	items = items[:index+1]
	for _, item := range tail {
		if item.Queued || item.QueuedEnd {
			items = append(items, item)
		}
	}
	for _, item := range tail {
		if !item.Queued && !item.QueuedEnd {
			items = append(items, item)
		}
	}
	q.state.items = items
	q.state.current = index
	q.state.playedFrom = index
	q.state.shuffleOrig = nil
}

func (q *Queue) nextIndexLocked(manual bool) (int, bool) {
	if len(q.state.items) == 0 {
		return 0, false
	}
	if q.state.repeat == model.RepeatOne && !manual {
		return q.state.current, true
	}
	if q.state.current+1 < len(q.state.items) {
		return q.state.current + 1, true
	}
	if q.state.repeat == model.RepeatAll || (manual && q.state.repeat == model.RepeatOne) {
		return 0, true
	}
	return 0, false
}
