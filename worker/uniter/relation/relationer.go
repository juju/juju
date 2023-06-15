// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"fmt"

	"github.com/juju/charm/v11/hooks"
	"github.com/juju/errors"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner/context"
)

// relationer manages a unit's presence in a relation.
type relationer struct {
	relationId int
	ru         RelationUnit
	stateMgr   StateManager
	unitGetter UnitGetter
	dying      bool

	logger Logger
}

// NewRelationer creates a new relationer. The unit will not join the
// relation until explicitly requested.
func NewRelationer(ru RelationUnit, stateMgr StateManager, unitGetter UnitGetter, logger Logger) Relationer {
	return &relationer{
		relationId: ru.Relation().Id(),
		ru:         ru,
		stateMgr:   stateMgr,
		unitGetter: unitGetter,
		logger:     logger,
	}
}

// ContextInfo returns a representation of the relationer's current state.
func (r *relationer) ContextInfo() *context.RelationInfo {
	st, err := r.stateMgr.Relation(r.relationId)
	if errors.IsNotFound(err) {
		st = NewState(r.relationId)
	}
	members := st.Members
	memberNames := make([]string, 0, len(members))
	for memberName := range members {
		memberNames = append(memberNames, memberName)
	}
	sh, _ := r.ru.(*RelationUnitShim)
	return &context.RelationInfo{
		RelationUnit: &context.RelationUnitShim{sh.RelationUnit},
		MemberNames:  memberNames,
	}
}

// IsImplicit returns whether the local relation endpoint is implicit. Implicit
// relations do not run hooks.
func (r *relationer) IsImplicit() bool {
	return r.ru.Endpoint().IsImplicit()
}

// IsDying returns whether the relation is dying.
func (r *relationer) IsDying() bool {
	return r.dying
}

// RelationUnit returns the relation unit associated with this relationer instance.
func (r *relationer) RelationUnit() RelationUnit {
	return r.ru
}

// Join initializes local state and causes the unit to enter its relation
// scope, allowing its counterpart units to detect its presence and settings
// changes.
func (r *relationer) Join() error {
	if r.dying {
		return errors.New("dying relationer must not join!")
	}
	// We need to make sure the state is persisted inState before we join
	// the relation, lest a subsequent restart of the unit agent report
	// local state that doesn't include relations recorded in remote state.
	if !r.stateMgr.RelationFound(r.relationId) {
		// Add a state for the new relation to the state manager.
		st := NewState(r.relationId)
		if err := r.stateMgr.SetRelation(st); err != nil {
			return err
		}
	}
	// uniter.RelationUnit.EnterScope() sets the unit's private address
	// internally automatically, so no need to set it here.
	return r.ru.EnterScope()
}

// SetDying informs the relationer that the unit is departing the relation,
// and that the only hooks it should send henceforth are -departed hooks,
// until the relation is empty, followed by a -broken hook.
func (r *relationer) SetDying() error {
	if r.IsImplicit() {
		r.dying = true
		return r.die()
	}
	r.dying = true
	return nil
}

// die is run when the relationer has no further responsibilities; it leaves
// relation scope, and removes relation state.
func (r *relationer) die() error {
	err := r.ru.LeaveScope()
	if err != nil && !params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return errors.Annotatef(err, "leaving scope of relation %q", r.ru.Relation())
	}
	return r.stateMgr.RemoveRelation(r.relationId, r.unitGetter, map[string]bool{})
}

// PrepareHook checks that the relation is in a state such that it makes
// sense to execute the supplied hook, and ensures that the relation context
// contains the latest relation state as communicated in the hook.Info. It
// returns the name of the hook that must be run.
func (r *relationer) PrepareHook(hi hook.Info) (string, error) {
	if r.IsImplicit() {
		// Implicit relations always return ErrNoOperation from
		// NextOp.  Something broken if we reach here.
		r.logger.Errorf("implicit relations must not run hooks")
		return "", dependency.ErrBounce
	}
	st, err := r.stateMgr.Relation(hi.RelationId)
	if err != nil {
		return "", errors.Trace(err)
	}
	if err = st.Validate(hi); err != nil {
		return "", errors.Trace(err)
	}
	name := r.ru.Endpoint().Name
	return fmt.Sprintf("%s-%s", name, hi.Kind), nil
}

// CommitHook persists the fact of the supplied hook's completion.
func (r *relationer) CommitHook(hi hook.Info) error {
	if r.IsImplicit() {
		// Implicit relations always return ErrNoOperation from
		// NextOp.  Something broken if we reach here.
		r.logger.Errorf("implicit relations must not run hooks")
		return dependency.ErrBounce
	}
	if hi.Kind == hooks.RelationBroken {
		return r.die()
	}
	st, err := r.stateMgr.Relation(hi.RelationId)
	if err != nil {
		return errors.Trace(err)
	}
	st.UpdateStateForHook(hi, r.logger)
	return r.stateMgr.SetRelation(st)
}
