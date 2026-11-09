package chrome

import (
	"io"
	"net/url"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type togetherMemberActions struct {
	kick     kit.Plaque
	transfer kit.Plaque
}

type togetherDecisionActions struct {
	approve kit.IconPlaque
	reject  kit.IconPlaque
}

// Together presents every Listen Together room state and host control.
type Together struct {
	scroll      look.Scroller
	username    kit.TextField
	server      kit.TextField
	roomCode    kit.TextField
	create      kit.Plaque
	join        kit.Plaque
	copyInvite  kit.IconPlaque
	leave       kit.Plaque
	suggest     kit.Plaque
	requestSync kit.Plaque
	members     map[string]*togetherMemberActions
	joins       map[string]*togetherDecisionActions
	suggestions map[string]*togetherDecisionActions
	lastToast   uint64
	serverSet   bool
}

// Layout renders the current Listen Together state from the daemon mirror.
func (together *Together) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	together.forwardNotice(e, snapshot.Toast)
	return layout.Inset{Top: 13, Right: 13, Bottom: 13, Left: 13}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		switch {
		case snapshot.LT.Status == "connecting" || snapshot.LT.Requesting:
			return together.layoutConnecting(gtx, e, snapshot.LT.Requesting)
		case snapshot.LT.Status == "connected" && snapshot.LT.Role == "host":
			return together.layoutHost(gtx, e, snapshot.LT)
		case snapshot.LT.Status == "connected" && snapshot.LT.Role == "guest":
			return together.layoutGuest(gtx, e, snapshot)
		default:
			return together.layoutIdle(gtx, e, snapshot.LT)
		}
	})
}

func (together *Together) forwardNotice(e *env.Env, toast *state.Toast) {
	if toast == nil || toast.Serial == together.lastToast || e.Toast == nil {
		return
	}
	together.lastToast = toast.Serial
	kind := env.ToastInfo
	if toast.Kind == state.ToastError {
		kind = env.ToastError
	} else if toast.Kind == state.ToastSuccess {
		kind = env.ToastSuccess
	}
	e.Toast(toast.Message, kind)
}

func (together *Together) layoutIdle(gtx layout.Context, e *env.Env, lt model.LTState) layout.Dimensions {
	if !together.serverSet {
		together.server.SetText(strings.TrimSpace(lt.ServerURL))
		together.serverSet = true
	}
	return together.scroll.Layout(gtx, e.Th, 5, func(gtx layout.Context, index int) layout.Dimensions {
		switch index {
		case 0:
			return together.layoutIdentity(gtx, e)
		case 1:
			return layout.Spacer{Height: 13}.Layout(gtx)
		case 2:
			return together.layoutCreate(gtx, e, lt)
		case 3:
			return layout.Spacer{Height: 13}.Layout(gtx)
		default:
			return together.layoutJoin(gtx, e, lt)
		}
	})
}

func (together *Together) layoutIdentity(gtx layout.Context, e *env.Env) layout.Dimensions {
	return togetherSlab(gtx, e, func(gtx layout.Context) layout.Dimensions {
		together.username.Label = "Your name"
		together.username.Placeholder = "Name"
		together.server.Label = "Sync server"
		together.server.Placeholder = "wss://sync.example"
		together.server.Help = "Use a ws:// or wss:// relay address."
		together.server.Mono = true
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return together.username.Layout(gtx, e.Th.Theme) }),
			layout.Rigid(layout.Spacer{Height: 13}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return together.server.Layout(gtx, e.Th.Theme) }),
		)
	})
}

func (together *Together) layoutCreate(gtx layout.Context, e *env.Env, lt model.LTState) layout.Dimensions {
	return togetherSlab(gtx, e, func(gtx layout.Context) layout.Dimensions {
		together.create.Kind = kit.PlaqueLine
		together.create.Icon = look.Icon.Person
		together.create.Label = "Create room"
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, "Create a room")),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg2, MaxLines: 3}).Layout(gtx, e.Th.Theme, "Host playback and approve who joins.")
			}),
			layout.Rigid(layout.Spacer{Height: 13}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := together.create.Layout(gtx, e.Th.Theme)
				if together.create.Clicked(gtx) {
					together.createRoom(e, lt)
				}
				return dims
			}),
		)
	})
}

