// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"
	corecharm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/runner/context"
)

//go:generate mockgen -package mocks -destination mocks/mock_statetracker.go github.com/juju/juju/worker/uniter/relation RelationStateTracker

type RelationStateTracker interface {
	// PrepareHook returns the name of the supplied relation hook, or an error
	// if the hook is unknown or invalid given current state.
	PrepareHook(hook.Info) (string, error)

	// CommitHook persists the state change encoded in the supplied relation
	// hook, or returns an error if the hook is unknown or invalid given
	// current relation state.
	CommitHook(hook.Info) error

	// SyncronizeScopes ensures that the locally tracked relation scopes
	// reflect the contents of the remote state snapshot by entering or
	// exiting scopes as required.
	SynchronizeScopes(remotestate.Snapshot) error

	// IsKnown returns true if the relation ID is known by the tracker.
	IsKnown(int) bool

	// IsImplicit returns true if the endpoint for a relation ID is implicit.
	IsImplicit(int) (bool, error)

	// IsPeerRelation returns true if the endpoint for a relation ID has a
	// Peer role.
	IsPeerRelation(int) (bool, error)

	// HasContainerScope returns true if the specified relation ID has a
	// container scope.
	HasContainerScope(int) (bool, error)

	// RelationCreated returns true if a relation created hook has been
	// fired for the specified relation ID.
	RelationCreated(int) bool

	// RemoteApplication returns the remote application name associated
	// with the specified relation ID.
	RemoteApplication(int) string

	// State returns a State instance for accessing the local state
	// for a relation ID.
	State(int) (*State, error)

	// StateFound returns a true if there is a state for the given in
	// in the state manager.
	StateFound(int) bool

	// GetInfo returns information about current relation state.
	GetInfo() map[int]*context.RelationInfo

	// Name returns the name of the relation with the supplied id, or an error
	// if the relation is unknown.
	Name(id int) (string, error)

	// LocalUnitName returns the name for the local unit.
	LocalUnitName() string

	// LocalUnitAndApplicationLife returns the life values for the local
	// unit and application.
	LocalUnitAndApplicationLife() (life.Value, life.Value, error)
}

// LeadershipContextFunc is a function that returns a leadership context.
type LeadershipContextFunc func(accessor context.LeadershipSettingsAccessor, tracker leadership.Tracker, unitName string) context.LeadershipContext

// RelationStateTrackerConfig contains configuration values for creating a new
// RlationStateTracker instance.
type RelationStateTrackerConfig struct {
	State                *uniter.State
	Unit                 *uniter.Unit
	Tracker              leadership.Tracker
	CharmDir             string
	NewLeadershipContext LeadershipContextFunc
	Abort                <-chan struct{}
}

// relationStateTracker implements RelationStateTracker.
type relationStateTracker struct {
	st              *uniter.State
	unit            *uniter.Unit
	leaderCtx       context.LeadershipContext
	abort           <-chan struct{}
	subordinate     bool
	principalName   string
	charmDir        string
	relationers     map[int]*Relationer
	remoteAppName   map[int]string
	relationCreated map[int]bool
	isPeerRelation  map[int]bool
	stateMgr        StateManager
}

// NewRelationStateTracker returns a new RelationStateTracker instance.
func NewRelationStateTracker(cfg RelationStateTrackerConfig) (RelationStateTracker, error) {
	principalName, subordinate, err := cfg.Unit.PrincipalName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	leadershipContext := cfg.NewLeadershipContext(
		cfg.State.LeadershipSettings,
		cfg.Tracker,
		cfg.Unit.Tag().Id(),
	)

	r := &relationStateTracker{
		st:              cfg.State,
		unit:            cfg.Unit,
		leaderCtx:       leadershipContext,
		subordinate:     subordinate,
		principalName:   principalName,
		charmDir:        cfg.CharmDir,
		relationers:     make(map[int]*Relationer),
		remoteAppName:   make(map[int]string),
		relationCreated: make(map[int]bool),
		isPeerRelation:  make(map[int]bool),
		abort:           cfg.Abort,
	}
	if err := r.loadInitialState(); err != nil {
		return nil, errors.Trace(err)
	}
	return r, nil
}

