package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/ryoku-dev/ryotunes/internal/daemon"
	"github.com/ryoku-dev/ryotunes/internal/diag"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print the Ryotunes version")
	socketPath := flag.String("socket", protocol.SocketPath(), "daemon Unix socket path")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Version)
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: diag.LevelFromEnv()}))
	slog.SetDefault(logger)
	if err := daemon.Run(context.Background(), *socketPath); err != nil {
		logger.Error("ryotunesd stopped", "error", err)
		os.Exit(1)
	}
}
