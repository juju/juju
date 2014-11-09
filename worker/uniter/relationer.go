// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"gopkg.in/juju/charm.v4/hooks"

	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/relation"
)

// Relationer manages a unit's presence in a relation.
type Relationer struct {
	ru    *apiuniter.RelationUnit
	dir   *relation.StateDir
	queue relation.HookQueue
	hooks chan<- hook.Info
	dying bool
}

// NewRelationer creates a new Relationer. The unit will not join the
// relation until explicitly requested.
func NewRelationer(ru *apiuniter.RelationUnit, dir *relation.StateDir, hooks chan<- hook.Info) *Relationer {
	return &Relationer{
		ru:    ru,
		dir:   dir,
		hooks: hooks,
	}
}

// ContextInfo returns a represention of r's current state.
func (r *Relationer) ContextInfo() *context.RelationInfo {
	members := r.dir.State().Members
	memberNames := make([]string, 0, len(members))
	for memberName := range members {
		memberNames = append(memberNames, memberName)
	}
	return &context.RelationInfo{r.ru, memberNames}
}

// IsImplicit returns whether the local relation endpoint is implicit. Implicit
// relations do not run hooks.
func (r *Relationer) IsImplicit() bool {
	return r.ru.Endpoint().IsImplicit()
}

// Join initializes local state and causes the unit to enter its relation
// scope, allowing its counterpart units to detect its presence and settings
// changes. Local state directory is not created until needed.
func (r *Relationer) Join() error {
	if r.dying {
		panic("dying relationer must not join!")
	}
	// We need to make sure the state directory exists before we join the
	// relation, lest a subsequent ReadAllStateDirs report local state that
	// doesn't include relations recorded in remote state.
	if err := r.dir.Ensure(); err != nil {
		return err
	}
	// uniter.RelationUnit.EnterScope() sets the unit's private address
	// internally automatically, so no need to set it here.
	return r.ru.EnterScope()
}

// SetDying informs the relationer that the unit is departing the relation,
// and that the only hooks it should send henceforth are -departed hooks,
// until the relation is empty, followed by a -broken hook.
func (r *Relationer) SetDying() error {
	if r.IsImplicit() {
		r.dying = true
		return r.die()
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

// die is run when the relationer has no further responsibilities; it leaves
// relation scope, and removes the local relation state directory.
func (r *Relationer) die() error {
	if err := r.ru.LeaveScope(); err != nil {
		return err
	}
	return r.dir.Remove()
}

// StartHooks starts watching the relation, and sending hook.Info events on the
// hooks channel. It will panic if called when already responding to relation
// changes.
func (r *Relationer) StartHooks() error {
	if r.IsImplicit() {
		return nil
	}
	if r.queue != nil {
		panic("hooks already started!")
	}
	if r.dying {
		r.queue = relation.NewDyingHookQueue(r.dir.State(), r.hooks)
	} else {
		w, err := r.ru.Watch()
		if err != nil {
			return err
		}
		r.queue = relation.NewAliveHookQueue(r.dir.State(), r.hooks, w)
	}
	return nil
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
	if r.IsImplicit() {
		panic("implicit relations must not run hooks")
	}
	if err = r.dir.State().Validate(hi); err != nil {
		return
	}
	name := r.ru.Endpoint().Name
	return fmt.Sprintf("%s-%s", name, hi.Kind), nil
}

// CommitHook persists the fact of the supplied hook's completion.
func (r *Relationer) CommitHook(hi hook.Info) error {
	if r.IsImplicit() {
		panic("implicit relations must not run hooks")
	}
	if hi.Kind == hooks.RelationBroken {
		return r.die()
	}
	return r.dir.Write(hi)
}
