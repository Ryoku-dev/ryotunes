package daemon

import (
	"context"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func TestEveryProtocolMethodHasHandler(t *testing.T) {
	for _, method := range protocol.Methods {
		if method.Name == protocol.MethodSubscribe {
			continue
		}
		if handlers[method.Name] == nil {
			t.Errorf("protocol method %q has no daemon handler", method.Name)
		}
	}
}

func TestGetLogsIncludesRuntimeDiagnostics(t *testing.T) {
	app := &App{EngineVersion: "mpv 0.40.0"}
	result, err := getLogs(context.Background(), app, protocol.GetLogsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if result.EngineVersion != app.EngineVersion {
		t.Fatalf("engine version = %q, want %q", result.EngineVersion, app.EngineVersion)
	}
	if result.MPRIS {
		t.Fatal("MPRIS reported active without a service")
	}
}
