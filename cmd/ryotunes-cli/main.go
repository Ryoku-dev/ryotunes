package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

const (
	shortUsage = "usage: ryotunes-cli [--socket PATH] <method> [json] | events [name...] | methods"
	helpText   = `Usage:
  ryotunes-cli [--socket PATH] <method> [json]
  ryotunes-cli [--socket PATH] events [name...]
  ryotunes-cli methods

Options:
  --socket PATH  Connect to PATH instead of the default daemon socket.
  --help, -h     Show this help.

Use "ryotunes-cli methods" to list available methods.`
	skinNotice = "Ryotunes skins are now palette files. Open Settings > Appearance to choose one."
)

type commandKind uint8

const (
	commandCall commandKind = iota
	commandEvents
	commandMethods
	commandHelp
	commandSkinNotice
)

type command struct {
	Kind       commandKind
	Socket     string
	Request    protocol.Request
	EventNames []string
}

type envelopeHeader struct {
	ID    *uint64 `json:"id"`
	Event *string `json:"event"`
}

type methodSuggestion struct {
	name     string
	distance int
}

func main() {
	os.Exit(execute(os.Args[1:], os.Stdout, os.Stderr, stdoutIsTerminal(os.Stdout)))
}

func execute(args []string, stdout, stderr io.Writer, pretty bool) int {
	cmd, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, shortUsage)
		return 2
	}

	switch cmd.Kind {
	case commandHelp:
		fmt.Fprintln(stdout, helpText)
		return 0
	case commandMethods:
		for _, method := range protocol.Methods {
			fmt.Fprintf(stdout, "%s\t%s\n", method.Name, method.Doc)
		}
		return 0
	case commandSkinNotice:
		fmt.Fprintln(stderr, skinNotice)
		return 2
	}

	path := cmd.Socket
	if path == "" {
		path = protocol.SocketPath()
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		fmt.Fprintf(stderr, "ryotunesd not reachable at %s: %v\n", path, err)
		return 3
	}
	defer conn.Close()

	if err := protocol.NewEncoder(conn).Encode(cmd.Request); err != nil {
		fmt.Fprintf(stderr, "ryotunes-cli: send request: %v\n", err)
		return 1
	}

	if cmd.Kind == commandEvents {
		return followEvents(conn, cmd.EventNames, stdout, stderr)
	}
	return readCallResponse(conn, cmd.Request.ID, stdout, stderr, pretty)
}

func parseArgs(args []string) (command, error) {
	var parsed command
	rest := args
	if len(rest) > 0 && rest[0] == "--socket" {
		if len(rest) < 2 || rest[1] == "" {
			return command{}, fmt.Errorf("--socket needs a path")
		}
		parsed.Socket = rest[1]
		rest = rest[2:]
	}
	if len(rest) == 0 {
		return command{}, fmt.Errorf("missing method")
	}

	switch rest[0] {
	case "--help", "-h":
		if len(rest) != 1 {
			return command{}, fmt.Errorf("help does not accept arguments")
		}
		parsed.Kind = commandHelp
		return parsed, nil
	case "methods":
		if len(rest) != 1 {
			return command{}, fmt.Errorf("methods does not accept arguments")
		}
		parsed.Kind = commandMethods
		return parsed, nil
	case "skin":
		parsed.Kind = commandSkinNotice
		return parsed, nil
	case "events":
		parsed.Kind = commandEvents
		parsed.EventNames = append([]string(nil), rest[1:]...)
		subscriptions := parsed.EventNames
		if len(subscriptions) == 0 {
			subscriptions = []string{"*"}
		}
		params, err := json.Marshal(protocol.SubscribeParams{Events: subscriptions})
		if err != nil {
			return command{}, fmt.Errorf("encode subscription: %w", err)
		}
		parsed.Request = protocol.Request{
			ID:     1,
			Method: protocol.MethodSubscribe,
			Params: params,
		}
		return parsed, nil
	}

	method := rest[0]
	if _, ok := protocol.Lookup(method); !ok {
		return command{}, unknownMethodError(method)
	}
	if len(rest) > 2 {
		return command{}, fmt.Errorf("method %q accepts at most one JSON argument", method)
	}

	params := json.RawMessage("null")
	if len(rest) == 2 {
		if err := json.Unmarshal([]byte(rest[1]), &params); err != nil {
			return command{}, fmt.Errorf("params: %w", err)
		}
	}
	parsed.Kind = commandCall
	parsed.Request = protocol.Request{ID: 1, Method: method, Params: params}
	return parsed, nil
}

