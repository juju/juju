package uniter

import (
	"fmt"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/state"
)

// Relationer manages a unit's presence in a relation.
type Relationer struct {
	ctx      *server.RelationContext
	ru       *state.RelationUnit
	rs       *RelationState
	queue    hookQueue
	hooks    chan<- HookInfo
	breaking bool
}

// NewRelationer creates a new Relationer. The unit will not join the
// relation until explicitly requested.
func NewRelationer(ru *state.RelationUnit, rs *RelationState, hooks chan<- HookInfo) *Relationer {
	return &Relationer{
		ctx:   server.NewRelationContext(ru, rs.Members),
		ru:    ru,
		rs:    rs,
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
	if r.breaking {
		panic("breaking unit must not join!")
	}
	if err := r.ru.Init(); err != nil {
		return err
	}
	return r.ru.Pinger().Start()
}

// Abandon stops the periodic signalling of the unit's presence in the relation.
// It does not immediately signal that the unit has departed the relation; this
// is done only when a -broken hook is committed.
func (r *Relationer) Abandon() error {
	return r.ru.Pinger().Stop()
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

// PrepareHook checks that the relation is in a state such that it makes
// sense to execute the supplied hook, and ensures that the relation context
// contains the latest relation state as communicated in the HookInfo. It
// returns the name of the hook that must be run.
func (r *Relationer) PrepareHook(hi HookInfo) (hookName string, err error) {
	if err = r.rs.Validate(hi); err != nil {
		return "", err
	}
	r.ctx.UpdateMembers(hi.Members)
	if hi.HookKind == "departed" {
		r.ctx.DeleteMember(hi.RemoteUnit)
	}
	relName := r.ru.Endpoint().RelationName
	return fmt.Sprintf("%s-relation-%s", relName, hi.HookKind), nil
}

// CommitHook persists the fact of the supplied hook's completion.
func (r *Relationer) CommitHook(hi HookInfo) error {
	if hi.HookKind == "broken" {
		if err := r.ru.Pinger().Kill(); err != nil {
			return err
		}
	}
	return r.rs.Commit(hi)
}
