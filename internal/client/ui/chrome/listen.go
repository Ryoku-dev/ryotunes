package chrome

import (
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/lithic"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type listenMemberActions struct {
	kick     lithic.Button
	transfer lithic.Button
}

type listenDecisionActions struct {
	approve lithic.Button
	reject  lithic.Button
}

// ListenPanel presents every Listen Together room state and host control.
type ListenPanel struct {
	list        lithic.List
	username    lithic.TextField
	roomCode    lithic.TextField
	create      lithic.Button
	join        lithic.Button
	settings    lithic.Button
	copyCode    lithic.IconButton
	leave       lithic.Button
	suggest     lithic.Button
	requestSync lithic.Button
	members     map[string]*listenMemberActions
	joins       map[string]*listenDecisionActions
	suggestions map[string]*listenDecisionActions
	lastToast   uint64
}

// Layout renders the current Listen Together state from the daemon mirror.
func (p *ListenPanel) Layout(gtx layout.Context, e *env.Env) layout.Dimensions {
	snapshot := e.Store.Snapshot()
	p.forwardNotice(e, snapshot.Toast)
	return layout.UniformInset(e.Th.Sp.S4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(lithic.Text(e.Th, e.Th.T.Section, e.Th.R.Fg, "Listen Together")),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				switch {
				case snapshot.LT.Status == "connecting" || snapshot.LT.Requesting:
					return p.layoutConnecting(gtx, e, snapshot.LT.Requesting)
				case snapshot.LT.Status == "connected" && snapshot.LT.Role == "host":
					return p.layoutHost(gtx, e, snapshot.LT)
				case snapshot.LT.Status == "connected" && snapshot.LT.Role == "guest":
					return p.layoutGuest(gtx, e, snapshot)
				default:
					return p.layoutIdle(gtx, e, snapshot.LT)
				}
			}),
		)
	})
}

func (p *ListenPanel) forwardNotice(e *env.Env, toast *state.Toast) {
	if toast == nil || toast.Serial == p.lastToast || e.Toast == nil {
		return
	}
	p.lastToast = toast.Serial
	kind := env.ToastInfo
	if toast.Kind == state.ToastError {
		kind = env.ToastError
	} else if toast.Kind == state.ToastSuccess {
		kind = env.ToastSuccess
	}
	e.Toast(toast.Message, kind)
}

func (p *ListenPanel) layoutIdle(gtx layout.Context, e *env.Env, lt model.LTState) layout.Dimensions {
	return p.list.Layout(gtx, e.Th, 4, func(gtx layout.Context, index int) layout.Dimensions {
		switch index {
		case 0:
			return lithic.Slab(gtx, e.Th.R.Stone2, e.Th.Cut.Mid, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, "Create a room")),
					layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.username.Layout(gtx, e.Th, "Your name", "Name", "", "")
					}),
					layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dims := p.create.Layout(gtx, e.Th, lithic.Gold, lithic.Icon("group"), "Create room")
						if p.create.Clicked(gtx) {
							p.createRoom(e, lt)
						}
						return dims
					}),
				)
			})
		case 1:
			return layout.Spacer{Height: e.Th.Sp.S3}.Layout(gtx)
		case 2:
			return lithic.Slab(gtx, e.Th.R.Stone2, e.Th.Cut.Mid, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, "Join a room")),
					layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						p.roomCode.Mono = true
						return p.roomCode.Layout(gtx, e.Th, "Room code", "Code", "", "")
					}),
					layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dims := p.join.Layout(gtx, e.Th, lithic.Line, lithic.Icon("login"), "Join")
						if p.join.Clicked(gtx) {
							p.joinRoom(e, lt)
						}
						return dims
					}),
				)
			})
		default:
			return p.serverHint(gtx, e, lt.ServerURL)
		}
	})
}

