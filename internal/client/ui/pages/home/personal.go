package home

import (
	"encoding/json"

	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/act"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type personalSaver struct {
	saving bool
	queued bool
}

func (p *page) mutatePersonal(e *env.Env, update func(*state.Personal) bool) bool {
	if !e.Store.UpdatePersonal(update) {
		return false
	}
	p.saver.queue(e)
	return true
}

func (s *personalSaver) queue(e *env.Env) {
	if s.saving {
		s.queued = true
		return
	}
	s.start(e)
}

func (s *personalSaver) start(e *env.Env) {
	raw, err := state.MarshalPersonal(e.Store.Snapshot().Personal)
	if err != nil {
		e.Toast("Could not save", env.ToastError)
		return
	}
	e.Store.MarkPersonalSave(json.RawMessage(raw))
	s.saving = true
	s.queued = false
	act.Call[protocol.Null](e, protocol.MethodSetPersonal, protocol.SetPersonalParams{Personal: raw}, func(_ protocol.Null, err error) {
		s.saving = false
		if err != nil {
			e.Toast("Could not save", env.ToastError)
		}
		if s.queued {
			s.start(e)
		}
	})
}
