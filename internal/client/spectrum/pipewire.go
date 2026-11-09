package spectrum

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const automaticSource = "auto"

type pipeWireObject struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Info struct {
		Props        map[string]any `json:"props"`
		OutputNodeID int            `json:"output-node-id"`
		InputNodeID  int            `json:"input-node-id"`
	} `json:"info"`
}

func resolveSink() string {
	return resolveSinkWith("pw-dump")
}

func resolveSinkWith(command string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, command).Output()
	if err != nil {
		return automaticSource
	}
	var objects []pipeWireObject
	if json.Unmarshal(output, &objects) != nil {
		return automaticSource
	}
	return sinkFromObjects(objects)
}

func sinkFromObjects(objects []pipeWireObject) string {
	nodes := make(map[int]pipeWireObject)
	streams := make(map[int]struct{})
	for _, object := range objects {
		if object.Type != "PipeWire:Interface:Node" {
			continue
		}
		nodes[object.ID] = object
		props := object.Info.Props
		if stringProperty(props, "media.class") != "Stream/Output/Audio" {
			continue
		}
		if stringProperty(props, "application.process.binary") == "ryotunesd" ||
			stringProperty(props, "node.name") == "Ryotunes" ||
			stringProperty(props, "application.name") == "Ryotunes" {
			streams[object.ID] = struct{}{}
		}
	}

	for _, object := range objects {
		if object.Type != "PipeWire:Interface:Link" {
			continue
		}
		outputID := object.Info.OutputNodeID
		if outputID == 0 {
			outputID = intProperty(object.Info.Props, "link.output.node")
		}
		if _, ok := streams[outputID]; !ok {
			continue
		}
		inputID := object.Info.InputNodeID
		if inputID == 0 {
			inputID = intProperty(object.Info.Props, "link.input.node")
		}
		sink, ok := nodes[inputID]
		if !ok || stringProperty(sink.Info.Props, "media.class") != "Audio/Sink" {
			continue
		}
		name := stringProperty(sink.Info.Props, "node.name")
		if name != "" && !strings.ContainsAny(name, "\r\n") {
			return name
		}
	}
	return automaticSource
}

func stringProperty(props map[string]any, name string) string {
	value, _ := props[name].(string)
	return value
}

func intProperty(props map[string]any, name string) int {
	switch value := props[name].(type) {
	case float64:
		return int(value)
	case string:
		n, _ := strconv.Atoi(value)
		return n
	default:
		return 0
	}
}
