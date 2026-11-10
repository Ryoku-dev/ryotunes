package shell

import (
	gioapp "gioui.org/app"
	"gioui.org/unit"
)

const relaxedWindowMax = unit.Dp(1 << 20)

type windowModePlan struct {
	width, height               unit.Dp
	minimumWidth, minimumHeight unit.Dp
	fixedAtMap                  bool
	persistSize                 bool
}

func mainWindowMode(width, height int, floating bool) windowModePlan {
	return windowModePlan{
		width: unit.Dp(width), height: unit.Dp(height),
		minimumWidth: 480, minimumHeight: 560,
		fixedAtMap: floating, persistSize: floating,
	}
}

func miniWindowMode(scale float32) windowModePlan {
	return windowModePlan{
		width: unit.Dp(520 * scale), height: unit.Dp(176 * scale),
		minimumWidth: unit.Dp(300 * scale), minimumHeight: unit.Dp(160 * scale),
		fixedAtMap: true,
	}
}

func (plan windowModePlan) creationOptions() []gioapp.Option {
	options := []gioapp.Option{gioapp.Size(plan.width, plan.height)}
	if plan.fixedAtMap {
		options = append(options,
			gioapp.MinSize(plan.width, plan.height),
			gioapp.MaxSize(plan.width, plan.height),
		)
		return options
	}
	return append(options, gioapp.MinSize(plan.minimumWidth, plan.minimumHeight))
}

func relaxedWindowOptions(minimumWidth, minimumHeight unit.Dp) []gioapp.Option {
	// Gio cannot clear an applied maximum, so a protocol-safe ceiling represents no practical maximum.
	return []gioapp.Option{
		gioapp.MinSize(minimumWidth, minimumHeight),
		gioapp.MaxSize(relaxedWindowMax, relaxedWindowMax),
	}
}

type windowPaddingTracker struct {
	padding  int
	measured bool
}

func (tracker *windowPaddingTracker) contentHeight(observedHeight int, plannedHeight unit.Dp) int {
	planned := int(plannedHeight + .5)
	if !tracker.measured {
		tracker.padding = max(0, observedHeight-planned)
		tracker.measured = true
	}
	if observedHeight == planned {
		tracker.padding = 0
	}
	return observedHeight - tracker.padding
}
