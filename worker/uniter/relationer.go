package uniter

import (
	"fmt"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/presence"
)

// Relationer allows a client to control a unit's presence in, and reactions
// to changes in, a relation.
type Relationer struct {
	ctx    *server.RelationContext
	ru     *state.RelationUnit
	rs     *RelationState
	pinger *presence.Pinger
	queue  *HookQueue
	hooks  chan<- HookInfo
}

// NewRelationer creates a new Relationer. The new instance will not signal
// its presence, and will not respond to relation changes, until asked.
func NewRelationer(ru *state.RelationUnit, rs *RelationState, hooks chan<- HookInfo) *Relationer {
	// TODO lifecycle handling?
	return &Relationer{
		ctx:   server.NewRelationContext(ru, rs.Members),
		ru:    ru,
		rs:    rs,
		hooks: hooks,
	}
}

// StartPresence starts a Pinger on the unit's relation presence node. It will
// panic if it is called while its Pinger is active.
func (r *Relationer) StartPresence() error {
	if r.pinger != nil {
		panic("presence already started!")
	}
	pinger, err := r.ru.Join()
	if err != nil {
		return err
	}
	r.pinger = pinger
	return nil
}

// StartHooks starts watching the relation, and sending HookInfo events on the
// hooks channel. It will panic if called when already responding to relation
// changes.
func (r *Relationer) StartHooks() {
	if r.queue != nil {
		panic("hooks already started!")
	}
	r.queue = NewHookQueue(r.rs, r.hooks, r.ru.Watch())
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

// StopPresence ensures that the relationer is not actively signalling its
// presence in the relation. It does *not* remove the presence node.
func (r *Relationer) StopPresence() error {
	if r.pinger == nil {
		return nil
	}
	pinger := r.pinger
	r.pinger = nil
	return pinger.Stop()
}

// Context returns the RelationContext that provides access to the relation
// when running hooks.
func (r *Relationer) Context() *server.RelationContext {
	return r.ctx
}

// PrepareHook ensures that the hook is valid, and that the relation context
// contains the latest relation state as communicated by the HookInfo. It
// returns the name of the hook that must be run.
func (r *Relationer) PrepareHook(hi HookInfo) (hookName string, err error) {
	if err = r.rs.Validate(hi); err != nil {
		return "", err
	}
	r.ctx.SetMembers(hi.Members)
	relName := r.ru.Endpoint().RelationName
	return fmt.Sprintf("%s-relation-%s", relName, hi.HookKind), nil
}

// CommitHook persists the fact of the supplied hook's completion.
func (r *Relationer) CommitHook(hi HookInfo) error {
	return r.rs.Commit(hi)
}
