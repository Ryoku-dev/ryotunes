// Package tray publishes the Ryotunes StatusNotifierItem and DBusMenu.
package tray

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	itemPath      = dbus.ObjectPath("/StatusNotifierItem")
	menuPath      = dbus.ObjectPath("/MenuBar")
	itemInterface = "org.kde.StatusNotifierItem"
	menuInterface = "com.canonical.dbusmenu"
)

const (
	menuShow         int32 = 1
	menuSeparatorOne int32 = 2
	menuPlayPause    int32 = 3
	menuNext         int32 = 4
	menuPrevious     int32 = 5
	menuSeparatorTwo int32 = 6
	menuQuit         int32 = 7
)

// Actions is the tray action surface supplied by the daemon.
type Actions interface {
	Show() error
	PlayPause() error
	Next() error
	Previous() error
	Quit() error
}

// IconPixmap is one StatusNotifierItem ARGB icon image.
type IconPixmap struct {
	Width  int32
	Height int32
	Data   []byte
}

// ToolTip is the StatusNotifierItem tooltip tuple.
type ToolTip struct {
	IconName    string
	IconPixmap  []IconPixmap
	Title       string
	Description string
}

// MenuLayout is one recursive DBusMenu layout node.
type MenuLayout struct {
	ID         int32
	Properties map[string]dbus.Variant
	Children   []dbus.Variant
}

// MenuEvent is one DBusMenu EventGroup input record.
type MenuEvent struct {
	ID        int32
	EventID   string
	Data      dbus.Variant
	Timestamp uint32
}

// Tray owns the StatusNotifierItem bus name and menu.
type Tray struct {
	conn      *dbus.Conn
	name      string
	actions   Actions
	log       *slog.Logger
	itemProps *prop.Properties
	menuProps *prop.Properties

	mu       sync.RWMutex
	playing  bool
	revision uint32
	closed   bool
	done     chan struct{}
}

// New registers the Ryotunes StatusNotifierItem and its exact action menu.
func New(ctx context.Context, actions Actions, log *slog.Logger) (*Tray, error) {
	if actions == nil {
		return nil, errors.New("tray: nil actions")
	}
	if log == nil {
		log = slog.Default()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("tray: connect session bus: %w", err)
	}
	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("tray: request bus name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		_ = conn.Close()
		return nil, fmt.Errorf("tray: bus name %s is already owned", name)
	}

	t := &Tray{conn: conn, name: name, actions: actions, log: log, revision: 1, done: make(chan struct{})}
	if err := conn.Export(t, itemPath, itemInterface); err != nil {
		t.Close()
		return nil, fmt.Errorf("tray: export item: %w", err)
	}
	if err := conn.Export(t, menuPath, menuInterface); err != nil {
		t.Close()
		return nil, fmt.Errorf("tray: export menu: %w", err)
	}
	t.itemProps, err = prop.Export(conn, itemPath, itemPropertySpec())
	if err != nil {
		t.Close()
		return nil, fmt.Errorf("tray: export item properties: %w", err)
	}
	t.menuProps, err = prop.Export(conn, menuPath, menuPropertySpec())
	if err != nil {
		t.Close()
		return nil, fmt.Errorf("tray: export menu properties: %w", err)
	}
	if err := t.exportIntrospection(); err != nil {
		t.Close()
		return nil, err
	}

	watcher := conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher")
	if err := watcher.CallWithContext(ctx, "org.kde.StatusNotifierWatcher.RegisterStatusNotifierItem", 0, name).Err; err != nil {
		log.Warn("tray: StatusNotifierItem registration failed", "error", err)
	}
	go func() {
		select {
		case <-ctx.Done():
			t.Close()
		case <-t.done:
		}
	}()
	return t, nil
}