func unknownMethodError(name string) error {
	suggestions := make([]methodSuggestion, 0, len(protocol.Methods))
	for _, method := range protocol.Methods {
		suggestions = append(suggestions, methodSuggestion{
			name:     method.Name,
			distance: editDistance(name, method.Name),
		})
	}
	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].distance == suggestions[j].distance {
			return suggestions[i].name < suggestions[j].name
		}
		return suggestions[i].distance < suggestions[j].distance
	})
	if len(suggestions) > 3 {
		suggestions = suggestions[:3]
	}
	names := make([]string, len(suggestions))
	for i, suggestion := range suggestions {
		names[i] = suggestion.name
	}
	return fmt.Errorf("unknown method %q; closest methods: %s", name, strings.Join(names, ", "))
}

func editDistance(left, right string) int {
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)
	for i := range previous {
		previous[i] = i
	}
	for i := 1; i <= len(left); i++ {
		current[0] = i
		for j := 1; j <= len(right); j++ {
			cost := 0
			if left[i-1] != right[j-1] {
				cost = 1
			}
			current[j] = min3(
				current[j-1]+1,
				previous[j]+1,
				previous[j-1]+cost,
			)
		}
		previous, current = current, previous
	}
	return previous[len(right)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func readCallResponse(conn net.Conn, requestID uint64, stdout, stderr io.Writer, pretty bool) int {
	reader := bufio.NewReader(conn)
	for {
		line, err := protocol.ReadLine(reader)
		if err != nil {
			fmt.Fprintf(stderr, "ryotunes-cli: read response: %v\n", err)
			return 1
		}
		var header envelopeHeader
		if err := json.Unmarshal(line, &header); err != nil {
			fmt.Fprintf(stderr, "ryotunes-cli: invalid response: %v\n", err)
			return 1
		}
		if header.ID == nil || *header.ID != requestID {
			continue
		}
		var response protocol.Response
		if err := json.Unmarshal(line, &response); err != nil {
			fmt.Fprintf(stderr, "ryotunes-cli: invalid response: %v\n", err)
			return 1
		}
		if response.Error != nil {
			if err := printJSON(stdout, mustMarshal(response.Error), false); err != nil {
				fmt.Fprintf(stderr, "ryotunes-cli: write response: %v\n", err)
			}
			return 1
		}
		if err := printJSON(stdout, response.Result, pretty); err != nil {
			fmt.Fprintf(stderr, "ryotunes-cli: write response: %v\n", err)
			return 1
		}
		return 0
	}
}

func followEvents(conn net.Conn, eventNames []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	filter := make(map[string]struct{}, len(eventNames))
	for _, name := range eventNames {
		filter[name] = struct{}{}
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := protocol.ReadLine(reader)
		if err != nil {
			if ctx.Err() != nil {
				return 0
			}
			fmt.Fprintf(stderr, "ryotunes-cli: read events: %v\n", err)
			return 1
		}
		var header envelopeHeader
		if err := json.Unmarshal(line, &header); err != nil {
			fmt.Fprintf(stderr, "ryotunes-cli: invalid event line: %v\n", err)
			return 1
		}
		if header.ID != nil && *header.ID == 1 {
			var response protocol.Response
			if err := json.Unmarshal(line, &response); err != nil {
				fmt.Fprintf(stderr, "ryotunes-cli: invalid response: %v\n", err)
				return 1
			}
			if response.Error != nil {
				if err := printJSON(stdout, mustMarshal(response.Error), false); err != nil {
					fmt.Fprintf(stderr, "ryotunes-cli: write response: %v\n", err)
				}
				return 1
			}
			continue
		}
		if header.Event == nil {
			continue
		}
		if len(filter) != 0 {
			if _, ok := filter[*header.Event]; !ok {
				continue
			}
		}
		if err := printJSON(stdout, line, false); err != nil {
			fmt.Fprintf(stderr, "ryotunes-cli: write event: %v\n", err)
			return 1
		}
	}
}

func printJSON(w io.Writer, raw json.RawMessage, pretty bool) error {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	var output bytes.Buffer
	var err error
	if pretty {
		err = json.Indent(&output, raw, "", "  ")
	} else {
		err = json.Compact(&output, raw)
	}
	if err != nil {
		return err
	}
	output.WriteByte('\n')
	_, err = w.Write(output.Bytes())
	return err
}

func mustMarshal(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func stdoutIsTerminal(stdout *os.File) bool {
	info, err := stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
