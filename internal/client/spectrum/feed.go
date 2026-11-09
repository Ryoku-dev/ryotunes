package spectrum

import (
	"bufio"
	"io"
	"os/exec"
	"sync"
	"time"
)

type Feed struct {
	cavaPath string
	capture  *captureLifecycle

	mu           sync.Mutex
	levels       [Bands]float32
	snapshot     [Bands]float32
	snapshotLive bool
}

func New(invalidate func()) *Feed {
	path, _ := exec.LookPath("cava")
	feed := &Feed{cavaPath: path}
	feed.capture = newCaptureLifecycle(path, invalidate, feed.start, nil)
	return feed
}

func NewStatic(levels []float32) *Feed {
	feed := &Feed{capture: newStaticCaptureLifecycle()}
	for index := range min(len(levels), Bands) {
		feed.levels[index] = max(0, min(levels[index], 1))
	}
	return feed
}

func (f *Feed) Want(now time.Time) {
	f.capture.Want(now)
}

func (f *Feed) SetActive(active bool) {
	f.capture.SetActive(active)
}

func (f *Feed) Levels(now time.Time) ([]float32, bool) {
	live := f.capture.Live(now)
	f.mu.Lock()
	defer f.mu.Unlock()
	if live {
		copy(f.snapshot[:], f.levels[:])
		f.snapshotLive = true
	} else if f.snapshotLive {
		clear(f.snapshot[:])
		f.snapshotLive = false
	}
	return f.snapshot[:], live
}

// Installed reports whether cava was found when the feed was created.
func (f *Feed) Installed() bool {
	return f.capture.Installed()
}

// Available reports whether cava can currently provide samples.
func (f *Feed) Available() bool {
	return f.capture.Available()
}

func (f *Feed) Close() {
	f.capture.Close()
}

func (f *Feed) start() (*runningCapture, error) {
	config, err := writeConfig(resolveSink())
	if err != nil {
		return nil, err
	}
	command := exec.Command(f.cavaPath, "-p", config)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	return startCapture(command, func() { f.readFrames(stdout) })
}

func (f *Feed) readFrames(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	var frame [Bands]float32
	for scanner.Scan() {
		if !parseFrame(scanner.Bytes(), frame[:]) {
			continue
		}
		f.capture.Publish(time.Now(), func() {
			f.mu.Lock()
			copy(f.levels[:], frame[:])
			f.mu.Unlock()
		})
	}
}