func itemPropertySpec() prop.Map {
	return prop.Map{itemInterface: {
		"Category":            {Value: "ApplicationStatus", Emit: prop.EmitConst},
		"Id":                  {Value: "ryotunes", Emit: prop.EmitConst},
		"Title":               {Value: "Ryotunes", Emit: prop.EmitConst},
		"Status":              {Value: "Active", Emit: prop.EmitConst},
		"WindowId":            {Value: uint32(0), Emit: prop.EmitConst},
		"IconName":            {Value: "ryotunes", Emit: prop.EmitConst},
		"IconPixmap":          {Value: []IconPixmap{}, Emit: prop.EmitConst},
		"OverlayIconName":     {Value: "", Emit: prop.EmitConst},
		"OverlayIconPixmap":   {Value: []IconPixmap{}, Emit: prop.EmitConst},
		"AttentionIconName":   {Value: "", Emit: prop.EmitConst},
		"AttentionIconPixmap": {Value: []IconPixmap{}, Emit: prop.EmitConst},
		"AttentionMovieName":  {Value: "", Emit: prop.EmitConst},
		"ToolTip":             {Value: ToolTip{IconName: "ryotunes", Title: "Ryotunes"}, Emit: prop.EmitConst},
		"ItemIsMenu":          {Value: false, Emit: prop.EmitConst},
		"Menu":                {Value: menuPath, Emit: prop.EmitConst},
	}}
}

func menuPropertySpec() prop.Map {
	return prop.Map{menuInterface: {
		"Version":       {Value: uint32(3), Emit: prop.EmitConst},
		"TextDirection": {Value: "ltr", Emit: prop.EmitConst},
		"Status":        {Value: "normal", Emit: prop.EmitConst},
		"IconThemePath": {Value: []string{}, Emit: prop.EmitConst},
	}}
}