func (together *Together) layoutJoin(gtx layout.Context, e *env.Env, lt model.LTState) layout.Dimensions {
	return togetherSlab(gtx, e, func(gtx layout.Context) layout.Dimensions {
		together.roomCode.Label = "Invite or room code"
		together.roomCode.Placeholder = "Paste invite"
		together.roomCode.Mono = true
		together.join.Kind = kit.PlaqueLine
		together.join.Label = "Join"
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, "Join a room")),
			layout.Rigid(layout.Spacer{Height: 13}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return together.roomCode.Layout(gtx, e.Th.Theme) }),
			layout.Rigid(layout.Spacer{Height: 13}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := together.join.Layout(gtx, e.Th.Theme)
				if together.join.Clicked(gtx) {
					together.joinRoom(e, lt)
				}
				return dims
			}),
		)
	})
}

func (together *Together) layoutConnecting(gtx layout.Context, e *env.Env, requesting bool) layout.Dimensions {
	copy := "Connecting to the room."
	if requesting {
		copy = "Waiting for the host to approve your request."
	}
	working := kit.Working{Active: true, Size: 34}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return working.Layout(gtx, e.Th.Theme) }),
			layout.Rigid(layout.Spacer{Height: 13}.Layout),
			layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Proof, e.Th.R.Fg2, copy)),
		)
	})
}

func (together *Together) layoutHost(gtx layout.Context, e *env.Env, lt model.LTState) layout.Dimensions {
	rows := make([]layout.Widget, 0, 5+len(lt.Users)+len(lt.PendingJoins)+len(lt.Suggestions))
	rows = append(rows, together.roomHeader(e, lt))
	if len(lt.Users) > 0 {
		rows = append(rows, togetherSection(e, "Members", len(lt.Users)))
		for _, user := range lt.Users {
			rows = append(rows, together.memberRow(e, lt, user))
		}
	}
	if len(lt.PendingJoins) > 0 {
		rows = append(rows, togetherSection(e, "Pending joins", len(lt.PendingJoins)))
		for _, join := range lt.PendingJoins {
			rows = append(rows, together.joinRow(e, join))
		}
	}
	if len(lt.Suggestions) > 0 {
		rows = append(rows, togetherSection(e, "Suggestions", len(lt.Suggestions)))
		for _, suggestion := range lt.Suggestions {
			rows = append(rows, together.suggestionRow(e, suggestion))
		}
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		together.leave.Kind = kit.PlaqueDangerLine
		together.leave.Label = "Leave"
		dims := together.leave.Layout(gtx, e.Th.Theme)
		if together.leave.Clicked(gtx) {
			callVoid(e, protocol.MethodLTLeave, protocol.Null{})
		}
		return dims
	})
	return together.scroll.Layout(gtx, e.Th, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
		return layout.Inset{Bottom: 8}.Layout(gtx, rows[index])
	})
}

func (together *Together) roomHeader(e *env.Env, lt model.LTState) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		code := ""
		if lt.RoomCode != nil {
			code = strings.TrimSpace(*lt.RoomCode)
		}
		return togetherSlab(gtx, e, func(gtx layout.Context) layout.Dimensions {
			together.copyInvite.Kind = kit.PlaqueQuiet
			together.copyInvite.Icon = look.Icon.Share
			together.copyInvite.Description = "Copy invite"
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg3, "Room code")),
						layout.Rigid(layout.Spacer{Height: 4}.Layout),
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Mono, e.Th.R.Fg, code)),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := together.copyInvite.Layout(gtx, e.Th.Theme)
					if together.copyInvite.Clicked(gtx) && code != "" {
						invite := togetherInvite(lt.ServerURL, code)
						gtx.Execute(clipboard.WriteCmd{Type: "text/plain", Data: io.NopCloser(strings.NewReader(invite))})
						if e.Toast != nil {
							e.Toast("Invite copied, send it to a friend", env.ToastSuccess)
						}
					}
					return dims
				}),
			)
		})
	}
}

func togetherSection(e *env.Env, title string, count int) layout.Widget {
	return kit.GroupHeader(e.Th.Theme, title, count)
}

func togetherSlab(gtx layout.Context, e *env.Env, body layout.Widget) layout.Dimensions {
	return kit.Slab(gtx, e.Th.R.Stone2, e.Th.Cut.Mid, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(13).Layout(gtx, body)
	})
}

