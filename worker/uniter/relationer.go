package uniter

import (
	"fmt"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/juju-core/worker/uniter/relation"
)

// Relationer manages a unit's presence in a relation.
type Relationer struct {
	ctx    *server.RelationContext
	ru     *state.RelationUnit
	dir    *relation.StateDir
	pinger *presence.Pinger
	queue  relation.HookQueue
	hooks  chan<- hook.Info
	dying  bool
}

// NewRelationer creates a new Relationer. The unit will not join the
// relation until explicitly requested.
func NewRelationer(ru *state.RelationUnit, dir *relation.StateDir, hooks chan<- hook.Info) *Relationer {
	return &Relationer{
		ctx:   server.NewRelationContext(ru, dir.State().Members),
		ru:    ru,
		dir:   dir,
		hooks: hooks,
	}
}

// Context returns the RelationContext associated with r.
func (r *Relationer) Context() *server.RelationContext {
	return r.ctx
}

// Join starts the periodic signalling of the unit's presence in the relation.
// It must not be called again until Abandon has been called, and must not be
// called at all if Breaking has been called.
func (r *Relationer) Join() error {
	if r.pinger != nil {
		panic("unit already joined!")
	}
	if r.dying {
		panic("dying relationer must not join!")
	}
	pinger, err := r.ru.Join()
	if err != nil {
		return err
	}
	r.pinger = pinger
	return nil
}

// Abandon stops the periodic signalling of the unit's presence in the relation.
// It does not immediately signal that the unit has departed the relation; this
// is done only when a -broken hook is committed.
func (r *Relationer) Abandon() error {
	if r.pinger == nil {
		return nil
	}
	pinger := r.pinger
	r.pinger = nil
	return pinger.Stop()
}

// SetDying informs the relationer that the unit is departing the relation,
// and that the only hooks it should send henceforth are -departed hooks,
// until the relation is empty, followed by a -broken hook.
func (r *Relationer) SetDying() error {
	if err := r.Abandon(); err != nil {
		return err
	}
	if r.queue != nil {
		if err := r.StopHooks(); err != nil {
			return err
		}
		defer r.StartHooks()
	}
	r.dying = true
	return nil
}

// StartHooks starts watching the relation, and sending hook.Info events on the
// hooks channel. It will panic if called when already responding to relation
// changes.
func (r *Relationer) StartHooks() {
	if r.queue != nil {
		panic("hooks already started!")
	}
	if r.dying {
		r.queue = relation.NewDyingHookQueue(r.dir.State(), r.hooks)
	} else {
		r.queue = relation.NewAliveHookQueue(r.dir.State(), r.hooks, r.ru.Watch())
	}
}

// StopHooks ensures that the relationer is not watching the relation, or sending
// hook.Info events on the hooks channel.
func (r *Relationer) StopHooks() error {
	if r.queue == nil {
		return nil
	}
	queue := r.queue
	r.queue = nil
	return queue.Stop()
}

// PrepareHook checks that the relation is in a state such that it makes
// sense to execute the supplied hook, and ensures that the relation context
// contains the latest relation state as communicated in the hook.Info. It
// returns the name of the hook that must be run.
func (r *Relationer) PrepareHook(hi hook.Info) (hookName string, err error) {
	if err = r.dir.State().Validate(hi); err != nil {
		return "", err
	}
	r.ctx.UpdateMembers(hi.Members)
	if hi.Kind == hook.RelationDeparted {
		r.ctx.DeleteMember(hi.RemoteUnit)
	}
	name := r.ru.Endpoint().RelationName
	return fmt.Sprintf("%s-%s", name, hi.Kind), nil
}

// CommitHook persists the fact of the supplied hook's completion.
func (r *Relationer) CommitHook(hi hook.Info) error {
	if hi.Kind == hook.RelationBroken {
		if err := r.ru.Depart(); err != nil {
			return err
		}
	}
	return r.dir.Write(hi)
}