func (p *ListenPanel) serverHint(gtx layout.Context, e *env.Env, serverURL string) layout.Dimensions {
	copy := "The relay server is set in Settings."
	if strings.TrimSpace(serverURL) == "" {
		copy = "Set a ws:// or wss:// relay server in Settings before creating or joining a room."
	}
	return layout.Inset{Top: e.Th.Sp.S3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (lithic.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg3, MaxLines: 3}).Layout(gtx, e.Th, copy)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if strings.TrimSpace(serverURL) == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: e.Th.Sp.S1}.Layout(gtx, lithic.Text(e.Th, e.Th.T.MonoSm, e.Th.R.Fg2, serverURL))
			}),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S2}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				p.settings.Small = true
				dims := p.settings.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("settings"), "Open settings")
				if p.settings.Clicked(gtx) {
					e.Nav.Push(env.Route{Kind: env.RouteSettings, Params: "integrations"})
				}
				return dims
			}),
		)
	})
}

func (p *ListenPanel) layoutConnecting(gtx layout.Context, e *env.Env, requesting bool) layout.Dimensions {
	copy := "Connecting to the room."
	if requesting {
		copy = "Waiting for the host to approve your request."
	}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return lithic.Working(gtx, e.Th, 34) }),
			layout.Rigid(layout.Spacer{Height: e.Th.Sp.S3}.Layout),
			layout.Rigid(lithic.Text(e.Th, e.Th.T.Body, e.Th.R.Fg2, copy)),
		)
	})
}

func (p *ListenPanel) layoutHost(gtx layout.Context, e *env.Env, lt model.LTState) layout.Dimensions {
	rows := make([]layout.Widget, 0, 5+len(lt.Users)+len(lt.PendingJoins)+len(lt.Suggestions))
	rows = append(rows, p.roomHeader(e, lt))
	if len(lt.Users) > 0 {
		rows = append(rows, sectionLabel(e, "Members"))
		for i := range lt.Users {
			user := lt.Users[i]
			rows = append(rows, p.memberRow(e, lt, user))
		}
	}
	if len(lt.PendingJoins) > 0 {
		rows = append(rows, sectionLabel(e, "Pending joins"))
		for i := range lt.PendingJoins {
			join := lt.PendingJoins[i]
			rows = append(rows, p.joinRow(e, join))
		}
	}
	if len(lt.Suggestions) > 0 {
		rows = append(rows, sectionLabel(e, "Suggestions"))
		for i := range lt.Suggestions {
			suggestion := lt.Suggestions[i]
			rows = append(rows, p.suggestionRow(e, suggestion))
		}
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: e.Th.Sp.S4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			dims := p.leave.Layout(gtx, e.Th, lithic.DangerLine, lithic.Icon("logout"), "Leave")
			if p.leave.Clicked(gtx) {
				listenCall(e, protocol.MethodLTLeave, protocol.Null{})
			}
			return dims
		})
	})
	return p.list.Layout(gtx, e.Th, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
		return layout.Inset{Bottom: e.Th.Sp.S2}.Layout(gtx, rows[index])
	})
}

func (p *ListenPanel) roomHeader(e *env.Env, lt model.LTState) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		code := ""
		if lt.RoomCode != nil {
			code = *lt.RoomCode
		}
		return lithic.Slab(gtx, e.Th.R.Stone2, e.Th.Cut.Mid, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Fg3, "Room code")),
						layout.Rigid(layout.Spacer{Height: e.Th.Sp.S1}.Layout),
						layout.Rigid(lithic.Text(e.Th, e.Th.T.Mono, e.Th.R.Fg, code)),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := p.copyCode.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("content_copy"), "Copy room code")
					if p.copyCode.Clicked(gtx) && code != "" {
						gtx.Execute(clipboard.WriteCmd{Type: "text/plain", Data: io.NopCloser(strings.NewReader(code))})
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

func sectionLabel(e *env.Env, title string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: e.Th.Sp.S3, Bottom: e.Th.Sp.S1}.Layout(gtx, lithic.Text(e.Th, e.Th.T.Group, e.Th.R.Fg3, title))
	}
}

