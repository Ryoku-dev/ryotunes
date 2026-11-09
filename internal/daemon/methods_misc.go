package daemon

import (
	"context"

	"github.com/ryoku-dev/ryotunes/internal/diag"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/version"
)

func init() {
	register(protocol.MethodHello, typed(hello))
	register(protocol.MethodShow, typed(show))
	register(protocol.MethodQuit, typed(quit))
	register(protocol.MethodGetLogs, typed(getLogs))
	register(protocol.MethodLogClientError, typed(logClientError))
	register(protocol.MethodOpenLogsFolder, typed(openLogsFolder))
}

func hello(_ context.Context, a *App, _ protocol.Null) (protocol.Hello, error) {
	clients := 0
	if a.Server != nil {
		clients = a.Server.Clients()
	}
	return protocol.Hello{Protocol: protocol.Version, Daemon: version.Version, Clients: clients}, nil
}
func show(_ context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	if a.Server == nil {
		return protocol.Null{}, nil
	}
	return protocol.Null{}, a.Server.Show()
}
func quit(_ context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	return protocol.Null{}, nil
}
func getLogs(_ context.Context, a *App, p protocol.GetLogsParams) (model.LogsResult, error) {
	level, source, query, limit := "warn", "all", "", 500
	if p.Level != nil {
		level = *p.Level
	}
	if p.Source != nil {
		source = *p.Source
	}
	if p.Query != nil {
		query = *p.Query
	}
	if p.Limit != nil {
		limit = *p.Limit
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 2000 {
		limit = 2000
	}
	if a.Diag == nil {
		return diag.Query(level, source, query, limit), nil
	}
	return a.Diag.Query(level, source, query, limit), nil
}
func logClientError(_ context.Context, a *App, p protocol.LogClientErrorParams) (protocol.Null, error) {
	target, level := "client", "error"
	if p.Target != nil {
		target = *p.Target
	}
	if p.Level != nil && *p.Level == "warn" {
		level = "warn"
	}
	if a.Diag == nil {
		diag.RecordClient(p.Message, target, level)
	} else {
		a.Diag.RecordClient(p.Message, target, level)
	}
	return protocol.Null{}, nil
}
func openLogsFolder(_ context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	if a.Diag == nil {
		return protocol.Null{}, diag.OpenLogsFolder()
	}
	return protocol.Null{}, a.Diag.OpenLogsFolder()
}