func (t *Tray) exportIntrospection() error {
	itemNode := &introspect.Node{Name: string(itemPath), Interfaces: []introspect.Interface{
		introspect.IntrospectData,
		prop.IntrospectData,
		{
			Name: itemInterface,
			Methods: []introspect.Method{
				{Name: "ContextMenu", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
				{Name: "Activate", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
				{Name: "SecondaryActivate", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
				{Name: "Scroll", Args: []introspect.Arg{{Name: "delta", Type: "i", Direction: "in"}, {Name: "orientation", Type: "s", Direction: "in"}}},
			},
			Signals: []introspect.Signal{
				{Name: "NewTitle"}, {Name: "NewIcon"}, {Name: "NewAttentionIcon"},
				{Name: "NewOverlayIcon"}, {Name: "NewToolTip"},
				{Name: "NewStatus", Args: []introspect.Arg{{Name: "status", Type: "s"}}},
			},
			Properties: t.itemProps.Introspection(itemInterface),
		},
	}}
	if err := t.conn.Export(introspect.NewIntrospectable(itemNode), itemPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("tray: export item introspection: %w", err)
	}
	menuNode := &introspect.Node{Name: string(menuPath), Interfaces: []introspect.Interface{
		introspect.IntrospectData,
		prop.IntrospectData,
		{
			Name: menuInterface,
			Methods: []introspect.Method{
				{Name: "GetLayout", Args: []introspect.Arg{{Name: "parentId", Type: "i", Direction: "in"}, {Name: "recursionDepth", Type: "i", Direction: "in"}, {Name: "propertyNames", Type: "as", Direction: "in"}, {Name: "revision", Type: "u", Direction: "out"}, {Name: "layout", Type: "(ia{sv}av)", Direction: "out"}}},
				{Name: "GetGroupProperties", Args: []introspect.Arg{{Name: "ids", Type: "ai", Direction: "in"}, {Name: "propertyNames", Type: "as", Direction: "in"}, {Name: "properties", Type: "a(ia{sv})", Direction: "out"}}},
				{Name: "GetProperty", Args: []introspect.Arg{{Name: "id", Type: "i", Direction: "in"}, {Name: "name", Type: "s", Direction: "in"}, {Name: "value", Type: "v", Direction: "out"}}},
				{Name: "Event", Args: []introspect.Arg{{Name: "id", Type: "i", Direction: "in"}, {Name: "eventId", Type: "s", Direction: "in"}, {Name: "data", Type: "v", Direction: "in"}, {Name: "timestamp", Type: "u", Direction: "in"}}},
				{Name: "EventGroup", Args: []introspect.Arg{{Name: "events", Type: "a(isvu)", Direction: "in"}, {Name: "idErrors", Type: "ai", Direction: "out"}}},
				{Name: "AboutToShow", Args: []introspect.Arg{{Name: "id", Type: "i", Direction: "in"}, {Name: "needUpdate", Type: "b", Direction: "out"}}},
				{Name: "AboutToShowGroup", Args: []introspect.Arg{{Name: "ids", Type: "ai", Direction: "in"}, {Name: "updatesNeeded", Type: "ai", Direction: "out"}, {Name: "idErrors", Type: "ai", Direction: "out"}}},
			},
			Signals: []introspect.Signal{
				{Name: "ItemsPropertiesUpdated", Args: []introspect.Arg{{Name: "updatedProps", Type: "a(ia{sv})"}, {Name: "removedProps", Type: "a(ias)"}}},
				{Name: "LayoutUpdated", Args: []introspect.Arg{{Name: "revision", Type: "u"}, {Name: "parent", Type: "i"}}},
			},
			Properties: t.menuProps.Introspection(menuInterface),
		},
	}}
	if err := t.conn.Export(introspect.NewIntrospectable(menuNode), menuPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("tray: export menu introspection: %w", err)
	}
	return nil
}

// Activate handles a primary click by showing Ryotunes.
func (t *Tray) Activate(_, _ int32) *dbus.Error { return actionError(t.actions.Show()) }

// ContextMenu is present for StatusNotifierItem compatibility; the host opens Menu itself.
func (t *Tray) ContextMenu(_, _ int32) *dbus.Error { return nil }

// SecondaryActivate has no separate Ryotunes action.
func (t *Tray) SecondaryActivate(_, _ int32) *dbus.Error { return nil }

// Scroll has no tray-specific action.
func (t *Tray) Scroll(_ int32, _ string) *dbus.Error { return nil }

// SetPlaying changes the dynamic Play/Pause menu label.
func (t *Tray) SetPlaying(playing bool) {
	t.mu.Lock()
	if t.playing == playing || t.closed {
		t.mu.Unlock()
		return
	}
	t.playing = playing
	t.revision++
	revision := t.revision
	t.mu.Unlock()
	updated := []struct {
		ID         int32
		Properties map[string]dbus.Variant
	}{{ID: menuPlayPause, Properties: map[string]dbus.Variant{"label": dbus.MakeVariant(t.playPauseLabel())}}}
	if err := t.conn.Emit(menuPath, menuInterface+".ItemsPropertiesUpdated", updated, []struct {
		ID         int32
		Properties []string
	}{}); err != nil {
		t.log.Debug("tray menu property signal failed", "error", err)
	}
	if err := t.conn.Emit(menuPath, menuInterface+".LayoutUpdated", revision, int32(0)); err != nil {
		t.log.Debug("tray menu layout signal failed", "error", err)
	}
}

// GetLayout returns the current DBusMenu tree.
func (t *Tray) GetLayout(parentID, recursionDepth int32, propertyNames []string) (uint32, MenuLayout, *dbus.Error) {
	t.mu.RLock()
	revision := t.revision
	t.mu.RUnlock()
	layout, ok := t.layout(parentID, recursionDepth, propertyNames)
	if !ok {
		return revision, MenuLayout{}, dbus.NewError("com.canonical.dbusmenu.Error.InvalidMenu", []any{"unknown menu id"})
	}
	return revision, layout, nil
}

// GetGroupProperties returns selected properties for known menu IDs.
func (t *Tray) GetGroupProperties(ids []int32, propertyNames []string) ([]struct {
	ID         int32
	Properties map[string]dbus.Variant
}, *dbus.Error) {
	out := make([]struct {
		ID         int32
		Properties map[string]dbus.Variant
	}, 0, len(ids))
	for _, id := range ids {
		if properties, ok := t.menuItemProperties(id, propertyNames); ok {
			out = append(out, struct {
				ID         int32
				Properties map[string]dbus.Variant
			}{ID: id, Properties: properties})
		}
	}
	return out, nil
}

// GetProperty returns one property for one menu item.
func (t *Tray) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	properties, ok := t.menuItemProperties(id, []string{name})
	if !ok {
		return dbus.Variant{}, dbus.NewError("com.canonical.dbusmenu.Error.InvalidMenu", []any{"unknown menu id"})
	}
	value, ok := properties[name]
	if !ok {
		return dbus.Variant{}, dbus.NewError("com.canonical.dbusmenu.Error.InvalidProperty", []any{"unknown property"})
	}
	return value, nil
}

// Event dispatches clicked DBusMenu events.
func (t *Tray) Event(id int32, eventID string, _ dbus.Variant, _ uint32) *dbus.Error {
	if eventID != "clicked" {
		return nil
	}
	var err error
	switch id {
	case menuShow:
		err = t.actions.Show()
	case menuPlayPause:
		err = t.actions.PlayPause()
	case menuNext:
		err = t.actions.Next()
	case menuPrevious:
		err = t.actions.Previous()
	case menuQuit:
		err = t.actions.Quit()
	case menuSeparatorOne, menuSeparatorTwo, 0:
		return nil
	default:
		return dbus.NewError("com.canonical.dbusmenu.Error.InvalidMenu", []any{"unknown menu id"})
	}
	return actionError(err)
}

// EventGroup dispatches a batch and returns IDs which could not be handled.
func (t *Tray) EventGroup(events []MenuEvent) ([]int32, *dbus.Error) {
	failed := make([]int32, 0)
	for _, event := range events {
		if err := t.Event(event.ID, event.EventID, event.Data, event.Timestamp); err != nil {
			failed = append(failed, event.ID)
		}
	}
	return failed, nil
}

// AboutToShow reports that the menu is already current.
func (t *Tray) AboutToShow(id int32) (bool, *dbus.Error) {
	_, ok := t.menuItemProperties(id, nil)
	if !ok {
		return false, dbus.NewError("com.canonical.dbusmenu.Error.InvalidMenu", []any{"unknown menu id"})
	}
	return false, nil
}

// AboutToShowGroup reports no pending layout updates and any unknown IDs.
func (t *Tray) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	unknown := make([]int32, 0)
	for _, id := range ids {
		if _, ok := t.menuItemProperties(id, nil); !ok {
			unknown = append(unknown, id)
		}
	}
	return []int32{}, unknown, nil
}

func (t *Tray) layout(parentID, depth int32, names []string) (MenuLayout, bool) {
	properties, ok := t.menuItemProperties(parentID, names)
	if !ok {
		return MenuLayout{}, false
	}
	layout := MenuLayout{ID: parentID, Properties: properties, Children: []dbus.Variant{}}
	if parentID != 0 || depth == 0 {
		return layout, true
	}
	for _, id := range []int32{menuShow, menuSeparatorOne, menuPlayPause, menuNext, menuPrevious, menuSeparatorTwo, menuQuit} {
		childProperties, _ := t.menuItemProperties(id, names)
		layout.Children = append(layout.Children, dbus.MakeVariant(MenuLayout{ID: id, Properties: childProperties, Children: []dbus.Variant{}}))
	}
	return layout, true
}

func (t *Tray) menuItemProperties(id int32, requested []string) (map[string]dbus.Variant, bool) {
	properties := map[string]dbus.Variant{"enabled": dbus.MakeVariant(true), "visible": dbus.MakeVariant(true)}
	switch id {
	case 0:
		properties["children-display"] = dbus.MakeVariant("submenu")
	case menuShow:
		properties["label"] = dbus.MakeVariant("Show Ryotunes")
	case menuPlayPause:
		properties["label"] = dbus.MakeVariant(t.playPauseLabel())
	case menuNext:
		properties["label"] = dbus.MakeVariant("Next")
	case menuPrevious:
		properties["label"] = dbus.MakeVariant("Previous")
	case menuQuit:
		properties["label"] = dbus.MakeVariant("Quit")
	case menuSeparatorOne, menuSeparatorTwo:
		properties["type"] = dbus.MakeVariant("separator")
	default:
		return nil, false
	}
	if len(requested) == 0 {
		return properties, true
	}
	filtered := make(map[string]dbus.Variant, len(requested))
	for _, name := range requested {
		if value, ok := properties[name]; ok {
			filtered[name] = value
		}
	}
	return filtered, true
}

func (t *Tray) playPauseLabel() string {
	t.mu.RLock()
	playing := t.playing
	t.mu.RUnlock()
	if playing {
		return "Pause"
	}
	return "Play"
}

// Close unregisters the tray item and releases its bus name.
func (t *Tray) Close() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	close(t.done)
	t.mu.Unlock()
	if t.conn == nil {
		return
	}
	_ = t.conn.Export(nil, itemPath, itemInterface)
	_ = t.conn.Export(nil, itemPath, "org.freedesktop.DBus.Properties")
	_ = t.conn.Export(nil, itemPath, "org.freedesktop.DBus.Introspectable")
	_ = t.conn.Export(nil, menuPath, menuInterface)
	_ = t.conn.Export(nil, menuPath, "org.freedesktop.DBus.Properties")
	_ = t.conn.Export(nil, menuPath, "org.freedesktop.DBus.Introspectable")
	_, _ = t.conn.ReleaseName(t.name)
	_ = t.conn.Close()
}

func actionError(err error) *dbus.Error {
	if err == nil {
		return nil
	}
	return dbus.MakeFailedError(err)
}
