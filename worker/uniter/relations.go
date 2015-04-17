// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	corecharm "gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/relation"
	"github.com/juju/juju/worker/uniter/runner"
)

// Relations exists to encapsulate relation state and operations behind an
// interface for the benefit of future refactoring.
type Relations interface {

	// Name returns the name of the relation with the supplied id, or an error
	// if the relation is unknown.
	Name(id int) (string, error)

	// Hooks returns the channel on which relation hook execution requests
	// are sent.
	Hooks() <-chan hook.Info

	// StartHooks starts sending hook execution requests on the Hooks channel.
	StartHooks()

	// StopHooks stops sending hook execution requests on the Hooks channel.
	StopHooks() error

	// PrepareHook returns the name of the supplied relation hook, or an error
	// if the hook is unknown or invalid given current state.
	PrepareHook(hookInfo hook.Info) (string, error)

	// CommitHook persists the state change encoded in the supplied relation
	// hook, or returns an error if the hook is unknown or invalid given
	// current relation state.
	CommitHook(hookInfo hook.Info) error

	// GetInfo returns information about current relation state.
	GetInfo() map[int]*runner.RelationInfo

	// Update checks for and responds to changes in the life states of the
	// relations with the supplied ids. If any id corresponds to an alive
	// relation that is not already recorded, the unit will enter scope for
	// that relation and start its hook queue.
	Update(ids []int) error

	// SetDying notifies all known relations that the only hooks to be requested
	// should be those necessary to cleanly exit the relation.
	SetDying() error
}

// relations implements Relations.
type relations struct {
	st            *uniter.State
	unit          *uniter.Unit
	charmDir      string
	relationsDir  string
	relationers   map[int]*Relationer
	relationHooks chan hook.Info
	abort         <-chan struct{}
}