func (together *Together) memberRow(e *env.Env, lt model.LTState, user model.LTUser) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		actions := together.memberActions(user.UserID)
		status, tone := "Connected", palette.Alive
		if !user.IsConnected {
			status, tone = "Disconnected", palette.Idle
		}
		if user.IsHost {
			status = "Host · " + status
		}
		return togetherSlab(gtx, e, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(4)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Row, e.Th.R.Fg, user.Username)),
						layout.Rigid(kit.Claim(e.Th.Theme, tone, status)),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if lt.MyID != nil && user.UserID == *lt.MyID {
						return layout.Dimensions{}
					}
					actions.transfer.Kind = kit.PlaqueQuiet
					actions.transfer.Label = "Transfer"
					actions.transfer.Small = true
					dims := actions.transfer.Layout(gtx, e.Th.Theme)
					if actions.transfer.Clicked(gtx) {
						callVoid(e, protocol.MethodLTTransferHost, protocol.LTUserParams{UserID: user.UserID})
					}
					return dims
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if lt.MyID != nil && user.UserID == *lt.MyID {
						return layout.Dimensions{}
					}
					actions.kick.Kind = kit.PlaqueDangerLine
					actions.kick.Label = "Kick"
					actions.kick.Small = true
					dims := actions.kick.Layout(gtx, e.Th.Theme)
					if actions.kick.Clicked(gtx) {
						callVoid(e, protocol.MethodLTKick, protocol.LTUserParams{UserID: user.UserID})
					}
					return dims
				}),
			)
		})
	}
}

func (together *Together) joinRow(e *env.Env, join model.LTPendingJoin) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		actions := together.joinActions(join.UserID)
		return togetherDecision(gtx, e, join.Username, "Wants to join", actions, func() {
			callVoid(e, protocol.MethodLTApproveJoin, protocol.LTUserParams{UserID: join.UserID})
		}, func() {
			callVoid(e, protocol.MethodLTRejectJoin, protocol.LTUserParams{UserID: join.UserID})
		})
	}
}

func (together *Together) suggestionRow(e *env.Env, suggestion model.LTSuggestion) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		actions := together.suggestionActions(suggestion.ID)
		detail := strings.TrimSpace(suggestion.Track.Artist)
		if suggestion.FromUsername != "" {
			if detail != "" {
				detail += " · "
			}
			detail += "Suggested by " + suggestion.FromUsername
		}
		return togetherDecision(gtx, e, suggestion.Track.Title, detail, actions, func() {
			callVoid(e, protocol.MethodLTApproveSuggestion, protocol.LTSuggestionParams{ID: suggestion.ID})
		}, func() {
			callVoid(e, protocol.MethodLTRejectSuggestion, protocol.LTSuggestionParams{ID: suggestion.ID})
		})
	}
}

func togetherDecision(gtx layout.Context, e *env.Env, title, detail string, actions *togetherDecisionActions, approve, reject func()) layout.Dimensions {
	return togetherSlab(gtx, e, func(gtx layout.Context) layout.Dimensions {
		actions.approve.Kind = kit.PlaqueQuiet
		actions.approve.Icon = look.Icon.Check
		actions.approve.Description = "Approve"
		actions.approve.Small = true
		actions.reject.Kind = kit.PlaqueQuiet
		actions.reject.Icon = look.Icon.Close
		actions.reject.Description = "Reject"
		actions.reject.Small = true
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(4)}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Row, e.Th.R.Fg, title)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return (kit.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg3, MaxLines: 2}).Layout(gtx, e.Th.Theme, detail)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := actions.approve.Layout(gtx, e.Th.Theme)
				if actions.approve.Clicked(gtx) {
					approve()
				}
				return dims
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := actions.reject.Layout(gtx, e.Th.Theme)
				if actions.reject.Clicked(gtx) {
					reject()
				}
				return dims
			}),
		)
	})
}

func (together *Together) layoutGuest(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	host := "The host"
	for _, user := range snapshot.LT.Users {
		if user.IsHost {
			host = user.Username
			break
		}
	}
	return together.scroll.Layout(gtx, e.Th, 4, func(gtx layout.Context, index int) layout.Dimensions {
		switch index {
		case 0:
			return togetherSlab(gtx, e, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Caption, e.Th.R.Fg3, "Host")),
					layout.Rigid(layout.Spacer{Height: 4}.Layout),
					layout.Rigid(kit.Text(e.Th.Theme, e.Th.T.Section, e.Th.R.Fg, host)),
				)
			})
		case 1:
			together.suggest.Kind = kit.PlaqueLine
			together.suggest.Icon = look.Icon.PlaylistAdd
			together.suggest.Label = "Suggest current track"
			enabled := snapshot.Playback.Now != nil
			buttonGTX := gtx
			if !enabled {
				buttonGTX = buttonGTX.Disabled()
			}
			dims := together.suggest.Layout(buttonGTX, e.Th.Theme)
			if enabled && together.suggest.Clicked(gtx) {
				callVoid(e, protocol.MethodLTSuggest, protocol.LTSuggestParams{Item: currentTogetherTrack(snapshot)})
			}
			return dims
		case 2:
			together.requestSync.Kind = kit.PlaqueQuiet
			together.requestSync.Icon = look.Icon.Refresh
			together.requestSync.Label = "Request sync"
			dims := together.requestSync.Layout(gtx, e.Th.Theme)
			if together.requestSync.Clicked(gtx) {
				callVoid(e, protocol.MethodLTRequestSync, protocol.Null{})
			}
			return dims
		default:
			together.leave.Kind = kit.PlaqueDangerLine
			together.leave.Label = "Leave"
			dims := together.leave.Layout(gtx, e.Th.Theme)
			if together.leave.Clicked(gtx) {
				callVoid(e, protocol.MethodLTLeave, protocol.Null{})
			}
			return dims
		}
	})
}

