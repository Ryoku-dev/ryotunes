package spectrum

import (
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

type runningCapture struct {
	command *exec.Cmd
	wait    <-chan error
}

type captureLifecycle struct {
	path       string
	static     bool
	invalidate func()
	start      func() (*runningCapture, error)
	onStart    func()

	mu          sync.Mutex
	lastWant    time.Time
	wantedUntil time.Time
	latest      time.Time
	active      bool
	running     bool
	available   bool
	needsWake   bool
	activation  uint64

	wake      chan struct{}
	close     chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func newCaptureLifecycle(path string, invalidate func(), start func() (*runningCapture, error), onStart func()) *captureLifecycle {
	capture := &captureLifecycle{
		path:       path,
		invalidate: invalidate,
		start:      start,
		onStart:    onStart,
		available:  path != "",
		wake:       make(chan struct{}, 1),
		close:      make(chan struct{}),
		done:       make(chan struct{}),
	}
	go capture.run()
	return capture
}

func newStaticCaptureLifecycle() *captureLifecycle {
	done := make(chan struct{})
	close(done)
	return &captureLifecycle{
		static:    true,
		available: true,
		wake:      make(chan struct{}, 1),
		close:     make(chan struct{}),
		done:      done,
	}
}

func (capture *captureLifecycle) Want(now time.Time) {
	capture.mu.Lock()
	wasWanted := now.Before(capture.wantedUntil)
	capture.lastWant = now
	capture.wantedUntil = now.Add(wantLifetime)
	capture.mu.Unlock()
	if !wasWanted {
		capture.notify()
	}
}

func (capture *captureLifecycle) SetActive(active bool) {
	capture.mu.Lock()
	changed := active != capture.active
	if changed && active {
		capture.activation++
	}
	capture.active = active
	if !active {
		capture.latest = time.Time{}
	}
	capture.mu.Unlock()
	if changed {
		capture.notify()
	}
}

func (capture *captureLifecycle) Live(now time.Time) bool {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.static {
		return capture.active && now.Before(capture.wantedUntil)
	}
	return capture.active && capture.running && !capture.latest.IsZero() && !capture.latest.Add(freshLifetime).Before(now)
}

func (capture *captureLifecycle) Publish(now time.Time, write func()) {
	capture.mu.Lock()
	if !capture.running || !capture.active {
		capture.mu.Unlock()
		return
	}
	write()
	capture.latest = now
	pulling := !capture.lastWant.IsZero() && now.Sub(capture.lastWant) < pullInvalidateWindow
	invalidate := capture.needsWake || !pulling
	capture.needsWake = false
	capture.mu.Unlock()
	if invalidate && capture.invalidate != nil {
		capture.invalidate()
	}
}

func (capture *captureLifecycle) Installed() bool {
	return capture.static || capture.path != ""
}

func (capture *captureLifecycle) Available() bool {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.available
}

func (capture *captureLifecycle) Close() {
	capture.closeOnce.Do(func() {
		close(capture.close)
		<-capture.done
	})
}

func (capture *captureLifecycle) notify() {
	select {
	case capture.wake <- struct{}{}:
	default:
	}
}

func (capture *captureLifecycle) run() {
	defer close(capture.done)
	var process *runningCapture
	var retryAt time.Time
	var failures []time.Time
	var activation uint64
	givenUp := false

	for {
		now := time.Now()
		active, wantedUntil, currentActivation := capture.demand()
		if currentActivation != activation {
			activation = currentActivation
			failures = failures[:0]
			givenUp = false
			capture.setAvailable(capture.Installed())
		}

		eligible := capture.Installed() && !givenUp && active && now.Before(wantedUntil)
		if process == nil {
			if !eligible {
				if capture.waitForWake() {
					return
				}
				continue
			}
			if now.Before(retryAt) {
				if capture.waitUntil(retryAt) {
					return
				}
				continue
			}
			started, err := capture.start()
			if err != nil {
				failures, givenUp = recordCaptureFailure(failures, time.Now(), failureWindow)
				if givenUp {
					capture.setAvailable(false)
				} else {
					retryAt = time.Now().Add(restartDelay)
				}
				continue
			}
			process = started
			if capture.onStart != nil {
				capture.onStart()
			}
			capture.setRunning(true)
			continue
		}

		if !eligible {
			capture.setRunning(false)
			stopCapture(process)
			process = nil
			continue
		}

		timer := time.NewTimer(time.Until(wantedUntil))
		select {
		case <-capture.close:
			stopTimer(timer)
			capture.setRunning(false)
			stopCapture(process)
			return
		case <-capture.wake:
			stopTimer(timer)
		case <-timer.C:
		case <-process.wait:
			stopTimer(timer)
			capture.setRunning(false)
			process = nil
			failures, givenUp = recordCaptureFailure(failures, time.Now(), failureWindow)
			if givenUp {
				capture.setAvailable(false)
			} else {
				retryAt = time.Now().Add(restartDelay)
			}
		}
	}
}

func (capture *captureLifecycle) demand() (bool, time.Time, uint64) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.active, capture.wantedUntil, capture.activation
}

func (capture *captureLifecycle) setRunning(running bool) {
	capture.mu.Lock()
	capture.running = running
	capture.needsWake = running
	if !running {
		capture.latest = time.Time{}
	}
	capture.mu.Unlock()
}

func (capture *captureLifecycle) setAvailable(available bool) {
	capture.mu.Lock()
	capture.available = available
	capture.mu.Unlock()
}

func recordCaptureFailure(failures []time.Time, now time.Time, window time.Duration) ([]time.Time, bool) {
	first := 0
	for first < len(failures) && now.Sub(failures[first]) > window {
		first++
	}
	failures = append(failures[first:], now)
	return failures, len(failures) >= failureLimit
}

func (capture *captureLifecycle) waitForWake() bool {
	select {
	case <-capture.close:
		return true
	case <-capture.wake:
		return false
	}
}

func (capture *captureLifecycle) waitUntil(deadline time.Time) bool {
	timer := time.NewTimer(time.Until(deadline))
	defer stopTimer(timer)
	select {
	case <-capture.close:
		return true
	case <-capture.wake:
		return false
	case <-timer.C:
		return false
	}
}

func startCapture(command *exec.Cmd, read func()) (*runningCapture, error) {
	if err := command.Start(); err != nil {
		return nil, err
	}
	wait := make(chan error, 1)
	go func() {
		wait <- command.Wait()
	}()
	go read()
	return &runningCapture{command: command, wait: wait}, nil
}

func stopCapture(process *runningCapture) {
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
