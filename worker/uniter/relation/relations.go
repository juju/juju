// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"
	corecharm "gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/runner/context"
)

var logger = loggo.GetLogger("juju.worker.uniter.relation")

// Relations exists to encapsulate relation state and operations behind an
// interface for the benefit of future refactoring.
type Relations interface {
	// Name returns the name of the relation with the supplied id, or an error
	// if the relation is unknown.
	Name(id int) (string, error)

	// PrepareHook returns the name of the supplied relation hook, or an error
	// if the hook is unknown or invalid given current state.
	PrepareHook(hookInfo hook.Info) (string, error)

	// CommitHook persists the state change encoded in the supplied relation
	// hook, or returns an error if the hook is unknown or invalid given
	// current relation state.
	CommitHook(hookInfo hook.Info) error

	// GetInfo returns information about current relation state.
	GetInfo() map[int]*context.RelationInfo

	// NextHook returns details on the next hook to execute, based on the local
	// and remote states.
	NextHook(resolver.LocalState, remotestate.Snapshot) (hook.Info, error)
}

// NewRelationsResolver returns a new Resolver that handles differences in
// relation state.
func NewRelationsResolver(r Relations) resolver.Resolver {
	return &relationsResolver{r}
}

type relationsResolver struct {
	relations Relations
}

// NextOp implements resolver.Resolver.
func (s *relationsResolver) NextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	hook, err := s.relations.NextHook(localState, remoteState)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return opFactory.NewRunHook(hook)
}

// relations implements Relations.
type relations struct {
	st           *uniter.State
	unit         *uniter.Unit
	charmDir     string
	relationsDir string
	relationers  map[int]*Relationer
	abort        <-chan struct{}
}

