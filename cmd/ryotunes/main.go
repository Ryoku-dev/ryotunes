package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	gioapp "gioui.org/app"

	"github.com/ryoku-dev/ryotunes/internal/client/conn"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/shell"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func main() {
	pngPath := flag.String("png", "", "render one frame to a PNG and exit")
	size := flag.String("size", "", "window or PNG size as WIDTHxHEIGHT (default 1280x800)")
	demoMode := flag.Bool("demo", false, "use the in-process demo daemon")
	route := flag.String("route", "", "initial route (default home)")
	panel := flag.String("panel", "", "initial panel: queue, lyrics, now-playing, listen-together, or empty")
	mode := flag.String("mode", "", "palette mode override: follow, dark, or light")
	mini := flag.Bool("mini", false, "open the mini player")
	flag.Parse()

	width, height := 0, 0
	if *size != "" {
		var err error
		width, height, err = parseSize(*size)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ryotunes:", err)
			os.Exit(2)
		}
	}
	if !*demoMode && *pngPath == "" {
		shown, err := showExisting()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ryotunes:", err)
			os.Exit(1)
		}
		if shown {
			return
		}
	}

	options := shell.Options{Demo: *demoMode, PNG: *pngPath, Width: width, Height: height, Route: *route, Panel: *panel, Mode: *mode, Mini: *mini}
	if *pngPath != "" {
		if err := shell.Run(options); err != nil {
			fmt.Fprintln(os.Stderr, "ryotunes:", err)
			os.Exit(1)
		}
		return
	}
	go func() {
		if err := shell.Run(options); err != nil {
			fmt.Fprintln(os.Stderr, "ryotunes:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	gioapp.Main()
}

func parseSize(value string) (int, int, error) {
	widthText, heightText, ok := strings.Cut(strings.ToLower(strings.TrimSpace(value)), "x")
	if !ok {
		return 0, 0, fmt.Errorf("invalid --size %q: use WIDTHxHEIGHT", value)
	}
	width, widthErr := strconv.Atoi(widthText)
	height, heightErr := strconv.Atoi(heightText)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("invalid --size %q: use positive dimensions", value)
	}
	return width, height, nil
}

func showExisting() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "unix", protocol.SocketPath())
	if err != nil {
		return false, nil
	}
	defer connection.Close()
	stop := context.AfterFunc(ctx, func() { _ = connection.SetDeadline(time.Now()) })
	defer stop()
	encoder := protocol.NewEncoder(connection)
	if err := encoder.Encode(protocol.Request{ID: 1, Method: protocol.MethodHello, Params: json.RawMessage("null")}); err != nil {
		return false, err
	}
	reader := bufio.NewReader(connection)
	for {
		line, err := protocol.ReadLine(reader)
		if err != nil {
			return false, err
		}
		var response protocol.Response
		if json.Unmarshal(line, &response) != nil || response.ID != 1 {
			continue
		}
		if response.Error != nil {
			return false, response.Error
		}
		var hello protocol.Hello
		if err := json.Unmarshal(response.Result, &hello); err != nil {
			return false, err
		}
		if hello.Clients <= 0 {
			return false, nil
		}
		showContext, showCancel := context.WithTimeout(context.Background(), time.Second)
		defer showCancel()
		if err := conn.Show(showContext); err != nil {
			return false, err
		}
		return true, nil
	}
}