// loadInitialState reconciles the local state with the remote
// state of the corresponding relations.
func (r *relationStateTracker) loadInitialState() error {
	relationStatus, err := r.unit.RelationsStatus()
	if err != nil {
		return errors.Trace(err)
	}

	r.stateMgr, err = NewStateManager(r.unit)
	if err != nil {
		return errors.Trace(err)
	}

	// Keep the relations ordered for reliable testing.
	var orderedIds []int
	activeRelations := make(map[int]*uniter.Relation)
	relationSuspended := make(map[int]bool)
	for _, rs := range relationStatus {
		if !rs.InScope {
			continue
		}
		rel, err := r.st.Relation(rs.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		relationSuspended[rel.Id()] = rs.Suspended
		activeRelations[rel.Id()] = rel
		orderedIds = append(orderedIds, rel.Id())

		// The relation-created hook always fires before joining.
		// Since we are already in scope, the relation-created hook
		// must have fired in the past so we can mark the relation as
		// already created.
		r.relationCreated[rel.Id()] = true
	}

	for _, id := range r.stateMgr.KnownIDs() {
		if rel, ok := activeRelations[id]; ok {
			if err := r.joinRelation(rel); err != nil {
				return errors.Trace(err)
			}
		} else if !relationSuspended[id] {
			// Relations which are suspended may become active
			// again so we keep the local state, otherwise we
			// remove it.
			if err := r.stateMgr.RemoveRelation(id); err != nil {
				return errors.Trace(err)
			}
		}
	}

	for _, id := range orderedIds {
		rel := activeRelations[id]
		if r.stateMgr.RelationFound(id) {
			continue
		}
		if err := r.joinRelation(rel); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// joinRelation causes the unit agent to join the supplied relation, and to
// store persistent state. It will block until the
// operation succeeds or fails; or until the abort chan is closed, in which
// case it will return resolver.ErrLoopAborted.
func (r *relationStateTracker) joinRelation(rel *uniter.Relation) (err error) {
	logger.Infof("joining relation %q", rel)
	ru, err := rel.Unit(r.unit)
	if err != nil {
		return errors.Trace(err)
	}
	relationer := NewRelationer(ru, r.stateMgr)
	unitWatcher, err := r.unit.Watch()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		if e := worker.Stop(unitWatcher); e != nil {
			if err == nil {
				err = e
			} else {
				logger.Errorf("while stopping unit watcher: %v", e)
			}
		}
	}()
	for {
		select {
		case <-r.abort:
			// Should this be a different error? e.g. resolver.ErrAborted, that
			// Loop translates into ErrLoopAborted?
			return resolver.ErrLoopAborted
		case _, ok := <-unitWatcher.Changes():
			if !ok {
				return errors.New("unit watcher closed")
			}
			err := relationer.Join()
			if params.IsCodeCannotEnterScopeYet(err) {
				logger.Infof("cannot enter scope for relation %q; waiting for subordinate to be removed", rel)
				continue
			} else if err != nil {
				return errors.Trace(err)
			}
			logger.Infof("joined relation %q", rel)
			// Leaders get to set the relation status.
			var isLeader bool
			isLeader, err = r.leaderCtx.IsLeader()
			if err != nil {
				return errors.Trace(err)
			}
			if isLeader {
				err = rel.SetStatus(relation.Joined)
				if err != nil {
					return errors.Trace(err)
				}
			}
			r.relationers[rel.Id()] = relationer
			return nil
		}
	}
}

func (r *relationStateTracker) SynchronizeScopes(remote remotestate.Snapshot) error {
	var charmSpec *charm.CharmDir
	for id, relationSnapshot := range remote.Relations {
		if rel, found := r.relationers[id]; found {
			// We've seen this relation before. The only changes
			// we care about are to the lifecycle state or status,
			// and to the member settings versions. We handle
			// differences in settings in nextRelationHook.
			rel.ru.Relation().UpdateSuspended(relationSnapshot.Suspended)
			if relationSnapshot.Life == life.Dying || relationSnapshot.Suspended {
				if err := r.setDying(id); err != nil {
					return errors.Trace(err)
				}
			}
			continue
		}

		// Relations that are not alive are simply skipped, because they
		// were not previously known anyway.
		if relationSnapshot.Life != life.Alive || relationSnapshot.Suspended {
			continue
		}
		rel, err := r.st.RelationById(id)
		if err != nil {
			if params.IsCodeNotFoundOrCodeUnauthorized(err) {
				continue
			}
			return errors.Trace(err)
		}

		// Make sure we ignore relations not implemented by the unit's charm.
		if charmSpec == nil {
			if charmSpec, err = charm.ReadCharmDir(r.charmDir); err != nil {
				return errors.Trace(err)
			}
		}

		ep, err := rel.Endpoint()
		if err != nil {
			return errors.Trace(err)
		} else if !ep.ImplementedBy(charmSpec) {
			logger.Warningf("skipping relation with unknown endpoint %q", ep.Name)
			continue
		}

		// Keep track of peer relations
		if ep.Role == charm.RolePeer {
			r.isPeerRelation[id] = true
		}

		if joinErr := r.joinRelation(rel); joinErr != nil {
			removeErr := r.stateMgr.RemoveRelation(id)
			if !params.IsCodeCannotEnterScope(joinErr) {
				return errors.Trace(joinErr)
			} else if removeErr != nil {
				return errors.Trace(removeErr)
			}
		}

		// Keep track of the remote application
		r.remoteAppName[id] = rel.OtherApplication()
	}

	if !r.subordinate {
		return nil
	}

	// If no Alive relations remain between a subordinate unit's application
	// and its principal's application, the subordinate must become Dying.
	principalApp, err := names.UnitApplication(r.principalName)
	if err != nil {
		return errors.Trace(err)
	}
	for _, relationer := range r.relationers {
		if relationer.ru.Relation().OtherApplication() != principalApp {
			continue
		}
		scope := relationer.ru.Endpoint().Scope
		if scope == corecharm.ScopeContainer && !relationer.dying {
			return nil
		}
	}
	return r.unit.Destroy()
}

// setDying notifies the relationer identified by the supplied id that the
// only hook executions to be requested should be those necessary to cleanly
// exit the relation.
func (r *relationStateTracker) setDying(id int) error {
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

// IsKnown returns true if the relation ID is known by the tracker.
func (r *relationStateTracker) IsKnown(id int) bool {
	return r.relationers[id] != nil
}

// IsImplicit returns true if the endpoint for a relation ID is implicit.
func (r *relationStateTracker) IsImplicit(id int) (bool, error) {
	if rel := r.relationers[id]; rel != nil {
		return rel.IsImplicit(), nil
	}

	return false, errors.Errorf("unknown relation: %d", id)
}

// IsPeerRelation returns true if the endpoint for a relation ID has a Peer role.
func (r *relationStateTracker) IsPeerRelation(id int) (bool, error) {
	if rel := r.relationers[id]; rel != nil {
		return r.isPeerRelation[id], nil
	}

	return false, errors.Errorf("unknown relation: %d", id)
}

// HasContainerScope returns true if the specified relation ID has a container
// scope.
func (r *relationStateTracker) HasContainerScope(id int) (bool, error) {
	if rel := r.relationers[id]; rel != nil {
		return rel.RelationUnit().Endpoint().Scope == charm.ScopeContainer, nil
	}

	return false, errors.Errorf("unknown relation: %d", id)
}

// RelationCreated returns true if a relation created hook has been
// fired for the specified relation ID.
func (r *relationStateTracker) RelationCreated(id int) bool {
	return r.relationCreated[id]
}

// RemoteApplication returns the remote application name associated with the
// specified relation ID.
func (r *relationStateTracker) RemoteApplication(id int) string {
	return r.remoteAppName[id]
}

// State returns a State instance for accessing the persisted state for a
// relation ID.
func (r *relationStateTracker) State(id int) (*State, error) {
	if rel, ok := r.relationers[id]; ok && rel != nil {
		return r.stateMgr.Relation(id)
	}

	return nil, errors.Errorf("unknown relation: %d", id)
}

func (r *relationStateTracker) StateFound(id int) bool {
	return r.stateMgr.RelationFound(id)
}

// PrepareHook is part of the RelationStateTracker interface.
func (r *relationStateTracker) PrepareHook(hookInfo hook.Info) (string, error) {
	if !hookInfo.Kind.IsRelation() {
		return "", errors.Errorf("not a relation hook: %#v", hookInfo)
	}
	relationer, found := r.relationers[hookInfo.RelationId]
	if !found {
		return "", errors.Errorf("unknown relation: %d", hookInfo.RelationId)
	}
	return relationer.PrepareHook(hookInfo)
}

// CommitHook is part of the RelationStateTracker interface.
func (r *relationStateTracker) CommitHook(hookInfo hook.Info) (err error) {
	defer func() {
		if err != nil {
			return
		}

		if hookInfo.Kind == hooks.RelationCreated {
			r.relationCreated[hookInfo.RelationId] = true
		} else if hookInfo.Kind == hooks.RelationBroken {
			delete(r.relationers, hookInfo.RelationId)
			delete(r.relationCreated, hookInfo.RelationId)
			delete(r.remoteAppName, hookInfo.RelationId)
		}
	}()
	if !hookInfo.Kind.IsRelation() {
		return errors.Errorf("not a relation hook: %#v", hookInfo)
	}
	relationer, found := r.relationers[hookInfo.RelationId]
	if !found {
		return errors.Errorf("unknown relation: %d", hookInfo.RelationId)
	}
	return relationer.CommitHook(hookInfo)
}

// GetInfo is part of the Relations interface.
func (r *relationStateTracker) GetInfo() map[int]*context.RelationInfo {
	relationInfos := map[int]*context.RelationInfo{}
	for id, relationer := range r.relationers {
		relationInfos[id] = relationer.ContextInfo()
	}
	return relationInfos
}

// Name is part of the Relations interface.
func (r *relationStateTracker) Name(id int) (string, error) {
	relationer, found := r.relationers[id]
	if !found {
		return "", errors.Errorf("unknown relation: %d", id)
	}
	return relationer.ru.Endpoint().Name, nil
}

// LocalUnitName returns the name for the local unit.
func (r *relationStateTracker) LocalUnitName() string {
	return r.unit.Name()
}

// LocalUnitAndApplicationLife returns the life values for the local unit and
// application.
func (r *relationStateTracker) LocalUnitAndApplicationLife() (life.Value, life.Value, error) {
	if err := r.unit.Refresh(); err != nil {
		return life.Value(""), life.Value(""), errors.Trace(err)
	}

	app, err := r.unit.Application()
	if err != nil {
		return life.Value(""), life.Value(""), errors.Trace(err)
	}

	return r.unit.Life(), app.Life(), nil
}