func currentTogetherTrack(snapshot state.State) model.Track {
	var current *model.Track
	if snapshot.Queue.CurrentIndex >= 0 && snapshot.Queue.CurrentIndex < len(snapshot.Queue.Items) {
		current = &snapshot.Queue.Items[snapshot.Queue.CurrentIndex]
	}
	return trackFromPlayback(snapshot.Playback.Now, current)
}

func (together *Together) createRoom(e *env.Env, lt model.LTState) {
	username := strings.TrimSpace(together.username.Text())
	if username == "" {
		showTogetherError(e, "Enter a name first")
		return
	}
	together.withServer(e, lt, func() {
		callVoid(e, protocol.MethodLTCreateRoom, protocol.LTCreateRoomParams{Username: username})
	})
}

func (together *Together) joinRoom(e *env.Env, lt model.LTState) {
	username := strings.TrimSpace(together.username.Text())
	if username == "" {
		showTogetherError(e, "Enter a name first")
		return
	}
	raw := strings.TrimSpace(together.roomCode.Text())
	if raw == "" {
		showTogetherError(e, "Paste the invite code your friend sent")
		return
	}
	code, inviteServer, full, valid := parseTogetherInvite(raw)
	if full && !valid {
		showTogetherError(e, "The invite does not contain a valid sync server")
		return
	}
	if inviteServer != "" {
		together.server.SetText(inviteServer)
	}
	if code == "" {
		showTogetherError(e, "Paste the full invite from the host, it carries the server address")
		return
	}
	together.withServer(e, lt, func() {
		callVoid(e, protocol.MethodLTJoinRoom, protocol.LTJoinRoomParams{Code: code, Username: username})
	})
}

func (together *Together) withServer(e *env.Env, lt model.LTState, then func()) {
	server := strings.TrimSpace(together.server.Text())
	if server == "" {
		showTogetherError(e, "Enter your sync server URL")
		return
	}
	if !validTogetherServer(server) {
		showTogetherError(e, "Use a ws:// or wss:// sync server address")
		return
	}
	if server == strings.TrimSpace(lt.ServerURL) {
		then()
		return
	}
	act.Call[protocol.Null](e, protocol.MethodLTSetServerURL, protocol.LTServerURLParams{URL: server}, func(_ protocol.Null, err error) {
		if err != nil {
			showTogetherError(e, err.Error())
			return
		}
		then()
	})
}

func validTogetherServer(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "ws" || parsed.Scheme == "wss") && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == ""
}

func togetherInvite(server, code string) string {
	server = strings.TrimSpace(server)
	if parsed, err := url.Parse(server); err == nil {
		parsed.Fragment = "room=" + strings.TrimSpace(code)
		return parsed.String()
	}
	return strings.TrimSpace(code)
}

func parseTogetherInvite(raw string) (code, server string, full, valid bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		return strings.TrimSpace(raw), "", false, true
	}
	full = true
	fragment := strings.TrimSpace(parsed.Fragment)
	fragment = strings.TrimPrefix(fragment, "room=")
	parsed.Fragment = ""
	server = parsed.String()
	valid = fragment != "" && validTogetherServer(server)
	return fragment, server, full, valid
}

func (together *Together) memberActions(id string) *togetherMemberActions {
	if together.members == nil {
		together.members = make(map[string]*togetherMemberActions)
	}
	if together.members[id] == nil {
		together.members[id] = new(togetherMemberActions)
	}
	return together.members[id]
}

func (together *Together) joinActions(id string) *togetherDecisionActions {
	if together.joins == nil {
		together.joins = make(map[string]*togetherDecisionActions)
	}
	if together.joins[id] == nil {
		together.joins[id] = new(togetherDecisionActions)
	}
	return together.joins[id]
}

func (together *Together) suggestionActions(id string) *togetherDecisionActions {
	if together.suggestions == nil {
		together.suggestions = make(map[string]*togetherDecisionActions)
	}
	if together.suggestions[id] == nil {
		together.suggestions[id] = new(togetherDecisionActions)
	}
	return together.suggestions[id]
}

func showTogetherError(e *env.Env, message string) {
	if e.Toast != nil {
		e.Toast(message, env.ToastError)
	}
}