func (p *ListenPanel) memberRow(e *env.Env, lt model.LTState, user model.LTUser) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		actions := p.memberActions(user.UserID)
		status := "Connected"
		tone := lithic.Alive
		if !user.IsConnected {
			status = "Disconnected"
			tone = lithic.Idle
		}
		if user.IsHost {
			status = "Host · " + status
		}
		return lithic.Slab(gtx, e.Th.R.Stone2, e.Th.Cut.Mid, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(lithic.Text(e.Th, e.Th.T.Row, e.Th.R.Fg, user.Username)),
						layout.Rigid(statusLine(e, tone, status)),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if lt.MyID != nil && user.UserID == *lt.MyID {
						return layout.Dimensions{}
					}
					actions.transfer.Small = true
					dims := actions.transfer.Layout(gtx, e.Th, lithic.Quiet, nil, "Transfer")
					if actions.transfer.Clicked(gtx) {
						listenCall(e, protocol.MethodLTTransferHost, protocol.LTUserParams{UserID: user.UserID})
					}
					return dims
				}),
				layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if lt.MyID != nil && user.UserID == *lt.MyID {
						return layout.Dimensions{}
					}
					actions.kick.Small = true
					dims := actions.kick.Layout(gtx, e.Th, lithic.DangerLine, nil, "Kick")
					if actions.kick.Clicked(gtx) {
						listenCall(e, protocol.MethodLTKick, protocol.LTUserParams{UserID: user.UserID})
					}
					return dims
				}),
			)
		})
	}
}

func statusLine(e *env.Env, tone lithic.Tone, word string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return lithic.Flake(gtx, e.Th, tone, 1, 9)
			}),
			layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
			layout.Rigid(lithic.Claim(e.Th, tone, word)),
		)
	}
}

func (p *ListenPanel) joinRow(e *env.Env, join model.LTPendingJoin) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		actions := p.joinActions(join.UserID)
		return decisionTablet(gtx, e, join.Username, "Wants to join", actions, func() {
			listenCall(e, protocol.MethodLTApproveJoin, protocol.LTUserParams{UserID: join.UserID})
		}, func() {
			listenCall(e, protocol.MethodLTRejectJoin, protocol.LTUserParams{UserID: join.UserID})
		})
	}
}

func (p *ListenPanel) suggestionRow(e *env.Env, suggestion model.LTSuggestion) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		actions := p.suggestionActions(suggestion.ID)
		detail := suggestion.Track.Artist
		if suggestion.FromUsername != "" {
			detail += " · Suggested by " + suggestion.FromUsername
		}
		return decisionTablet(gtx, e, suggestion.Track.Title, detail, actions, func() {
			listenCall(e, protocol.MethodLTApproveSuggestion, protocol.LTSuggestionParams{ID: suggestion.ID})
		}, func() {
			listenCall(e, protocol.MethodLTRejectSuggestion, protocol.LTSuggestionParams{ID: suggestion.ID})
		})
	}
}

func decisionTablet(gtx layout.Context, e *env.Env, title, detail string, actions *listenDecisionActions, approve, reject func()) layout.Dimensions {
	return lithic.Slab(gtx, e.Th.R.Stone2, e.Th.Cut.Mid, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(lithic.Text(e.Th, e.Th.T.Row, e.Th.R.Fg, title)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return (lithic.Label{Style: e.Th.T.Caption, Color: e.Th.R.Fg3, MaxLines: 2}).Layout(gtx, e.Th, detail)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				actions.approve.Small = true
				dims := actions.approve.Layout(gtx, e.Th, lithic.Line, lithic.Icon("check"), "Approve")
				if actions.approve.Clicked(gtx) {
					approve()
				}
				return dims
			}),
			layout.Rigid(layout.Spacer{Width: e.Th.Sp.S2}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				actions.reject.Small = true
				dims := actions.reject.Layout(gtx, e.Th, lithic.DangerLine, lithic.Icon("close"), "Reject")
				if actions.reject.Clicked(gtx) {
					reject()
				}
				return dims
			}),
		)
	})
}

