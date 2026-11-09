package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func TestParseArgsMethodWithoutParams(t *testing.T) {
	parsed, err := parseArgs([]string{"hello"})
	if err != nil {
		t.Fatalf("parseArgs returned an error: %v", err)
	}
	if parsed.Kind != commandCall {
		t.Fatalf("Kind = %v, want commandCall", parsed.Kind)
	}
	if parsed.Request.ID != 1 || parsed.Request.Method != protocol.MethodHello {
		t.Fatalf("Request = %#v", parsed.Request)
	}
	if string(parsed.Request.Params) != "null" {
		t.Fatalf("Params = %s, want null", parsed.Request.Params)
	}
}

func TestParseArgsMethodWithParams(t *testing.T) {
	parsed, err := parseArgs([]string{"seek", `{"position":12.5}`})
	if err != nil {
		t.Fatalf("parseArgs returned an error: %v", err)
	}
	var params protocol.SeekParams
	if err := json.Unmarshal(parsed.Request.Params, &params); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if params.Position != 12.5 {
		t.Fatalf("Position = %v, want 12.5", params.Position)
	}
}

func TestParseArgsSocketOverride(t *testing.T) {
	parsed, err := parseArgs([]string{"--socket", "/tmp/ryotunes-test.sock", "hello"})
	if err != nil {
		t.Fatalf("parseArgs returned an error: %v", err)
	}
	if parsed.Socket != "/tmp/ryotunes-test.sock" {
		t.Fatalf("Socket = %q", parsed.Socket)
	}
}

func TestParseArgsEvents(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantNames  []string
		wantEvents []string
	}{
		{
			name:       "all",
			args:       []string{"events"},
			wantEvents: []string{"*"},
		},
		{
			name:       "named",
			args:       []string{"events", "position", "queue-changed"},
			wantNames:  []string{"position", "queue-changed"},
			wantEvents: []string{"position", "queue-changed"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parseArgs(test.args)
			if err != nil {
				t.Fatalf("parseArgs returned an error: %v", err)
			}
			if parsed.Kind != commandEvents {
				t.Fatalf("Kind = %v, want commandEvents", parsed.Kind)
			}
			if parsed.Request.Method != protocol.MethodSubscribe {
				t.Fatalf("Method = %q, want subscribe", parsed.Request.Method)
			}
			if !equalStrings(parsed.EventNames, test.wantNames) {
				t.Fatalf("EventNames = %#v, want %#v", parsed.EventNames, test.wantNames)
			}
			var params protocol.SubscribeParams
			if err := json.Unmarshal(parsed.Request.Params, &params); err != nil {
				t.Fatalf("decode params: %v", err)
			}
			if !equalStrings(params.Events, test.wantEvents) {
				t.Fatalf("Events = %#v, want %#v", params.Events, test.wantEvents)
			}
		})
	}
}

func TestParseArgsLocalCommands(t *testing.T) {
	tests := []struct {
		args []string
		kind commandKind
	}{
		{args: []string{"methods"}, kind: commandMethods},
		{args: []string{"--help"}, kind: commandHelp},
		{args: []string{"-h"}, kind: commandHelp},
		{args: []string{"skin", "use", "old-theme"}, kind: commandSkinNotice},
	}
	for _, test := range tests {
		parsed, err := parseArgs(test.args)
		if err != nil {
			t.Fatalf("parseArgs(%q) returned an error: %v", test.args, err)
		}
		if parsed.Kind != test.kind {
			t.Fatalf("parseArgs(%q).Kind = %v, want %v", test.args, parsed.Kind, test.kind)
		}
	}
}

func TestParseArgsErrors(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains string
	}{
		{name: "missing method", contains: "missing method"},
		{name: "missing socket path", args: []string{"--socket"}, contains: "--socket needs a path"},
		{name: "invalid params", args: []string{"seek", "{"}, contains: "params:"},
		{name: "extra params", args: []string{"seek", "null", "extra"}, contains: "at most one JSON argument"},
		{name: "methods arguments", args: []string{"methods", "hello"}, contains: "does not accept arguments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseArgs(test.args)
			if err == nil {
				t.Fatal("parseArgs returned no error")
			}
			if !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %q, want it to contain %q", err, test.contains)
			}
		})
	}
}

func TestParseArgsUnknownMethodSuggestsCloseNames(t *testing.T) {
	_, err := parseArgs([]string{"helo"})
	if err == nil {
		t.Fatal("parseArgs returned no error")
	}
	if !strings.Contains(err.Error(), "hello") {
		t.Fatalf("error = %q, want hello suggestion", err)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
