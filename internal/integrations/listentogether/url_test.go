package listentogether

import (
	"errors"
	"strings"
	"testing"
)

func TestDefaultServerURLIsUnset(t *testing.T) {
	if DefaultServerURL != "" {
		t.Fatalf("DefaultServerURL = %q, want empty", DefaultServerURL)
	}
}

func TestValidateServerURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		err  error
	}{
		{name: "websocket", raw: "  ws://127.0.0.1:8080  ", want: "ws://127.0.0.1:8080/"},
		{name: "secure path and query", raw: "wss://sync.example/room?x=1", want: "wss://sync.example/room?x=1"},
		{name: "too long", raw: "ws://" + strings.Repeat("a", maxServerURLBytes), err: ErrServerURLTooLong},
		{name: "invalid syntax", raw: "ws://%", err: ErrServerURLInvalid},
		{name: "relative text", raw: "sync.example", err: ErrServerURLInvalid},
		{name: "wrong scheme", raw: "https://sync.example", err: ErrServerURLScheme},
		{name: "missing host", raw: "ws:///socket", err: ErrServerURLScheme},
		{name: "credentials", raw: "wss://user:secret@sync.example", err: ErrServerURLCredentials},
		{name: "fragment", raw: "wss://sync.example/#private", err: ErrServerURLCredentials},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ValidateServerURL(test.raw)
			if !errors.Is(err, test.err) {
				t.Fatalf("error = %v, want %v", err, test.err)
			}
			if got != test.want {
				t.Fatalf("URL = %q, want %q", got, test.want)
			}
		})
	}
}

func TestServerURLErrorSentences(t *testing.T) {
	want := []string{
		"Listen Together server URL is too long.",
		"Enter a valid ws:// or wss:// server URL.",
		"Listen Together server must use ws:// or wss:// and include a host.",
		"Listen Together server URLs cannot contain credentials or fragments.",
	}
	got := []error{ErrServerURLTooLong, ErrServerURLInvalid, ErrServerURLScheme, ErrServerURLCredentials}
	for index, err := range got {
		if err.Error() != want[index] {
			t.Errorf("error %d = %q, want %q", index, err, want[index])
		}
	}
}