func newRelations(st *uniter.State, tag names.UnitTag, paths Paths, abort <-chan struct{}) (*relations, error) {
	unit, err := st.Unit(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	r := &relations{
		st:            st,
		unit:          unit,
		charmDir:      paths.State.CharmDir,
		relationsDir:  paths.State.RelationsDir,
		relationers:   make(map[int]*Relationer),
		relationHooks: make(chan hook.Info),
		abort:         abort,
	}
	if err := r.init(); err != nil {
		return nil, errors.Trace(err)
	}
	return r, nil
}

// init reconciles the local relation state dirs with the remote state of
// the corresponding relations. It's only expected to be called while a
// *relations is being created.
func (r *relations) init() error {
	joinedRelationTags, err := r.unit.JoinedRelations()
	if err != nil {
		return errors.Trace(err)
	}
	joinedRelations := make(map[int]*uniter.Relation)
	for _, tag := range joinedRelationTags {
		relation, err := r.st.Relation(tag)
		if err != nil {
			return errors.Trace(err)
		}
		joinedRelations[relation.Id()] = relation
	}
	knownDirs, err := relation.ReadAllStateDirs(r.relationsDir)
	if err != nil {
		return errors.Trace(err)
	}
	for id, dir := range knownDirs {
		if rel, ok := joinedRelations[id]; ok {
			if err := r.add(rel, dir); err != nil {
				return errors.Trace(err)
			}
		} else if err := dir.Remove(); err != nil {
			return errors.Trace(err)
		}
	}
	for id, rel := range joinedRelations {
		if _, ok := knownDirs[id]; ok {
			continue
		}
		dir, err := relation.ReadStateDir(r.relationsDir, id)
		if err != nil {
			return errors.Trace(err)
		}
		if err := r.add(rel, dir); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// Name is part of the Relations interface.
func (r *relations) Name(id int) (string, error) {
	relationer, found := r.relationers[id]
	if !found {
		return "", errors.Errorf("unknown relation: %d", id)
	}
	return relationer.ru.Endpoint().Name, nil
}

// Hooks is part of the Relations interface.
func (r *relations) Hooks() <-chan hook.Info {
	return r.relationHooks
}

// StartHooks is part of the Relations interface.
func (r *relations) StartHooks() {
	for _, relationer := range r.relationers {
		relationer.StartHooks()
	}
}

// StopHooks is part of the Relations interface.
func (r *relations) StopHooks() (err error) {
	for _, relationer := range r.relationers {
		if e := relationer.StopHooks(); e != nil {
			if err == nil {
				err = e
			} else {
				logger.Errorf("additional error while stopping hooks: %v", e)
			}
		}
	}
	return err
}

// PrepareHook is part of the Relations interface.
func (r *relations) PrepareHook(hookInfo hook.Info) (string, error) {
	if !hookInfo.Kind.IsRelation() {
		return "", errors.Errorf("not a relation hook: %#v", hookInfo)
	}
	relationer, found := r.relationers[hookInfo.RelationId]
	if !found {
		return "", errors.Errorf("unknown relation: %d", hookInfo.RelationId)
	}
	return relationer.PrepareHook(hookInfo)
}

// CommitHook is part of the Relations interface.
func (r *relations) CommitHook(hookInfo hook.Info) error {
	if !hookInfo.Kind.IsRelation() {
		return errors.Errorf("not a relation hook: %#v", hookInfo)
	}
	relationer, found := r.relationers[hookInfo.RelationId]
	if !found {
		return errors.Errorf("unknown relation: %d", hookInfo.RelationId)
	}
	if hookInfo.Kind == hooks.RelationBroken {
		delete(r.relationers, hookInfo.RelationId)
	}
	return relationer.CommitHook(hookInfo)
}

// GetInfo is part of the Relations interface.
func (r *relations) GetInfo() map[int]*runner.RelationInfo {
	relationInfos := map[int]*runner.RelationInfo{}
	for id, relationer := range r.relationers {
		relationInfos[id] = relationer.ContextInfo()
	}
	return relationInfos
}

// Update is part of the Relations interface.
func (r *relations) Update(ids []int) error {
	for _, id := range ids {
		if relationer, found := r.relationers[id]; found {
			rel := relationer.ru.Relation()
			if err := rel.Refresh(); err != nil {
				return errors.Annotatef(err, "cannot update relation %q", rel)
			}
			if rel.Life() == params.Dying {
				if err := r.setDying(id); err != nil {
					return errors.Trace(err)
				}
			}
			continue
		}
		// Relations that are not alive are simply skipped, because they
		// were not previously known anyway.
		rel, err := r.st.RelationById(id)
		if err != nil {
			if params.IsCodeNotFoundOrCodeUnauthorized(err) {
				continue
			}
			return errors.Trace(err)
		}
		if rel.Life() != params.Alive {
			continue
		}
		// Make sure we ignore relations not implemented by the unit's charm.
		ch, err := corecharm.ReadCharmDir(r.charmDir)
		if err != nil {
			return errors.Trace(err)
		}
		if ep, err := rel.Endpoint(); err != nil {
			return errors.Trace(err)
		} else if !ep.ImplementedBy(ch) {
			logger.Warningf("skipping relation with unknown endpoint %q", ep.Name)
			continue
		}
		dir, err := relation.ReadStateDir(r.relationsDir, id)
		if err != nil {
			return errors.Trace(err)
		}
		err = r.add(rel, dir)
		if err == nil {
			r.relationers[id].StartHooks()
			continue
		}
		e := dir.Remove()
		if !params.IsCodeCannotEnterScope(err) {
			return errors.Trace(err)
		}
		if e != nil {
			return errors.Trace(e)
		}
	}
	if ok, err := r.unit.IsPrincipal(); err != nil {
		return errors.Trace(err)
	} else if ok {
		return nil
	}
	// If no Alive relations remain between a subordinate unit's service
	// and its principal's service, the subordinate must become Dying.
	for _, relationer := range r.relationers {
		scope := relationer.ru.Endpoint().Scope
		if scope == corecharm.ScopeContainer && !relationer.dying {
			return nil
		}
	}
	return r.unit.Destroy()
}

// SetDying is part of the Relations interface.
// should be those necessary to cleanly exit the relation.
func (r *relations) SetDying() error {
	for id := range r.relationers {
		if err := r.setDying(id); err != nil {
			return err
		}
	}
	return nil
}

// add causes the unit agent to join the supplied relation, and to
// store persistent state in the supplied dir.
func (r *relations) add(rel *uniter.Relation, dir *relation.StateDir) (err error) {
	logger.Infof("joining relation %q", rel)
	ru, err := rel.Unit(r.unit)
	if err != nil {
		return errors.Trace(err)
	}
	relationer := NewRelationer(ru, dir, r.relationHooks)
	w, err := r.unit.Watch()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		if e := w.Stop(); e != nil {
			if err == nil {
				err = e
			} else {
				logger.Errorf("error stopping unit watcher: %v", e)
			}
		}
	}()
	for {
		select {
		case <-r.abort:
			return tomb.ErrDying
		case _, ok := <-w.Changes():
			if !ok {
				return watcher.EnsureErr(w)
			}
			err := relationer.Join()
			if params.IsCodeCannotEnterScopeYet(err) {
				logger.Infof("cannot enter scope for relation %q; waiting for subordinate to be removed", rel)
				continue
			} else if err != nil {
				return errors.Trace(err)
			}
			logger.Infof("joined relation %q", rel)
			r.relationers[rel.Id()] = relationer
			return nil
		}
	}
}

// setDying notifies the relationer identified by the supplied id that the
// only hook executions to be requested should be those necessary to cleanly
// exit the relation.
func (r *relations) setDying(id int) error {
	relationer, found := r.relationers[id]
	if !found {
		return nil
	}
	if err := relationer.SetDying(); err != nil {
		return errors.Trace(err)
	}
	if relationer.IsImplicit() {
		delete(r.relationers, id)
	}
	return nil
}
