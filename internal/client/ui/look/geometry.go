package look

import (
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
)

// Class is the responsive frame structure selected from the window width.
type Class uint8

const (
	Compact Class = iota
	Medium
	Expanded
)

// Frame dimensions in dp.
const (
	TopBar              unit.Dp = 56
	Gap                 unit.Dp = 8
	SidebarExpanded     unit.Dp = 240
	SidebarMedium       unit.Dp = 72
	Panel               unit.Dp = 340
	PlayerBar           unit.Dp = 88
	PlayerCompact       unit.Dp = 64
	BottomNav           unit.Dp = 56
	ContentMinWithPanel unit.Dp = 560
)

// ClassFor resolves the frame structure for a width in dp.
func ClassFor(widthDp float32) Class {
	switch {
	case widthDp >= tokens.BreakpointExpanded:
		return Expanded
	case widthDp >= tokens.BreakpointMedium:
		return Medium
	default:
		return Compact
	}
}

// ContentInset returns the horizontal content inset for a frame class.
func ContentInset(class Class) unit.Dp {
	switch class {
	case Expanded:
		return 34
	case Medium:
		return 21
	default:
		return 13
	}
}
