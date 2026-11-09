package spectrum

import (
	"encoding/json"
	"testing"
)

func TestSinkFromObjects(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    string
	}{
		{
			name:    "direct sink",
			fixture: pipeWireFixture("alsa_output.pci-0000_00_1f.3.analog-stereo", "ryotunesd", "mpv", "mpv"),
			want:    "alsa_output.pci-0000_00_1f.3.analog-stereo",
		},
		{
			name:    "filter sink",
			fixture: pipeWireFixture("easyeffects_sink", "mpv", "Ryotunes", "Ryotunes"),
			want:    "easyeffects_sink",
		},
		{
			name: "no stream",
			fixture: `[
				{"id":51,"type":"PipeWire:Interface:Node","info":{"props":{"media.class":"Audio/Sink","node.name":"speakers"}}}
			]`,
			want: automaticSource,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var objects []pipeWireObject
			if err := json.Unmarshal([]byte(test.fixture), &objects); err != nil {
				t.Fatal(err)
			}
			if got := sinkFromObjects(objects); got != test.want {
				t.Fatalf("sinkFromObjects() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveSinkMissingPwDump(t *testing.T) {
	if got := resolveSinkWith("ryotunes-test-pw-dump-does-not-exist"); got != automaticSource {
		t.Fatalf("resolveSinkWith(missing) = %q, want %q", got, automaticSource)
	}
}

func pipeWireFixture(sink, binary, nodeName, applicationName string) string {
	return `[
		{"id":42,"type":"PipeWire:Interface:Node","info":{"props":{
			"media.class":"Stream/Output/Audio",
			"application.process.binary":"` + binary + `",
			"node.name":"` + nodeName + `",
			"application.name":"` + applicationName + `"
		}}},
		{"id":51,"type":"PipeWire:Interface:Node","info":{"props":{
			"media.class":"Audio/Sink","node.name":"` + sink + `"
		}}},
		{"id":60,"type":"PipeWire:Interface:Link","info":{
			"output-node-id":42,"input-node-id":51,"props":{}
		}}
	]`
}
