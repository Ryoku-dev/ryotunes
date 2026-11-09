package shell

import (
	"context"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func (s *uiShell) layoutOverlays(gtx layout.Context, e *env.Env) {
	s.itemMenu.LayoutDialogs(gtx, e)
	s.layoutConfirm(gtx, e)
	s.placards.Layout(gtx, e.Th.Theme)
	s.overlays.Layout(gtx, e.Th)
}

func (s *uiShell) layoutConfirm(gtx layout.Context, e *env.Env) {
	request := s.confirmReq
	if request == nil {
		return
	}
	if s.confirm.Layout(gtx, e.Th.Theme, request.title, request.body, request.action, request.required) {
		run := request.run
		s.confirmReq = nil
		if run != nil {
			run()
		}
	}
	if s.confirm.Closed() {
		s.confirmReq = nil
	}
}

func (s *uiShell) callNull(e *env.Env, method string) {
	s.call(e, method, protocol.Null{})
}

func (s *uiShell) call(e *env.Env, method string, params any) {
	e.Go(func(ctx context.Context) {
		if err := e.Conn.Call(ctx, method, params, nil); err != nil {
			e.Toast(err.Error(), env.ToastError)
		}
	})
}

func (s *uiShell) cycleRepeat(e *env.Env) {
	mode := e.Store.Snapshot().Queue.Repeat
	next := model.RepeatAll
	switch mode {
	case model.RepeatAll:
		next = model.RepeatOne
	case model.RepeatOne:
		next = model.RepeatOff
	}
	s.call(e, protocol.MethodSetRepeat, protocol.SetRepeatParams{Mode: next})
}
