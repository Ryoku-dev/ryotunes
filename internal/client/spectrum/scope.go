package spectrum

import (
	"encoding/binary"
	"io"
	"math"
	"os/exec"
	"sync"
	"time"
)

const (
	scopeRate        = 48000
	scopeChannels    = 2
	scopeRingFrames  = 8192
	scopeChunkFrames = 256
)

// Scope captures the music sink's stereo samples for the oscilloscope and
// vectorscope.
type Scope struct {
	pwRecordPath string
	capture      *captureLifecycle

	mu         sync.Mutex
	ring       [scopeRingFrames * scopeChannels]float32
	writeFrame int
	stored     int
}

// NewScope creates a stereo sample feed backed by pw-record.
func NewScope(invalidate func()) *Scope {
	path, _ := exec.LookPath("pw-record")
	scope := &Scope{pwRecordPath: path}
	scope.capture = newCaptureLifecycle(path, invalidate, scope.start, scope.reset)
	return scope
}

// NewStaticScope creates a deterministic sample feed for demos and renders.
func NewStaticScope(samples []float32) *Scope {
	scope := &Scope{capture: newStaticCaptureLifecycle()}
	scope.publishInterleaved(samples)
	return scope
}

// Want keeps capture alive while a visible scope is drawing.
func (s *Scope) Want(now time.Time) {
	s.capture.Want(now)
}

// SetActive gates capture for playback and the selected visualizer style.
func (s *Scope) SetActive(active bool) {
	s.capture.SetActive(active)
}

// Read copies the newest complete interleaved stereo frames into dst.
func (s *Scope) Read(dst []float32) (frames int, live bool) {
	live = s.capture.Live(time.Now())
	s.mu.Lock()
	defer s.mu.Unlock()

	frames = min(s.stored, len(dst)/scopeChannels)
	start := s.writeFrame - frames
	if start < 0 {
		start += scopeRingFrames
	}
	for frame := range frames {
		source := ((start + frame) % scopeRingFrames) * scopeChannels
		target := frame * scopeChannels
		dst[target], dst[target+1] = s.ring[source], s.ring[source+1]
	}
	return frames, live
}

// Installed reports whether pw-record was found when the scope was created.
func (s *Scope) Installed() bool {
	return s.capture.Installed()
}

// Available reports whether pw-record can currently provide samples.
func (s *Scope) Available() bool {
	return s.capture.Available()
}

// Close stops capture and releases its process.
func (s *Scope) Close() {
	s.capture.Close()
}

func (s *Scope) start() (*runningCapture, error) {
	command := exec.Command(s.pwRecordPath,
		"--target", resolveSink(),
		"-P", "{ stream.capture.sink = true }",
		"--rate", "48000",
		"--channels", "2",
		"--format", "f32",
		"--raw", "-",
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	return startCapture(command, func() { s.readSamples(stdout) })
}

func (s *Scope) readSamples(reader io.Reader) {
	var raw [scopeChunkFrames * scopeChannels * 4]byte
	for {
		if _, err := io.ReadFull(reader, raw[:]); err != nil {
			return
		}
		s.capture.Publish(time.Now(), func() {
			s.mu.Lock()
			s.publishBytesLocked(raw[:])
			s.mu.Unlock()
		})
	}
}

func (s *Scope) reset() {
	s.mu.Lock()
	clear(s.ring[:])
	s.writeFrame = 0
	s.stored = 0
	s.mu.Unlock()
}

func (s *Scope) publishInterleaved(samples []float32) {
	frames := len(samples) / scopeChannels
	if frames > scopeRingFrames {
		samples = samples[(frames-scopeRingFrames)*scopeChannels:]
		frames = scopeRingFrames
	}
	for frame := range frames {
		target := s.writeFrame * scopeChannels
		source := frame * scopeChannels
		s.ring[target], s.ring[target+1] = samples[source], samples[source+1]
		s.writeFrame = (s.writeFrame + 1) % scopeRingFrames
	}
	s.stored = min(scopeRingFrames, s.stored+frames)
}

func (s *Scope) publishBytesLocked(raw []byte) {
	frames := len(raw) / (scopeChannels * 4)
	for frame := range frames {
		target := s.writeFrame * scopeChannels
		source := frame * scopeChannels * 4
		s.ring[target] = math.Float32frombits(binary.LittleEndian.Uint32(raw[source:]))
		s.ring[target+1] = math.Float32frombits(binary.LittleEndian.Uint32(raw[source+4:]))
		s.writeFrame = (s.writeFrame + 1) % scopeRingFrames
	}
	s.stored = min(scopeRingFrames, s.stored+frames)
}
