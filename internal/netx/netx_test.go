package netx

import (
	"net/http"
	"testing"
	"time"
)

func TestSetProxyUpdatesSharedTransport(t *testing.T) {
	t.Cleanup(func() { _ = SetProxy("") })
	request, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"http://127.0.0.1:8080", "https://proxy.example:8443"} {
		if err := SetProxy(raw); err != nil {
			t.Fatal(err)
		}
		got, err := Transport().Proxy(request)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.String() != raw {
			t.Fatalf("proxy = %v, want %s", got, raw)
		}
		if Proxy() != raw {
			t.Fatalf("raw proxy = %q, want %q", Proxy(), raw)
		}
	}
}

func TestSetProxyRejectsInvalidURL(t *testing.T) {
	for _, raw := range []string{"proxy.example:8080", "file:///tmp/proxy"} {
		if err := SetProxy(raw); err == nil {
			t.Fatalf("SetProxy(%q) succeeded", raw)
		}
	}
}

func TestClientUsesSharedTransport(t *testing.T) {
	client := Client(3 * time.Second)
	if client.Transport != Transport() {
		t.Fatal("client does not use shared transport")
	}
	if client.Timeout != 3*time.Second {
		t.Fatalf("timeout = %v", client.Timeout)
	}
}
