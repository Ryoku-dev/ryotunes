package downloads

import (
	"fmt"
	"sort"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

type downloadItem struct {
	job        *model.DownloadEntry
	collection *downloadCollection
}

type downloadCollection struct {
	key   string
	label string
	kind  string
	jobs  []model.DownloadEntry
}

type downloadStats struct {
	done, waiting, moving, failed, cancelled int
	average                                  int
}

func projectDownloads(jobs []model.DownloadEntry) (active, history []downloadItem, activeCount, queuedCount int) {
	activeJobs := make([]model.DownloadEntry, 0, len(jobs))
	historyJobs := make([]model.DownloadEntry, 0, len(jobs))
	for _, job := range jobs {
		switch job.Status {
		case model.DownloadStatusDownloading:
			activeCount++
			activeJobs = append(activeJobs, job)
		case model.DownloadStatusQueued:
			queuedCount++
			activeJobs = append(activeJobs, job)
		default:
			historyJobs = append(historyJobs, job)
		}
	}
	sort.SliceStable(activeJobs, func(i, j int) bool {
		if activeJobs[i].Status != activeJobs[j].Status {
			return activeJobs[i].Status == model.DownloadStatusDownloading
		}
		return activeJobs[i].CreatedAt < activeJobs[j].CreatedAt
	})
	sort.SliceStable(historyJobs, func(i, j int) bool {
		left, right := historyJobs[i].CreatedAt, historyJobs[j].CreatedAt
		if historyJobs[i].FinishedAt != nil {
			left = *historyJobs[i].FinishedAt
		}
		if historyJobs[j].FinishedAt != nil {
			right = *historyJobs[j].FinishedAt
		}
		return left > right
	})
	if len(historyJobs) > 200 {
		historyJobs = historyJobs[:200]
	}
	return collapseCollections(activeJobs, "active"), collapseCollections(historyJobs, "history"), activeCount, queuedCount
}

func collapseCollections(jobs []model.DownloadEntry, scope string) []downloadItem {
	counts := make(map[string]int)
	for i := range jobs {
		if jobs[i].Collection == nil || *jobs[i].Collection == "" {
			continue
		}
		counts[collectionKey(jobs[i])]++
	}
	items := make([]downloadItem, 0, len(jobs))
	groups := make(map[string]*downloadCollection)
	for i := range jobs {
		job := &jobs[i]
		key := collectionKey(*job)
		if key == "" || counts[key] < 2 {
			items = append(items, downloadItem{job: job})
			continue
		}
		group := groups[key]
		if group == nil {
			group = &downloadCollection{key: scope + ":" + key, label: *job.Collection}
			if job.CollectionKind != nil {
				group.kind = *job.CollectionKind
			}
			groups[key] = group
			items = append(items, downloadItem{collection: group})
		}
		group.jobs = append(group.jobs, *job)
	}
	return items
}

func collectionKey(job model.DownloadEntry) string {
	if job.Collection == nil || *job.Collection == "" {
		return ""
	}
	kind := ""
	if job.CollectionKind != nil {
		kind = *job.CollectionKind
	}
	return kind + "\x00" + *job.Collection
}

func statsFor(jobs []model.DownloadEntry) downloadStats {
	var stats downloadStats
	var progress int
	for _, job := range jobs {
		switch job.Status {
		case model.DownloadStatusCompleted:
			stats.done++
		case model.DownloadStatusDownloading:
			stats.moving++
			progress += job.Progress
		case model.DownloadStatusQueued:
			stats.waiting++
		case model.DownloadStatusFailed:
			stats.failed++
		case model.DownloadStatusCancelled:
			stats.cancelled++
		}
	}
	if stats.moving > 0 {
		stats.average = progress / stats.moving
	}
	return stats
}

func (s downloadStats) summary() string {
	parts := make([]string, 0, 5)
	if s.done > 0 {
		parts = append(parts, fmt.Sprintf("%d saved", s.done))
	}
	if s.moving > 0 {
		parts = append(parts, fmt.Sprintf("%d moving", s.moving))
	}
	if s.waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d waiting", s.waiting))
	}
	if s.failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", s.failed))
	}
	if s.cancelled > 0 {
		parts = append(parts, fmt.Sprintf("%d cancelled", s.cancelled))
	}
	if len(parts) == 0 {
		return "No downloads"
	}
	result := parts[0]
	for _, part := range parts[1:] {
		result += " · " + part
	}
	return result
}
