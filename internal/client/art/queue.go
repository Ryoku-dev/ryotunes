package art

import (
	"container/heap"
	"context"
	"sync"
	"sync/atomic"
)

type jobKind uint8

const (
	artworkJob jobKind = iota
	paletteJob
	// refreshJob refetches a cover that is already shown from an old disk entry.
	refreshJob
)

type loadJob struct {
	kind       jobKind
	key        cacheKey
	generation uint64
	priority   uint64
	requested  atomic.Uint64
}

type jobHeap []*loadJob

func (h jobHeap) Len() int { return len(h) }
func (h jobHeap) Less(i, j int) bool {
	if h[i].priority == h[j].priority {
		return h[i].generation > h[j].generation
	}
	return h[i].priority > h[j].priority
}
func (h jobHeap) Swap(i, j int)   { h[i], h[j] = h[j], h[i] }
func (h *jobHeap) Push(value any) { *h = append(*h, value.(*loadJob)) }
func (h *jobHeap) Pop() any {
	old := *h
	last := len(old) - 1
	job := old[last]
	old[last] = nil
	*h = old[:last]
	return job
}

type jobQueue struct {
	mu     sync.Mutex
	ready  chan struct{}
	closed bool
	jobs   jobHeap
}

func newJobQueue() *jobQueue {
	return &jobQueue{ready: make(chan struct{}, 1)}
}

func (q *jobQueue) push(job *loadJob) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	job.priority = job.requested.Load()
	heap.Push(&q.jobs, job)
	select {
	case q.ready <- struct{}{}:
	default:
	}
	return true
}

func (q *jobQueue) promote(job *loadJob, priority uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for index, queued := range q.jobs {
		if queued == job {
			job.priority = priority
			heap.Fix(&q.jobs, index)
			return true
		}
	}
	return false
}

func (q *jobQueue) pop(ctx context.Context) (*loadJob, bool) {
	for {
		q.mu.Lock()
		if len(q.jobs) > 0 {
			job := heap.Pop(&q.jobs).(*loadJob)
			if len(q.jobs) > 0 {
				select {
				case q.ready <- struct{}{}:
				default:
				}
			}
			q.mu.Unlock()
			return job, true
		}
		closed := q.closed
		q.mu.Unlock()
		if closed {
			return nil, false
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-q.ready:
		}
	}
}

func (q *jobQueue) close() {
	q.mu.Lock()
	q.closed = true
	for len(q.jobs) > 0 {
		heap.Pop(&q.jobs)
	}
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
}
