package spectrum

import (
	"strconv"
	"strings"
	"testing"
)

func TestParseFrame(t *testing.T) {
	line := testFrame()
	var levels [Bands]float32
	if !parseFrame(line, levels[:]) {
		t.Fatal("valid frame was rejected")
	}
	for index := range Bands {
		want := float32(index*15) / 1000
		if levels[index] != want {
			t.Fatalf("band %d = %v, want %v", index, levels[index], want)
		}
	}
}

func TestParseFrameRejectsMalformedInput(t *testing.T) {
	valid := testFrame()
	tests := map[string][]byte{
		"empty":           nil,
		"missing value":   valid[:len(valid)-4],
		"missing trailer": valid[:len(valid)-1],
		"extra value":     append(append([]byte{}, valid...), '0', ';'),
		"negative":        append([]byte("-1;"), valid[2:]...),
		"over range":      append([]byte("1001;"), valid[2:]...),
		"not numeric":     append([]byte("x;"), valid[2:]...),
	}
	for name, line := range tests {
		t.Run(name, func(t *testing.T) {
			var levels [Bands]float32
			if parseFrame(line, levels[:]) {
				t.Fatal("malformed frame was accepted")
			}
		})
	}
}

func TestParseFrameAllocations(t *testing.T) {
	line := testFrame()
	var levels [Bands]float32
	if allocations := testing.AllocsPerRun(1000, func() {
		if !parseFrame(line, levels[:]) {
			panic("valid frame rejected")
		}
	}); allocations != 0 {
		t.Fatalf("parseFrame allocated %v times per call", allocations)
	}
}

func testFrame() []byte {
	var line strings.Builder
	for index := range Bands {
		line.WriteString(strconv.Itoa(index * 15))
		line.WriteByte(';')
	}
	return []byte(line.String())
}