// NewRelations returns a new Relations instance.
func NewRelations(st *uniter.State, tag names.UnitTag, charmDir, relationsDir string, abort <-chan struct{}) (Relations, error) {
	unit, err := st.Unit(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	r := &relations{
		st:           st,
		unit:         unit,
		charmDir:     charmDir,
		relationsDir: relationsDir,
		relationers:  make(map[int]*Relationer),
		abort:        abort,
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
	knownDirs, err := ReadAllStateDirs(r.relationsDir)
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
		dir, err := ReadStateDir(r.relationsDir, id)
		if err != nil {
			return errors.Trace(err)
		}
		if err := r.add(rel, dir); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// NextHook implements Relations.
func (r *relations) NextHook(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
) (hook.Info, error) {

	if remoteState.Life == params.Dying {
		// The unit is Dying, so make sure all subordinates are dying.
		var destroyAllSubordinates bool
		for relationId, relationSnapshot := range remoteState.Relations {
			if relationSnapshot.Life != params.Alive {
				continue
			}
			relationer, ok := r.relationers[relationId]
			if !ok {
				continue
			}
			if relationer.ru.Endpoint().Scope == corecharm.ScopeContainer {
				relationSnapshot.Life = params.Dying
				remoteState.Relations[relationId] = relationSnapshot
				destroyAllSubordinates = true
			}
		}
		if destroyAllSubordinates {
			if err := r.unit.DestroyAllSubordinates(); err != nil {
				return hook.Info{}, errors.Trace(err)
			}
		}
	}

	// Add/remove local relation state; enter and leave scope as necessary.
	if err := r.update(remoteState.Relations); err != nil {
		return hook.Info{}, errors.Trace(err)
	}

	if localState.Kind != operation.Continue {
		return hook.Info{}, resolver.ErrNoOperation
	}

	// See if any of the relations have operations to perform.
	for relationId, relationSnapshot := range remoteState.Relations {
		relationer, ok := r.relationers[relationId]
		if !ok || relationer.IsImplicit() {
			continue
		}
		var remoteBroken bool
		if remoteState.Life == params.Dying || relationSnapshot.Life == params.Dying {
			relationSnapshot = remotestate.RelationSnapshot{}
			remoteBroken = true
			// TODO(axw) if relation is implicit, leave scope & remove.
		}
		// If either the unit or the relation are Dying,
		// then the relation should be broken.
		hook, err := nextRelationHook(relationer.dir.State(), relationSnapshot, remoteBroken)
		if err == resolver.ErrNoOperation {
			continue
		}
		return hook, err
	}
	return hook.Info{}, resolver.ErrNoOperation
}

// nextRelationHook returns the next hook op that should be executed in the
// relation characterised by the supplied local and remote state; or an error
// if the states do not refer to the same relation; or ErrRelationUpToDate if
// no hooks need to be executed.
func nextRelationHook(
	local *State,
	remote remotestate.RelationSnapshot,
	remoteBroken bool,
) (hook.Info, error) {

	// If there's a guaranteed next hook, return that.
	relationId := local.RelationId
	if local.ChangedPending != "" {
		unitName := local.ChangedPending
		return hook.Info{
			Kind:          hooks.RelationChanged,
			RelationId:    relationId,
			RemoteUnit:    unitName,
			ChangeVersion: remote.Members[unitName],
		}, nil
	}

	// Get the union of all relevant units, and sort them, so we produce events
	// in a consistent order (largely for the convenience of the tests).
	allUnitNames := set.NewStrings()
	for unitName := range local.Members {
		allUnitNames.Add(unitName)
	}
	for unitName := range remote.Members {
		allUnitNames.Add(unitName)
	}
	sortedUnitNames := allUnitNames.SortedValues()

	// If there are any locally known units that are no longer reflected in
	// remote state, depart them.
	for _, unitName := range sortedUnitNames {
		changeVersion, found := local.Members[unitName]
		if !found {
			continue
		}
		if _, found := remote.Members[unitName]; !found {
			return hook.Info{
				Kind:          hooks.RelationDeparted,
				RelationId:    relationId,
				RemoteUnit:    unitName,
				ChangeVersion: changeVersion,
			}, nil
		}
	}

	// If the relation's meant to be broken, break it.
	if remoteBroken {
		return hook.Info{
			Kind:       hooks.RelationBroken,
			RelationId: relationId,
		}, nil
	}

	// If there are any remote units not locally known, join them.
	for _, unitName := range sortedUnitNames {
		changeVersion, found := remote.Members[unitName]
		if !found {
			continue
		}
		if _, found := local.Members[unitName]; !found {
			return hook.Info{
				Kind:          hooks.RelationJoined,
				RelationId:    relationId,
				RemoteUnit:    unitName,
				ChangeVersion: changeVersion,
			}, nil
		}
	}

	// Finally scan for remote units whose latest version is not reflected
	// in local state.
	for _, unitName := range sortedUnitNames {
		remoteChangeVersion, found := remote.Members[unitName]
		if !found {
			continue
		}
		localChangeVersion, found := local.Members[unitName]
		if !found {
			continue
		}
		// NOTE(axw) we use != and not > to cater due to the
		// use of the relation settings document's txn-revno
		// as the version. When model-uuid migration occurs, the
		// document is recreated, resetting txn-revno.
		if remoteChangeVersion != localChangeVersion {
			return hook.Info{
				Kind:          hooks.RelationChanged,
				RelationId:    relationId,
				RemoteUnit:    unitName,
				ChangeVersion: remoteChangeVersion,
			}, nil
		}
	}

	// Nothing left to do for this relation.
	return hook.Info{}, resolver.ErrNoOperation
}

// Name is part of the Relations interface.
func (r *relations) Name(id int) (string, error) {
	relationer, found := r.relationers[id]
	if !found {
		return "", errors.Errorf("unknown relation: %d", id)
	}
	return relationer.ru.Endpoint().Name, nil
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
func (r *relations) GetInfo() map[int]*context.RelationInfo {
	relationInfos := map[int]*context.RelationInfo{}
	for id, relationer := range r.relationers {
		relationInfos[id] = relationer.ContextInfo()
	}
	return relationInfos
}

func (r *relations) update(remote map[int]remotestate.RelationSnapshot) error {
	for id, relationSnapshot := range remote {
		if _, found := r.relationers[id]; found {
			// We've seen this relation before. The only changes
			// we care about are to the lifecycle state, and to
			// the member settings versions. We handle differences
			// in settings in nextRelationHook.
			if relationSnapshot.Life == params.Dying {
				if err := r.setDying(id); err != nil {
					return errors.Trace(err)
				}
			}
			continue
		}
		// Relations that are not alive are simply skipped, because they
		// were not previously known anyway.
		if relationSnapshot.Life != params.Alive {
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
		dir, err := ReadStateDir(r.relationsDir, id)
		if err != nil {
			return errors.Trace(err)
		}
		addErr := r.add(rel, dir)
		if addErr == nil {
			continue
		}
		removeErr := dir.Remove()
		if !params.IsCodeCannotEnterScope(addErr) {
			return errors.Trace(addErr)
		}
		if removeErr != nil {
			return errors.Trace(removeErr)
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

// add causes the unit agent to join the supplied relation, and to
// store persistent state in the supplied dir. It will block until the
// operation succeeds or fails; or until the abort chan is closed, in
// which case it will return resolver.ErrLoopAborted.
func (r *relations) add(rel *uniter.Relation, dir *StateDir) (err error) {
	logger.Infof("joining relation %q", rel)
	ru, err := rel.Unit(r.unit)
	if err != nil {
		return errors.Trace(err)
	}
	relationer := NewRelationer(ru, dir)
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
