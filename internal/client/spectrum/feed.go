package spectrum

import (
	"bufio"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	wantLifetime         = 750 * time.Millisecond
	freshLifetime        = 250 * time.Millisecond
	pullInvalidateWindow = 33 * time.Millisecond
	restartDelay         = 1200 * time.Millisecond
	stopGrace            = 500 * time.Millisecond
	failureWindow        = 30 * time.Second
	failureLimit         = 3
)

type Feed struct {
	invalidate func()
	cavaPath   string
	static     bool

	mu           sync.Mutex
	levels       [Bands]float32
	snapshot     [Bands]float32
	latest       time.Time
	lastWant     time.Time
	wantedUntil  time.Time
	active       bool
	running      bool
	available    bool
	needsWake    bool
	snapshotLive bool
	activation   uint64

	wake      chan struct{}
	close     chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

type runningCava struct {
	command *exec.Cmd
	wait    <-chan error
}

func New(invalidate func()) *Feed {
	path, _ := exec.LookPath("cava")
	feed := &Feed{
		invalidate: invalidate,
		cavaPath:   path,
		available:  path != "",
		wake:       make(chan struct{}, 1),
		close:      make(chan struct{}),
		done:       make(chan struct{}),
	}
	go feed.run()
	return feed
}

func NewStatic(levels []float32) *Feed {
	done := make(chan struct{})
	close(done)
	feed := &Feed{
		static:    true,
		available: true,
		wake:      make(chan struct{}, 1),
		close:     make(chan struct{}),
		done:      done,
	}
	for index := range min(len(levels), Bands) {
		feed.levels[index] = max(0, min(levels[index], 1))
	}
	return feed
}

func (f *Feed) Want(now time.Time) {
	f.mu.Lock()
	f.lastWant = now
	f.wantedUntil = now.Add(wantLifetime)
	if f.static {
		f.latest = now
	}
	f.mu.Unlock()
	f.notify()
}

func (f *Feed) SetActive(active bool) {
	f.mu.Lock()
	if active && !f.active {
		f.activation++
	}
	f.active = active
	if !active {
		f.latest = time.Time{}
	}
	f.mu.Unlock()
	f.notify()
}

func (f *Feed) Levels(now time.Time) ([]float32, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	live := f.active && ((f.static && now.Before(f.wantedUntil)) ||
		(!f.static && f.running && !f.latest.IsZero() && !f.latest.Add(freshLifetime).Before(now)))
	if live {
		copy(f.snapshot[:], f.levels[:])
		f.snapshotLive = true
	} else if f.snapshotLive {
		clear(f.snapshot[:])
		f.snapshotLive = false
	}
	return f.snapshot[:], live
}

func (f *Feed) Available() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.available
}

func (f *Feed) Close() {
	f.closeOnce.Do(func() {
		close(f.close)
		<-f.done
	})
}

func (f *Feed) notify() {
	select {
	case f.wake <- struct{}{}:
	default:
	}
}

func (f *Feed) run() {
	defer close(f.done)
	var process *runningCava
	var retryAt time.Time
	var failures []time.Time
	var activation uint64
	givenUp := false

	for {
		now := time.Now()
		active, wantedUntil, currentActivation := f.demand()
		if currentActivation != activation {
			activation = currentActivation
			failures = failures[:0]
			givenUp = false
			f.setAvailable(f.cavaPath != "")
		}

		eligible := f.cavaPath != "" && !givenUp && active && now.Before(wantedUntil)
		if process == nil {
			if !eligible {
				if f.waitForWake() {
					return
				}
				continue
			}
			if now.Before(retryAt) {
				if f.waitUntil(retryAt) {
					return
				}
				continue
			}
			started, err := f.start()
			if err != nil {
				failures, givenUp = f.failedStart(failures, time.Now())
				if givenUp {
					f.setAvailable(false)
				} else {
					retryAt = time.Now().Add(restartDelay)
				}
				continue
			}
			process = started
			f.setRunning(true)
			continue
		}

		if !eligible {
			f.setRunning(false)
			stop(process)
			process = nil
			continue
		}

		timer := time.NewTimer(time.Until(wantedUntil))
		select {
		case <-f.close:
			stopTimer(timer)
			f.setRunning(false)
			stop(process)
			return
		case <-f.wake:
			stopTimer(timer)
		case <-timer.C:
		case <-process.wait:
			stopTimer(timer)
			f.setRunning(false)
			process = nil
			failures, givenUp = f.failedStart(failures, time.Now())
			if givenUp {
				f.setAvailable(false)
			} else {
				retryAt = time.Now().Add(restartDelay)
			}
		}
	}
}

func (f *Feed) start() (*runningCava, error) {
	config, err := writeConfig(resolveSink())
	if err != nil {
		return nil, err
	}
	command := exec.Command(f.cavaPath, "-p", config)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	wait := make(chan error, 1)
	go func() {
		wait <- command.Wait()
	}()
	go f.readFrames(stdout)
	return &runningCava{command: command, wait: wait}, nil
}

func (f *Feed) readFrames(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	var frame [Bands]float32
	for scanner.Scan() {
		if !parseFrame(scanner.Bytes(), frame[:]) {
			continue
		}
		now := time.Now()
		f.mu.Lock()
		publish := f.running && f.active
		invalidate := false
		if publish {
			copy(f.levels[:], frame[:])
			f.latest = now
			pulling := !f.lastWant.IsZero() && now.Sub(f.lastWant) < pullInvalidateWindow
			invalidate = f.needsWake || !pulling
			f.needsWake = false
		}
		f.mu.Unlock()
		if invalidate && f.invalidate != nil {
			f.invalidate()
		}
	}
}

func (f *Feed) demand() (bool, time.Time, uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active, f.wantedUntil, f.activation
}

func (f *Feed) setRunning(running bool) {
	f.mu.Lock()
	f.running = running
	f.needsWake = running
	if !running {
		f.latest = time.Time{}
	}
	f.mu.Unlock()
}

func (f *Feed) setAvailable(available bool) {
	f.mu.Lock()
	f.available = available
	f.mu.Unlock()
}

func (f *Feed) failedStart(failures []time.Time, now time.Time) ([]time.Time, bool) {
	first := 0
	for first < len(failures) && now.Sub(failures[first]) > failureWindow {
		first++
	}
	failures = append(failures[first:], now)
	return failures, len(failures) >= failureLimit
}

func (f *Feed) waitForWake() bool {
	select {
	case <-f.close:
		return true
	case <-f.wake:
		return false
	}
}

func (f *Feed) waitUntil(deadline time.Time) bool {
	timer := time.NewTimer(time.Until(deadline))
	defer stopTimer(timer)
	select {
	case <-f.close:
		return true
	case <-f.wake:
		return false
	case <-timer.C:
		return false
	}
}

func stop(process *runningCava) {
	_ = process.command.Process.Signal(syscall.SIGTERM)
	timer := time.NewTimer(stopGrace)
	defer stopTimer(timer)
	select {
	case <-process.wait:
		return
	case <-timer.C:
		_ = process.command.Process.Kill()
		<-process.wait
	}
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