func (p *ListenPanel) layoutGuest(gtx layout.Context, e *env.Env, snapshot state.State) layout.Dimensions {
	host := "The host"
	for i := range snapshot.LT.Users {
		if snapshot.LT.Users[i].IsHost {
			host = snapshot.LT.Users[i].Username
			break
		}
	}
	return p.list.Layout(gtx, e.Th, 4, func(gtx layout.Context, index int) layout.Dimensions {
		switch index {
		case 0:
			return lithic.Slab(gtx, e.Th.R.Stone2, e.Th.Cut.Mid, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(lithic.Text(e.Th, e.Th.T.Caption, e.Th.R.Fg3, "Host")),
					layout.Rigid(layout.Spacer{Height: e.Th.Sp.S1}.Layout),
					layout.Rigid(lithic.Text(e.Th, e.Th.T.Title, e.Th.R.Fg, host)),
				)
			})
		case 1:
			return layout.Inset{Top: e.Th.Sp.S3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				enabled := snapshot.Playback.Now != nil
				ctx := gtx
				if !enabled {
					ctx = ctx.Disabled()
				}
				dims := p.suggest.Layout(ctx, e.Th, lithic.Line, lithic.Icon("playlist_add"), "Suggest current track")
				if enabled && p.suggest.Clicked(gtx) {
					listenCall(e, protocol.MethodLTSuggest, protocol.LTSuggestParams{Item: currentTrack(snapshot)})
				}
				return dims
			})
		case 2:
			return layout.Inset{Top: e.Th.Sp.S2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := p.requestSync.Layout(gtx, e.Th, lithic.Quiet, lithic.Icon("refresh"), "Request sync")
				if p.requestSync.Clicked(gtx) {
					listenCall(e, protocol.MethodLTRequestSync, protocol.Null{})
				}
				return dims
			})
		default:
			return layout.Inset{Top: e.Th.Sp.S4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := p.leave.Layout(gtx, e.Th, lithic.DangerLine, lithic.Icon("logout"), "Leave")
				if p.leave.Clicked(gtx) {
					listenCall(e, protocol.MethodLTLeave, protocol.Null{})
				}
				return dims
			})
		}
	})
}

func currentTrack(snapshot state.State) model.Track {
	var current *model.Track
	if snapshot.Queue.CurrentIndex >= 0 && snapshot.Queue.CurrentIndex < len(snapshot.Queue.Items) {
		current = &snapshot.Queue.Items[snapshot.Queue.CurrentIndex]
	}
	return trackFromPlayback(snapshot.Playback.Now, current)
}

func (p *ListenPanel) createRoom(e *env.Env, lt model.LTState) {
	username := strings.TrimSpace(p.username.Text())
	if username == "" {
		showListenError(e, "Enter a name first")
		return
	}
	if strings.TrimSpace(lt.ServerURL) == "" {
		showListenError(e, "Enter your sync server URL")
		return
	}
	listenCall(e, protocol.MethodLTCreateRoom, protocol.LTCreateRoomParams{Username: username})
}

func (p *ListenPanel) joinRoom(e *env.Env, lt model.LTState) {
	username := strings.TrimSpace(p.username.Text())
	if username == "" {
		showListenError(e, "Enter a name first")
		return
	}
	code := strings.TrimSpace(p.roomCode.Text())
	if code == "" {
		showListenError(e, "Paste the invite code your friend sent")
		return
	}
	if strings.TrimSpace(lt.ServerURL) == "" {
		showListenError(e, "Enter your sync server URL")
		return
	}
	listenCall(e, protocol.MethodLTJoinRoom, protocol.LTJoinRoomParams{Code: code, Username: username})
}

func (p *ListenPanel) memberActions(id string) *listenMemberActions {
	if p.members == nil {
		p.members = make(map[string]*listenMemberActions)
	}
	if p.members[id] == nil {
		p.members[id] = new(listenMemberActions)
	}
	return p.members[id]
}

func (p *ListenPanel) joinActions(id string) *listenDecisionActions {
	if p.joins == nil {
		p.joins = make(map[string]*listenDecisionActions)
	}
	if p.joins[id] == nil {
		p.joins[id] = new(listenDecisionActions)
	}
	return p.joins[id]
}

func (p *ListenPanel) suggestionActions(id string) *listenDecisionActions {
	if p.suggestions == nil {
		p.suggestions = make(map[string]*listenDecisionActions)
	}
	if p.suggestions[id] == nil {
		p.suggestions[id] = new(listenDecisionActions)
	}
	return p.suggestions[id]
}

func listenCall(e *env.Env, method string, params any) {
	callVoid(e, method, params)
}

func showListenError(e *env.Env, message string) {
	if e.Toast != nil {
		e.Toast(message, env.ToastError)
	}
}
