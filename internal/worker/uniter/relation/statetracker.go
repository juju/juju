// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	stdcontext "context"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/kr/pretty"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/rpc/params"
)

// RelationStateTrackerConfig contains configuration values for creating a new
// RelationStateTracker instance.
type RelationStateTrackerConfig struct {
	Client            StateTrackerClient
	Unit              api.Unit
	CharmDir          string
	LeadershipContext context.LeadershipContext
	Abort             <-chan struct{}
	Logger            logger.Logger
}

// relationStateTracker implements RelationStateTracker.
type relationStateTracker struct {
	client          StateTrackerClient
	unit            api.Unit
	leaderCtx       context.LeadershipContext
	abort           <-chan struct{}
	subordinate     bool
	principalName   string
	charmDir        string
	relationers     map[int]Relationer
	remoteAppName   map[int]string
	relationCreated map[int]bool
	isPeerRelation  map[int]bool
	stateMgr        StateManager
	logger          logger.Logger
	newRelationer   func(api.RelationUnit, StateManager, UnitGetter, logger.Logger) Relationer
}

// NewRelationStateTracker returns a new RelationStateTracker instance.
func NewRelationStateTracker(ctx stdcontext.Context, cfg RelationStateTrackerConfig) (RelationStateTracker, error) {
	principalName, subordinate, err := cfg.Unit.PrincipalName(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	r := &relationStateTracker{
		client:          cfg.Client,
		unit:            cfg.Unit,
		leaderCtx:       cfg.LeadershipContext,
		subordinate:     subordinate,
		principalName:   principalName,
		charmDir:        cfg.CharmDir,
		relationers:     make(map[int]Relationer),
		remoteAppName:   make(map[int]string),
		relationCreated: make(map[int]bool),
		isPeerRelation:  make(map[int]bool),
		abort:           cfg.Abort,
		logger:          cfg.Logger,
		newRelationer:   NewRelationer,
	}
	r.stateMgr, err = NewStateManager(ctx, r.unit, r.logger)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := r.loadInitialState(ctx); err != nil {
		return nil, errors.Trace(err)
	}
	return r, nil
}

// loadInitialState reconciles the local state with the remote
// state of the corresponding relations.
func (r *relationStateTracker) loadInitialState(ctx stdcontext.Context) error {
	relationStatus, err := r.unit.RelationsStatus(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// Keep the relations ordered for reliable testing.
	var orderedIds []int
	isScopeRelations := make(map[int]api.Relation)
	relationSuspended := make(map[int]bool)
	for _, rs := range relationStatus {
		if !rs.InScope {
			continue
		}
		rel, err := r.client.Relation(ctx, rs.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		relationSuspended[rel.Id()] = rs.Suspended
		isScopeRelations[rel.Id()] = rel
		orderedIds = append(orderedIds, rel.Id())

		// The relation-created hook always fires before joining.
		// Since we are already in scope, the relation-created hook
		// must have fired in the past so we can mark the relation as
		// already created.
		r.relationCreated[rel.Id()] = true
	}

	if r.logger.IsLevelEnabled(logger.TRACE) {
		if mgr, ok := r.stateMgr.(*stateManager); ok {
			r.logger.Tracef(ctx, "initialising relation state tracker: %# v", pretty.Formatter(mgr.relationState))
		}
	}
	knownUnits := make(map[string]bool)
	for _, id := range r.stateMgr.KnownIDs() {
		if rel, ok := isScopeRelations[id]; ok {
			//shouldJoin := localRelState.Members[rel.]
			if err := r.joinRelation(ctx, rel); err != nil {
				return errors.Trace(err)
			}
		} else if !relationSuspended[id] {
			// Relations which are suspended may become active
			// again so we keep the local state, otherwise we
			// remove it.
			if err := r.stateMgr.RemoveRelation(ctx, id, r.client, knownUnits); err != nil {
				return errors.Trace(err)
			}
		}
	}

	for _, id := range orderedIds {
		rel := isScopeRelations[id]
		if r.stateMgr.RelationFound(id) {
			continue
		}
		if err := r.joinRelation(ctx, rel); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (r *relationStateTracker) relationGone(id int) {
	delete(r.relationers, id)
	delete(r.remoteAppName, id)
	delete(r.isPeerRelation, id)
	delete(r.relationCreated, id)
}

// joinRelation causes the unit agent to join the supplied relation, and to
// store persistent state. It will block until the
// operation succeeds or fails; or until the abort chan is closed, in which
// case it will return resolver.ErrLoopAborted.
func (r *relationStateTracker) joinRelation(ctx stdcontext.Context, rel api.Relation) (err error) {
	unitName := r.unit.Name()
	r.logger.Tracef(ctx, "%q (re-)joining: %q", unitName, rel)
	ru, err := rel.Unit(ctx, r.unit.Tag())
	if err != nil {
		return errors.Trace(err)
	}
	relationer := r.newRelationer(ru, r.stateMgr, r.client, r.logger)
	unitWatcher, err := r.unit.Watch(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		if e := worker.Stop(unitWatcher); e != nil {
			if err == nil {
				err = e
			} else {
				r.logger.Errorf(ctx, "while stopping unit watcher: %v", e)
			}
		}
	}()
	timeout := time.After(time.Minute)
	for {
		select {
		case <-r.abort:
			// Should this be a different error? e.g. resolver.ErrAborted, that
			// Loop translates into ErrLoopAborted?
			return resolver.ErrLoopAborted
		case <-timeout:
			return errors.Errorf("unit watcher for %q failed to trigger joining relation %q", unitName, rel)
		case _, ok := <-unitWatcher.Changes():
			if !ok {
				return errors.New("unit watcher closed")
			}
			err := relationer.Join(ctx)
			if params.IsCodeCannotEnterScopeYet(err) {
				r.logger.Infof(ctx, "cannot enter scope for relation %q; waiting for subordinate to be removed", rel)
				continue
			} else if err != nil {
				return errors.Trace(err)
			}
			// Leaders get to set the relation status.
			var isLeader bool
			isLeader, err = r.leaderCtx.IsLeader()
			if err != nil {
				return errors.Trace(err)
			}
			r.logger.Debugf(ctx, "unit %q (leader=%v) entered scope for relation %q", unitName, isLeader, rel)
			if isLeader {
				err = rel.SetStatus(ctx, relation.Joined)
				if err != nil {
					return errors.Trace(err)
				}
			}
			r.relationers[rel.Id()] = relationer
			return nil
		}
	}
}

func (r *relationStateTracker) SynchronizeScopes(ctx stdcontext.Context, remote remotestate.Snapshot) error {
	isTraceEnabled := r.logger.IsLevelEnabled(logger.TRACE)
	if isTraceEnabled {
		r.logger.Tracef(ctx, "%q synchronise scopes for remote relations %# v", r.unit.Name(), pretty.Formatter(remote.Relations))
	}
	var charmMeta *charm.Meta
	knownUnits := make(map[string]bool)
	for id, relationSnapshot := range remote.Relations {
		if relr, found := r.relationers[id]; found {
			// We've seen this relation before. The only changes
			// we care about are to the lifecycle state or status,
			// and to the member settings versions. We handle
			// differences in settings in nextRelationHook.
			relr.RelationUnit().Relation().UpdateSuspended(relationSnapshot.Suspended)
			if relationSnapshot.Life == life.Dying || relationSnapshot.Suspended {
				if err := r.setDying(ctx, id); err != nil {
					return errors.Trace(err)
				}
			}
			if isTraceEnabled {
				r.logger.Tracef(ctx, "already seen relation id %v", id)
			}
			continue
		}

		// Relations that are not alive are simply skipped, because they
		// were not previously known anyway.
		if relationSnapshot.Life != life.Alive || relationSnapshot.Suspended {
			continue
		}
		rel, err := r.client.RelationById(ctx, id)
		if err != nil {
			if params.IsCodeNotFoundOrCodeUnauthorized(err) {
				r.relationGone(id)
				r.logger.Tracef(ctx, "relation id %v has been removed", id)
				continue
			}
			return errors.Trace(err)
		}

		ep, err := rel.Endpoint(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		// Keep track of peer relations
		if ep.Role == charm.RolePeer {
			r.isPeerRelation[id] = true
		}
		// Keep track of the remote application
		r.remoteAppName[id] = rel.OtherApplication()

		// Make sure we ignore relations not implemented by the unit's charm.
		if !r.RelationCreated(id) {
			if charmMeta == nil {
				charmMeta, err = charm.ReadCharmDirMetadata(r.charmDir)
				if err != nil {
					if !errors.Is(err, charm.FileNotFound) {
						return errors.Trace(err)
					}
					r.logger.Warningf(ctx, "charm deleted, skipping relation endpoint check for %q", rel)
				}
			}
			if charmMeta != nil && !ep.ImplementedBy(charmMeta) {
				r.logger.Warningf(ctx, "skipping relation %q with unknown endpoint %q", rel, ep.Name)
				continue
			}
		}

		if joinErr := r.joinRelation(ctx, rel); joinErr != nil {
			removeErr := r.stateMgr.RemoveRelation(ctx, id, r.client, knownUnits)
			if !params.IsCodeCannotEnterScope(joinErr) {
				return errors.Trace(joinErr)
			} else if errors.Is(joinErr, errors.NotFound) {
				continue
			} else if removeErr != nil {
				return errors.Trace(removeErr)
			}
		}
	}

	if r.subordinate {
		return r.maybeSetSubordinateDying(ctx)
	}

	return nil
}

func (r *relationStateTracker) maybeSetSubordinateDying(ctx stdcontext.Context) error {
	// If no Alive relations remain between a subordinate unit's application
	// and its principal's application, the subordinate must become Dying.
	principalApp, err := names.UnitApplication(r.principalName)
	if err != nil {
		return errors.Trace(err)
	}
	for _, relationer := range r.relationers {
		relUnit := relationer.RelationUnit()
		if relUnit.Relation().OtherApplication() != principalApp {
			continue
		}
		scope := relUnit.Endpoint().Scope
		if scope == charm.ScopeContainer && !relationer.IsDying() {
			return nil
		}
	}
	return r.unit.Destroy(ctx)
}

// setDying notifies the relationer identified by the supplied id that the
// only hook executions to be requested should be those necessary to cleanly
// exit the relation.
func (r *relationStateTracker) setDying(ctx stdcontext.Context, id int) error {
	relationer, found := r.relationers[id]
	if !found {
		return nil
	}
	if err := relationer.SetDying(ctx); err != nil {
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
	return false, errors.NotFoundf("relation: %d", id)
}

// IsPeerRelation returns true if the endpoint for a relation ID has a Peer role.
func (r *relationStateTracker) IsPeerRelation(id int) (bool, error) {
	if rel := r.relationers[id]; rel != nil {
		return r.isPeerRelation[id], nil
	}

	return false, errors.NotFoundf("relation: %d", id)
}

// HasContainerScope returns true if the specified relation ID has a container
// scope.
func (r *relationStateTracker) HasContainerScope(id int) (bool, error) {
	if rel := r.relationers[id]; rel != nil {
		return rel.RelationUnit().Endpoint().Scope == charm.ScopeContainer, nil
	}

	return false, errors.NotFoundf("relation: %d", id)
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

	return nil, errors.NotFoundf("relation: %d", id)
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
		// There may have been a hook queued prior to a restart
		// and the relation has since been deleted.
		// There's nothing to prepare so allow the uniter
		// to continue with the next operation.
		r.logger.Warningf(stdcontext.Background(), "preparing hook %v for %v, relation %d has been removed", hookInfo.Kind, r.principalName, hookInfo.RelationId)
		return "", operation.ErrSkipExecute
	}
	return relationer.PrepareHook(hookInfo)
}

// CommitHook is part of the RelationStateTracker interface.
func (r *relationStateTracker) CommitHook(ctx stdcontext.Context, hookInfo hook.Info) (err error) {
	defer func() {
		if err != nil {
			return
		}

		if hookInfo.Kind == hooks.RelationCreated {
			r.relationCreated[hookInfo.RelationId] = true
		} else if hookInfo.Kind == hooks.RelationBroken {
			r.relationGone(hookInfo.RelationId)
		}
	}()
	if !hookInfo.Kind.IsRelation() {
		return errors.Errorf("not a relation hook: %#v", hookInfo)
	}
	relationer, found := r.relationers[hookInfo.RelationId]
	if !found {
		// There may have been a hook queued prior to a restart
		// and the relation has since been deleted.
		// There's nothing to commit so allow the uniter
		// to continue with the next operation.
		r.logger.Warningf(ctx, "committing hook %v for %v, relation %d has been removed", hookInfo.Kind, r.principalName, hookInfo.RelationId)
		return nil
	}
	return relationer.CommitHook(ctx, hookInfo)
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
		return "", errors.NotFoundf("relation: %d", id)
	}
	return relationer.RelationUnit().Endpoint().Name, nil
}

// LocalUnitName returns the name for the local unit.
func (r *relationStateTracker) LocalUnitName() string {
	return r.unit.Name()
}

// LocalUnitAndApplicationLife returns the life values for the local unit and
// application.
func (r *relationStateTracker) LocalUnitAndApplicationLife(ctx stdcontext.Context) (life.Value, life.Value, error) {
	if err := r.unit.Refresh(ctx); err != nil {
		return life.Value(""), life.Value(""), errors.Trace(err)
	}

	app, err := r.unit.Application(ctx)
	if err != nil {
		return life.Value(""), life.Value(""), errors.Trace(err)
	}

	return r.unit.Life(), app.Life(), nil
}

// Report provides information for the engine report.
func (r *relationStateTracker) Report() map[string]interface{} {
	result := make(map[string]interface{})

	stateMgr, ok := r.stateMgr.(*stateManager)
	if !ok {
		return nil
	}
	stateMgr.mu.Lock()
	relationState := stateMgr.relationState
	stateMgr.mu.Unlock()

	for id, st := range relationState {
		report := map[string]interface{}{
			"application-members": st.ApplicationMembers,
			"members":             st.Members,
			"is-peer":             r.isPeerRelation[id],
		}

		// Ensure that the relationer exists and is alive before reporting
		// the information.
		if relationer, ok := r.relationers[id]; ok && relationer != nil {
			report["dying"] = relationer.IsDying()
			report["endpoint"] = relationer.RelationUnit().Endpoint().Name
			report["relation"] = relationer.RelationUnit().Relation().String()
		}

		result[strconv.Itoa(id)] = report
	}

	return result
}
