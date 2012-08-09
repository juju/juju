package uniter

import (
	"fmt"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/presence"
)

// Relationer manages a unit's presence in a relation.
type Relationer struct {
	ctx      *server.RelationContext
	ru       *state.RelationUnit
	rs       *RelationState
	pinger   *presence.Pinger
	queue    hookQueue
	hooks    chan<- HookInfo
	breaking bool
}

// NewRelationer creates a new Relationer. The unit will not join the
// relation until explicitly requested.
func NewRelationer(ru *state.RelationUnit, rs *RelationState, hooks chan<- HookInfo) *Relationer {
	// TODO lifecycle handling?
	return &Relationer{
		ctx:   server.NewRelationContext(ru, rs.Members),
		ru:    ru,
		rs:    rs,
		hooks: hooks,
	}
}

// Join starts the periodic signalling of the unit's presence in the relation.
// It must not be called again until Abandon has been called.
func (r *Relationer) Join() error {
	if r.pinger != nil {
		panic("unit already joined!")
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

// Breaking informs the relationer that the unit is departing the relation,
// and that the only hooks it should send henceforth are -departed hooks,
// until the relation is empty, followed by a -broken hook.
func (r *Relationer) Breaking() error {
	if err := r.Abandon(); err != nil {
		return err
	}
	if r.queue != nil {
		if err := r.StopHooks(); err != nil {
			return err
		}
		defer r.StartHooks()
	}
	r.breaking = true
	return nil
}

// StartHooks starts watching the relation, and sending HookInfo events on the
// hooks channel. It will panic if called when already responding to relation
// changes.
func (r *Relationer) StartHooks() {
	if r.queue != nil {
		panic("hooks already started!")
	}
	if r.breaking {
		r.queue = NewBrokenHookQueue(r.rs, r.hooks)
	} else {
		r.queue = NewHookQueue(r.rs, r.hooks, r.ru.Watch())
	}
}

// StopHooks ensures that the relationer is not watching the relation, or sending
// HookInfo events on the hooks channel.
func (r *Relationer) StopHooks() error {
	if r.queue == nil {
		return nil
	}
	queue := r.queue
	r.queue = nil
	return queue.Stop()
}

// Context returns the RelationContext associated with r.
func (r *Relationer) Context() *server.RelationContext {
	return r.ctx
}

// PrepareHook checks that the relation is in a state such that it makes
// sense to execute the supplied hook, and ensures that the relation context
// contains the latest relation state as communicated in the HookInfo. It
// returns the name of the hook that must be run.
func (r *Relationer) PrepareHook(hi HookInfo) (hookName string, err error) {
	if err = r.rs.Validate(hi); err != nil {
		return "", err
	}
	if hi.HookKind == "departed" && r.breaking {
		// We're using a BrokenHookQueue, which does not send membership
		// information; it has no access to state, and can't send valid
		// settings. To avoid dropping cached settings for any remaining
		// members, we just delete the one we know is going away.
		r.ctx.DelMember(hi.RemoteUnit)
	} else {
		r.ctx.SetMembers(hi.Members)
	}
	relName := r.ru.Endpoint().RelationName
	return fmt.Sprintf("%s-relation-%s", relName, hi.HookKind), nil
}

// CommitHook persists the fact of the supplied hook's completion.
func (r *Relationer) CommitHook(hi HookInfo) error {
	if hi.HookKind == "broken" {
		if err := r.ru.Depart(); err != nil {
			return err
		}
	}
	return r.rs.Commit(hi)
}
